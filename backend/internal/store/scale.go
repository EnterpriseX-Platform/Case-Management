package store

// Large-volume features: archiving closed cases, a daily summary table for
// reports, and a read replica for heavy read-only queries.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DayMetric is one row of the daily summary.
type DayMetric struct {
	Day        string  `json:"day"` // YYYY-MM-DD
	ProcessKey string  `json:"processKey"`
	Started    int     `json:"started"`
	Completed  int     `json:"completed"`
	Cancelled  int     `json:"cancelled"`
	CycleHours float64 `json:"cycleHours"` // sum over completed cases
}

// Scaler is implemented by stores with archive and summary support.
type Scaler interface {
	// ArchiveCases moves up to limit cases closed before `before` (with
	// their events and work items) to the archive tables.
	ArchiveCases(ctx context.Context, before time.Time, limit int) (int, error)
	// RollupDaily recomputes the daily summary from `from` (a day) on.
	RollupDaily(ctx context.Context, from time.Time) error
	DailyMetrics(ctx context.Context, from, to time.Time, processKey string) ([]DayMetric, error)
	// OpenByStage counts running cases per stage (exact, any volume).
	OpenByStage(ctx context.Context, processKey string) (map[string]int, error)
}

// Reader returns a store that reads from the replica at url (writes still
// go to the primary through the original store).
func (p *Postgres) Reader(ctx context.Context, url string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	return &Postgres{pool: pool, Tenant: p.Tenant}, nil
}

func (p *Postgres) ArchiveCases(ctx context.Context, before time.Time, limit int) (int, error) {
	n := 0
	err := pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM cm_case WHERE tenant_id=$1 AND status IN ('completed','cancelled')
			AND (data->>'updatedAt')::timestamptz < $2 ORDER BY created_at LIMIT $3 FOR UPDATE SKIP LOCKED`, p.Tenant, before, limit)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil || len(ids) == 0 {
			return err
		}
		for _, q := range []string{
			`WITH m AS (DELETE FROM cm_event WHERE tenant_id=$1 AND case_id=ANY($2) RETURNING *)
			 INSERT INTO cm_event_archive (tenant_id,seq,case_id,at,type,node,data) SELECT tenant_id,seq,case_id,at,type,node,data FROM m ON CONFLICT DO NOTHING`,
			`WITH m AS (DELETE FROM cm_work WHERE tenant_id=$1 AND case_id=ANY($2) RETURNING *)
			 INSERT INTO cm_work_archive SELECT * FROM m ON CONFLICT DO NOTHING`,
			`DELETE FROM cm_subscription WHERE tenant_id=$1 AND case_id=ANY($2)`,
			`WITH m AS (DELETE FROM cm_case WHERE tenant_id=$1 AND id=ANY($2) RETURNING *)
			 INSERT INTO cm_case_archive SELECT * FROM m ON CONFLICT DO NOTHING`,
		} {
			if _, err := tx.Exec(ctx, q, p.Tenant, ids); err != nil {
				return err
			}
		}
		n = len(ids)
		return nil
	})
	return n, err
}

func (p *Postgres) RollupDaily(ctx context.Context, from time.Time) error {
	day := from.Truncate(24 * time.Hour)
	return p.exec(ctx, `
		WITH c AS (
		  SELECT process_key, created_at, status, (data->>'updatedAt')::timestamptz AS updated_at FROM cm_case WHERE tenant_id=$1
		  UNION ALL
		  SELECT process_key, created_at, status, (data->>'updatedAt')::timestamptz FROM cm_case_archive WHERE tenant_id=$1
		), days AS (
		  SELECT process_key, created_at::date AS day, count(*) AS started, 0 AS completed, 0 AS cancelled, 0::float8 AS hours
		    FROM c WHERE created_at >= $2 GROUP BY 1, 2
		  UNION ALL
		  SELECT process_key, updated_at::date, 0, count(*) FILTER (WHERE status='completed'), count(*) FILTER (WHERE status='cancelled'),
		         coalesce(sum(extract(epoch FROM updated_at-created_at)/3600) FILTER (WHERE status='completed'), 0)
		    FROM c WHERE status IN ('completed','cancelled') AND updated_at >= $2 GROUP BY 1, 2
		)
		INSERT INTO cm_metric_daily (tenant_id, day, process_key, started, completed, cancelled, cycle_hours)
		SELECT $1, day, process_key, sum(started), sum(completed), sum(cancelled), sum(hours) FROM days GROUP BY day, process_key
		ON CONFLICT (tenant_id, day, process_key) DO UPDATE SET started=EXCLUDED.started, completed=EXCLUDED.completed,
		  cancelled=EXCLUDED.cancelled, cycle_hours=EXCLUDED.cycle_hours`, p.Tenant, day)
}

func (p *Postgres) DailyMetrics(ctx context.Context, from, to time.Time, processKey string) ([]DayMetric, error) {
	rows, err := p.pool.Query(ctx, `SELECT to_char(day,'YYYY-MM-DD'), process_key, started, completed, cancelled, cycle_hours
		FROM cm_metric_daily WHERE tenant_id=$1 AND day BETWEEN $2::date AND $3::date AND ($4='' OR process_key=$4) ORDER BY day`,
		p.Tenant, from, to, processKey)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (DayMetric, error) {
		var m DayMetric
		err := r.Scan(&m.Day, &m.ProcessKey, &m.Started, &m.Completed, &m.Cancelled, &m.CycleHours)
		return m, err
	})
}

// archived reads a row from an archive table when it left the live table.
func (p *Postgres) archivedCase(ctx context.Context, id string) (*Case, error) {
	return one[Case](ctx, p.q(ctx), `SELECT data FROM cm_case_archive WHERE tenant_id=$1 AND id=$2`, p.Tenant, id)
}

func (p *Postgres) OpenByStage(ctx context.Context, processKey string) (map[string]int, error) {
	rows, err := p.pool.Query(ctx, `SELECT coalesce(data->>'stage',''), count(*) FROM cm_case
		WHERE tenant_id=$1 AND status='running' AND ($2='' OR process_key=$2) AND coalesce((data->>'test')::boolean,false)=false GROUP BY 1`, p.Tenant, processKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

package store

import (
	"context"
	"time"
)

func (p *Postgres) PutJob(ctx context.Context, j *JobRec) error {
	return p.exec(ctx, `INSERT INTO cm_job(tenant_id,id,case_id,instance_id,token_id,status,next_run_at,lease_until,data)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (tenant_id,id) DO UPDATE SET status=EXCLUDED.status, next_run_at=EXCLUDED.next_run_at,
		lease_until=EXCLUDED.lease_until, data=EXCLUDED.data`,
		p.Tenant, j.ID, j.CaseID, j.InstanceID, j.TokenID, j.Status, j.NextRunAt, j.LeaseUntil, js(j))
}
func (p *Postgres) GetJob(ctx context.Context, id string) (*JobRec, error) {
	return one[JobRec](ctx, p.q(ctx), `SELECT data FROM cm_job WHERE tenant_id=$1 AND id=$2`, p.Tenant, id)
}

// LeaseJobs claims due jobs (or jobs whose lease expired because a worker
// died) with SKIP LOCKED, so any number of workers can share the queue.
func (p *Postgres) LeaseJobs(ctx context.Context, worker string, now time.Time, lease time.Duration, limit int) ([]*JobRec, error) {
	until := now.Add(lease)
	return many[JobRec](ctx, p.q(ctx), `WITH due AS (
		  SELECT id FROM cm_job WHERE tenant_id=$1 AND next_run_at<=$2
		   AND (status='pending' OR (status='running' AND lease_until<$2))
		  ORDER BY next_run_at LIMIT $3 FOR UPDATE SKIP LOCKED)
		UPDATE cm_job j SET status='running', lease_until=$4,
		  data = j.data || jsonb_build_object('status','running','leaseUntil',to_jsonb($4::timestamptz),'leasedBy',$5::text)
		FROM due WHERE j.tenant_id=$1 AND j.id=due.id RETURNING j.data`, p.Tenant, now, limit, until, worker)
}
func (p *Postgres) DeleteJob(ctx context.Context, id string) error {
	return p.exec(ctx, `DELETE FROM cm_job WHERE tenant_id=$1 AND id=$2`, p.Tenant, id)
}
func (p *Postgres) DeleteJobsByToken(ctx context.Context, iid, tid string) error {
	return p.exec(ctx, `DELETE FROM cm_job WHERE tenant_id=$1 AND instance_id=$2 AND token_id=$3`, p.Tenant, iid, tid)
}
func (p *Postgres) ListJobs(ctx context.Context, caseID string) ([]*JobRec, error) {
	return many[JobRec](ctx, p.q(ctx), `SELECT data FROM cm_job WHERE tenant_id=$1 AND ($2='' OR case_id=$2) ORDER BY next_run_at LIMIT 1000`, p.Tenant, caseID)
}

func (p *Postgres) PutIncident(ctx context.Context, i *IncidentRec) error {
	return p.exec(ctx, `INSERT INTO cm_incident(tenant_id,id,case_id,status,created_at,data) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id,id) DO UPDATE SET status=EXCLUDED.status, data=EXCLUDED.data`,
		p.Tenant, i.ID, i.CaseID, i.Status, i.CreatedAt, js(i))
}
func (p *Postgres) GetIncident(ctx context.Context, id string) (*IncidentRec, error) {
	return one[IncidentRec](ctx, p.q(ctx), `SELECT data FROM cm_incident WHERE tenant_id=$1 AND id=$2`, p.Tenant, id)
}
func (p *Postgres) ListIncidents(ctx context.Context, st IncidentStatus, caseID string) ([]*IncidentRec, error) {
	return many[IncidentRec](ctx, p.q(ctx), `SELECT data FROM cm_incident WHERE tenant_id=$1 AND ($2='' OR status=$2)
		AND ($3='' OR case_id=$3) ORDER BY created_at DESC LIMIT 1000`, p.Tenant, st, caseID)
}

func (p *Postgres) AppendEvents(ctx context.Context, ev []EventRec) error {
	if len(ev) == 0 {
		return nil
	}
	// one round trip for the whole batch
	return p.exec(ctx, `INSERT INTO cm_event(tenant_id,case_id,at,type,node,data)
		SELECT $1, e->>'caseId', (e->>'at')::timestamptz, e->>'type', NULLIF(e->>'node',''), e
		FROM jsonb_array_elements($2::jsonb) e`, p.Tenant, js(ev))
}

func (p *Postgres) ListEvents(ctx context.Context, caseID string, limit int) ([]*EventRec, error) {
	if limit <= 0 {
		limit = 5000
	}
	rows, err := many[EventRec](ctx, p.q(ctx), `SELECT data || jsonb_build_object('seq',seq) FROM (
		SELECT seq, data FROM cm_event WHERE tenant_id=$1 AND ($2='' OR case_id=$2) ORDER BY seq DESC LIMIT $3) x ORDER BY seq`,
		p.Tenant, caseID, limit)
	return rows, err
}

func (p *Postgres) ReplaceSubscriptions(ctx context.Context, caseID string, subs []Subscription) error {
	if subs == nil {
		subs = []Subscription{}
	}
	return p.exec(ctx, `WITH d AS (DELETE FROM cm_subscription WHERE tenant_id=$1 AND case_id=$2)
		INSERT INTO cm_subscription(tenant_id,case_id,token_id,kind,name)
		SELECT DISTINCT $1, $2, s->>'tokenId', s->>'kind', s->>'name' FROM jsonb_array_elements($3::jsonb) s
		ON CONFLICT DO NOTHING`, p.Tenant, caseID, js(subs))
}

func (p *Postgres) FindSubscriptions(ctx context.Context, kind, name string) ([]Subscription, error) {
	rows, err := p.q(ctx).Query(ctx, `SELECT case_id, token_id, kind, name FROM cm_subscription WHERE tenant_id=$1 AND kind=$2 AND name=$3`, p.Tenant, kind, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.CaseID, &s.TokenID, &s.Kind, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

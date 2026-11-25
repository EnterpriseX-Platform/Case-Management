package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	pool   *pgxpool.Pool
	Tenant string
}

func NewPostgres(ctx context.Context, url, tenant string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}
	if tenant == "" {
		tenant = "default"
	}
	return &Postgres{pool: pool, Tenant: tenant}, nil
}

func (p *Postgres) Migrate(ctx context.Context, sql string) error {
	_, err := p.pool.Exec(ctx, sql)
	return err
}

type txKey struct{}

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (p *Postgres) q(ctx context.Context) querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return p.pool
}

func (p *Postgres) Tx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return fn(ctx)
	}
	return pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

func js(v any) []byte { b, _ := json.Marshal(v); return b }

func one[T any](ctx context.Context, q querier, sql string, args ...any) (*T, error) {
	var raw []byte
	if err := q.QueryRow(ctx, sql, args...).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var v T
	return &v, json.Unmarshal(raw, &v)
}

func many[T any](ctx context.Context, q querier, sql string, args ...any) ([]*T, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*T
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

func (p *Postgres) exec(ctx context.Context, sql string, args ...any) error {
	_, err := p.q(ctx).Exec(ctx, sql, args...)
	return err
}

func (p *Postgres) PutProcess(ctx context.Context, d *ProcessDef) error {
	return p.exec(ctx, `INSERT INTO cm_process(tenant_id,key,data) VALUES($1,$2,$3)
		ON CONFLICT (tenant_id,key) DO UPDATE SET data=EXCLUDED.data`, p.Tenant, d.Key, js(d))
}
func (p *Postgres) GetProcess(ctx context.Context, key string) (*ProcessDef, error) {
	return one[ProcessDef](ctx, p.q(ctx), `SELECT data FROM cm_process WHERE tenant_id=$1 AND key=$2`, p.Tenant, key)
}
func (p *Postgres) ListProcesses(ctx context.Context) ([]*ProcessDef, error) {
	return many[ProcessDef](ctx, p.q(ctx), `SELECT data FROM cm_process WHERE tenant_id=$1 ORDER BY key`, p.Tenant)
}

func (p *Postgres) PutVersion(ctx context.Context, v *Version) error {
	return p.exec(ctx, `INSERT INTO cm_version(tenant_id,id,process_key,no,status,fiscal_year,checksum,data)
		VALUES($1,$2,$3,$4,$5,NULLIF($6,0),$7,$8)
		ON CONFLICT (tenant_id,id) DO UPDATE SET status=EXCLUDED.status, fiscal_year=EXCLUDED.fiscal_year,
		checksum=EXCLUDED.checksum, data=EXCLUDED.data`,
		p.Tenant, v.ID, v.ProcessKey, v.No, v.Status, v.FiscalYear, v.Checksum, js(v))
}
func (p *Postgres) GetVersion(ctx context.Context, id string) (*Version, error) {
	return one[Version](ctx, p.q(ctx), `SELECT data FROM cm_version WHERE tenant_id=$1 AND id=$2`, p.Tenant, id)
}
func (p *Postgres) ListVersions(ctx context.Context, key string) ([]*Version, error) {
	return many[Version](ctx, p.q(ctx), `SELECT data FROM cm_version WHERE tenant_id=$1 AND ($2='' OR process_key=$2) ORDER BY no DESC`, p.Tenant, key)
}

func (p *Postgres) PutBindings(ctx context.Context, vid string, b []Binding) error {
	return p.exec(ctx, `INSERT INTO cm_binding(tenant_id,version_id,data) VALUES($1,$2,$3)
		ON CONFLICT (tenant_id,version_id) DO UPDATE SET data=EXCLUDED.data`, p.Tenant, vid, js(b))
}
func (p *Postgres) GetBindings(ctx context.Context, vid string) ([]Binding, error) {
	v, err := one[[]Binding](ctx, p.q(ctx), `SELECT data FROM cm_binding WHERE tenant_id=$1 AND version_id=$2`, p.Tenant, vid)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return *v, nil
}

func (p *Postgres) PutForm(ctx context.Context, f *Form) error {
	return p.exec(ctx, `INSERT INTO cm_form(tenant_id,key,data) VALUES($1,$2,$3)
		ON CONFLICT (tenant_id,key) DO UPDATE SET data=EXCLUDED.data`, p.Tenant, f.Key, js(f))
}
func (p *Postgres) GetForm(ctx context.Context, key string) (*Form, error) {
	return one[Form](ctx, p.q(ctx), `SELECT data FROM cm_form WHERE tenant_id=$1 AND key=$2`, p.Tenant, key)
}
func (p *Postgres) ListForms(ctx context.Context) ([]*Form, error) {
	return many[Form](ctx, p.q(ctx), `SELECT data FROM cm_form WHERE tenant_id=$1 ORDER BY key`, p.Tenant)
}

func (p *Postgres) PutScreen(ctx context.Context, s *Screen) error {
	return p.exec(ctx, `INSERT INTO cm_screen(tenant_id,key,data) VALUES($1,$2,$3)
		ON CONFLICT (tenant_id,key) DO UPDATE SET data=EXCLUDED.data`, p.Tenant, s.Key, js(s))
}
func (p *Postgres) GetScreen(ctx context.Context, key string) (*Screen, error) {
	return one[Screen](ctx, p.q(ctx), `SELECT data FROM cm_screen WHERE tenant_id=$1 AND key=$2`, p.Tenant, key)
}
func (p *Postgres) ListScreens(ctx context.Context) ([]*Screen, error) {
	return many[Screen](ctx, p.q(ctx), `SELECT data FROM cm_screen WHERE tenant_id=$1 ORDER BY key`, p.Tenant)
}

func (p *Postgres) PutCase(ctx context.Context, c *Case) error {
	return p.exec(ctx, `INSERT INTO cm_case(tenant_id,id,number,process_key,version_id,fiscal_year,parent_id,status,created_at,data)
		VALUES($1,$2,$3,$4,$5,NULLIF($6,0),NULLIF($7,''),$8,$9,$10)
		ON CONFLICT (tenant_id,id) DO UPDATE SET version_id=EXCLUDED.version_id, status=EXCLUDED.status, data=EXCLUDED.data`,
		p.Tenant, c.ID, c.Number, c.ProcessKey, c.VersionID, c.FiscalYear, c.ParentID, c.Status, c.CreatedAt, js(c))
}
func (p *Postgres) GetCase(ctx context.Context, id string) (*Case, error) {
	return one[Case](ctx, p.q(ctx), `SELECT data FROM cm_case WHERE tenant_id=$1 AND id=$2`, p.Tenant, id)
}
func (p *Postgres) ListCases(ctx context.Context, key, vid string, limit int) ([]*Case, error) {
	if limit <= 0 {
		limit = 500
	}
	return many[Case](ctx, p.q(ctx), `SELECT data FROM cm_case WHERE tenant_id=$1 AND ($2='' OR process_key=$2)
		AND ($3='' OR version_id=$3) ORDER BY created_at DESC LIMIT $4`, p.Tenant, key, vid, limit)
}
func (p *Postgres) NextCaseNumber(ctx context.Context, prefix string) (int, error) {
	var n int
	err := p.q(ctx).QueryRow(ctx, `INSERT INTO cm_case_seq(tenant_id,prefix,value) VALUES($1,$2,1)
		ON CONFLICT (tenant_id,prefix) DO UPDATE SET value=cm_case_seq.value+1 RETURNING value`, p.Tenant, prefix).Scan(&n)
	return n, err
}

func (p *Postgres) PutWork(ctx context.Context, w *Work) error {
	return p.exec(ctx, `INSERT INTO cm_work(tenant_id,id,case_id,role,assignee,status,due_at,created_at,data)
		VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9)
		ON CONFLICT (tenant_id,id) DO UPDATE SET role=EXCLUDED.role, assignee=EXCLUDED.assignee,
		status=EXCLUDED.status, due_at=EXCLUDED.due_at, data=EXCLUDED.data`,
		p.Tenant, w.ID, w.CaseID, w.Role, w.Assignee, w.Status, w.DueAt, w.CreatedAt, js(w))
}
func (p *Postgres) GetWork(ctx context.Context, id string) (*Work, error) {
	return one[Work](ctx, p.q(ctx), `SELECT data FROM cm_work WHERE tenant_id=$1 AND id=$2`, p.Tenant, id)
}
func (p *Postgres) ListWork(ctx context.Context, f WorkFilter) ([]*Work, error) {
	st := make([]string, len(f.Status))
	for i, s := range f.Status {
		st[i] = string(s)
	}
	all := f.Assignee == "" && len(f.Roles) == 0
	star := false
	for _, r := range f.Roles {
		star = star || r == "*"
	}
	return many[Work](ctx, p.q(ctx), `SELECT data FROM cm_work WHERE tenant_id=$1
		AND ($2='' OR case_id=$2)
		AND (cardinality($3::text[])=0 OR status=ANY($3))
		AND ($4 OR ($5<>'' AND assignee=$5) OR (assignee IS NULL AND ($6 OR role=ANY($7))))
		ORDER BY created_at`, p.Tenant, f.CaseID, st, all, f.Assignee, star, f.Roles)
}

func (p *Postgres) PutTimer(ctx context.Context, t *engine.Timer) error {
	return p.exec(ctx, `INSERT INTO cm_timer(tenant_id,id,instance_id,token_id,due_at,data) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (tenant_id,id) DO UPDATE SET due_at=EXCLUDED.due_at, data=EXCLUDED.data`,
		p.Tenant, t.ID, t.InstanceID, t.TokenID, t.DueAt, js(t))
}
func (p *Postgres) DeleteTimersByToken(ctx context.Context, iid, tid string) error {
	return p.exec(ctx, `DELETE FROM cm_timer WHERE tenant_id=$1 AND instance_id=$2 AND token_id=$3`, p.Tenant, iid, tid)
}
func (p *Postgres) DueTimers(ctx context.Context, now time.Time, limit int) ([]*engine.Timer, error) {
	// SKIP LOCKED lets several server replicas share the timer queue.
	return many[engine.Timer](ctx, p.q(ctx), `SELECT data FROM cm_timer WHERE tenant_id=$1 AND due_at<=$2
		ORDER BY due_at LIMIT $3 FOR UPDATE SKIP LOCKED`, p.Tenant, now, limit)
}
func (p *Postgres) DeleteTimer(ctx context.Context, id string) error {
	return p.exec(ctx, `DELETE FROM cm_timer WHERE tenant_id=$1 AND id=$2`, p.Tenant, id)
}

func (p *Postgres) AddAudit(ctx context.Context, a *Audit) error {
	return p.exec(ctx, `INSERT INTO cm_audit(tenant_id,at,ref,data) VALUES($1,$2,$3,$4)`, p.Tenant, a.At, a.Ref, js(a))
}
func (p *Postgres) ListAudit(ctx context.Context, ref string, limit int) ([]*Audit, error) {
	if limit <= 0 {
		limit = 200
	}
	return many[Audit](ctx, p.q(ctx), `SELECT data FROM cm_audit WHERE tenant_id=$1 AND ($2='' OR ref=$2)
		ORDER BY seq DESC LIMIT $3`, p.Tenant, ref, limit)
}

func (p *Postgres) PutImport(ctx context.Context, j *ImportJob) error {
	return p.exec(ctx, `INSERT INTO cm_import(tenant_id,id,created_at,data) VALUES($1,$2,$3,$4)
		ON CONFLICT (tenant_id,id) DO UPDATE SET data=EXCLUDED.data`, p.Tenant, j.ID, j.CreatedAt, js(j))
}
func (p *Postgres) ListImports(ctx context.Context) ([]*ImportJob, error) {
	return many[ImportJob](ctx, p.q(ctx), `SELECT data FROM cm_import WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 200`, p.Tenant)
}

var _ Store = (*Postgres)(nil)
var _ Store = (*Memory)(nil)
var _ = fmt.Sprintf

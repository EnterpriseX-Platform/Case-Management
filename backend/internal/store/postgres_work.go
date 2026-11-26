package store

import (
	"context"
	"errors"
	"time"
)

func (p *Postgres) putDoc(ctx context.Context, kind, key string, v any) error {
	return p.exec(ctx, `INSERT INTO cm_doc(tenant_id,kind,key,data) VALUES($1,$2,$3,$4)
		ON CONFLICT (tenant_id,kind,key) DO UPDATE SET data=EXCLUDED.data`, p.Tenant, kind, key, js(v))
}

func (p *Postgres) PutUser(ctx context.Context, u *UserProfile) error {
	return p.putDoc(ctx, "user", u.ID, u)
}
func (p *Postgres) GetUser(ctx context.Context, id string) (*UserProfile, error) {
	return one[UserProfile](ctx, p.q(ctx), `SELECT data FROM cm_doc WHERE tenant_id=$1 AND kind='user' AND key=$2`, p.Tenant, id)
}
func (p *Postgres) ListUsers(ctx context.Context) ([]*UserProfile, error) {
	return many[UserProfile](ctx, p.q(ctx), `SELECT data FROM cm_doc WHERE tenant_id=$1 AND kind='user' ORDER BY key`, p.Tenant)
}
func (p *Postgres) PutQueue(ctx context.Context, q *Queue) error {
	return p.putDoc(ctx, "queue", q.Key, q)
}
func (p *Postgres) GetQueue(ctx context.Context, k string) (*Queue, error) {
	return one[Queue](ctx, p.q(ctx), `SELECT data FROM cm_doc WHERE tenant_id=$1 AND kind='queue' AND key=$2`, p.Tenant, k)
}
func (p *Postgres) ListQueues(ctx context.Context) ([]*Queue, error) {
	return many[Queue](ctx, p.q(ctx), `SELECT data FROM cm_doc WHERE tenant_id=$1 AND kind='queue' ORDER BY key`, p.Tenant)
}
func (p *Postgres) DeleteQueue(ctx context.Context, k string) error {
	return p.exec(ctx, `DELETE FROM cm_doc WHERE tenant_id=$1 AND kind='queue' AND key=$2`, p.Tenant, k)
}
func (p *Postgres) GetSettings(ctx context.Context) (*Settings, error) {
	s, err := one[Settings](ctx, p.q(ctx), `SELECT data FROM cm_doc WHERE tenant_id=$1 AND kind='settings' AND key='main'`, p.Tenant)
	if errors.Is(err, ErrNotFound) {
		return &Settings{}, nil
	}
	return s, err
}
func (p *Postgres) PutSettings(ctx context.Context, s *Settings) error {
	return p.putDoc(ctx, "settings", "main", s)
}

func (p *Postgres) AddNotification(ctx context.Context, n *Notification) error {
	return p.exec(ctx, `INSERT INTO cm_notification(tenant_id,id,usr,at,read,data) VALUES($1,$2,$3,$4,$5,$6)`,
		p.Tenant, n.ID, n.User, n.At, n.Read, js(n))
}
func (p *Postgres) ListNotifications(ctx context.Context, user string, limit int) ([]*Notification, error) {
	if limit <= 0 {
		limit = 50
	}
	return many[Notification](ctx, p.q(ctx), `SELECT data || jsonb_build_object('read', read) FROM cm_notification
		WHERE tenant_id=$1 AND usr=$2 ORDER BY at DESC LIMIT $3`, p.Tenant, user, limit)
}
func (p *Postgres) MarkNotificationsRead(ctx context.Context, user string, ids []string) error {
	return p.exec(ctx, `UPDATE cm_notification SET read=true WHERE tenant_id=$1 AND usr=$2 AND (cardinality($3::text[])=0 OR id=ANY($3))`,
		p.Tenant, user, ids)
}
func (p *Postgres) SLADue(ctx context.Context, now time.Time, limit int) ([]*Work, error) {
	return many[Work](ctx, p.q(ctx), `SELECT data FROM cm_work WHERE tenant_id=$1 AND sla_open
		AND ((goal_at<=$2 AND NOT coalesce((data->>'goalDone')::boolean,false))
		  OR (due_at<=$2 AND NOT coalesce((data->>'deadlineDone')::boolean,false)))
		ORDER BY LEAST(goal_at, due_at) LIMIT $3`, p.Tenant, now, limit)
}
func (p *Postgres) OpenCounts(ctx context.Context) (map[string]int, error) {
	rows, err := p.q(ctx).Query(ctx, `SELECT assignee, count(*) FROM cm_work WHERE tenant_id=$1 AND assignee IS NOT NULL
		AND status IN ('open','claimed') GROUP BY assignee`, p.Tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var a string
		var n int
		if err := rows.Scan(&a, &n); err != nil {
			return nil, err
		}
		out[a] = n
	}
	return out, rows.Err()
}

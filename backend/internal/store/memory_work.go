package store

import (
	"context"
	"sort"
	"time"
)

type memWork struct {
	users    map[string]*UserProfile
	queues   map[string]*Queue
	settings *Settings
	notes    []*Notification
	outbox   map[string]*OutboxMsg
	views    map[string]*WorkView
	hooks    map[string]*Webhook
	rules    map[string]*AssignRule
}

func (m *Memory) wk() *memWork {
	if m.w == nil {
		m.w = &memWork{users: map[string]*UserProfile{}, queues: map[string]*Queue{}}
	}
	return m.w
}

func (m *Memory) PutUser(_ context.Context, u *UserProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wk().users[u.ID] = cp(u)
	return nil
}
func (m *Memory) GetUser(_ context.Context, id string) (*UserProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.wk().users[id]; ok {
		return cp(u), nil
	}
	return nil, ErrNotFound
}
func (m *Memory) ListUsers(context.Context) ([]*UserProfile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*UserProfile
	for _, u := range m.wk().users {
		out = append(out, cp(u))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (m *Memory) PutQueue(_ context.Context, q *Queue) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wk().queues[q.Key] = cp(q)
	return nil
}
func (m *Memory) GetQueue(_ context.Context, k string) (*Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if q, ok := m.wk().queues[k]; ok {
		return cp(q), nil
	}
	return nil, ErrNotFound
}
func (m *Memory) ListQueues(context.Context) ([]*Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Queue
	for _, q := range m.wk().queues {
		out = append(out, cp(q))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}
func (m *Memory) DeleteQueue(_ context.Context, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wk().queues, k)
	return nil
}
func (m *Memory) GetSettings(context.Context) (*Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.wk().settings == nil {
		return &Settings{}, nil
	}
	return cp(m.wk().settings), nil
}
func (m *Memory) PutSettings(_ context.Context, s *Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wk().settings = cp(s)
	return nil
}
func (m *Memory) AddNotification(_ context.Context, n *Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wk().notes = append(m.wk().notes, cp(n))
	return nil
}
func (m *Memory) ListNotifications(_ context.Context, user string, limit int) ([]*Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Notification
	ns := m.wk().notes
	for i := len(ns) - 1; i >= 0 && (limit <= 0 || len(out) < limit); i-- {
		if ns[i].User == user {
			out = append(out, cp(ns[i]))
		}
	}
	return out, nil
}
func (m *Memory) MarkNotificationsRead(_ context.Context, user string, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	all := len(ids) == 0
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	for _, n := range m.wk().notes {
		if n.User == user && (all || set[n.ID]) {
			n.Read = true
		}
	}
	return nil
}
func (m *Memory) SLADue(_ context.Context, now time.Time, limit int) ([]*Work, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Work
	for _, w := range m.work {
		if w.Status != WorkOpen && w.Status != WorkClaimed {
			continue
		}
		if (w.GoalAt != nil && !w.GoalDone && !w.GoalAt.After(now)) || (w.DueAt != nil && !w.DeadlineDone && !w.DueAt.After(now)) {
			out = append(out, cp(w))
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
func (m *Memory) OpenCounts(context.Context) (map[string]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int{}
	for _, w := range m.work {
		if w.Assignee != "" && (w.Status == WorkOpen || w.Status == WorkClaimed) {
			out[w.Assignee]++
		}
	}
	return out, nil
}

func (m *Memory) PutOutbox(_ context.Context, o *OutboxMsg) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.wk().outbox == nil {
		m.wk().outbox = map[string]*OutboxMsg{}
	}
	m.wk().outbox[o.ID] = cp(o)
	return nil
}
func (m *Memory) LeaseOutbox(_ context.Context, now time.Time, lease time.Duration, limit int) ([]*OutboxMsg, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*OutboxMsg
	for _, o := range m.wk().outbox {
		if len(out) >= limit {
			break
		}
		free := o.Status == "pending" || (o.Status == "sending" && o.LeaseUntil != nil && o.LeaseUntil.Before(now))
		if free && !o.NextRunAt.After(now) {
			until := now.Add(lease)
			o.Status, o.LeaseUntil = "sending", &until
			out = append(out, cp(o))
		}
	}
	return out, nil
}
func (m *Memory) DeleteOutbox(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wk().outbox, id)
	return nil
}

func (m *Memory) PutView(_ context.Context, v *WorkView) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.wk().views == nil {
		m.wk().views = map[string]*WorkView{}
	}
	m.wk().views[v.ID] = cp(v)
	return nil
}
func (m *Memory) ListViews(context.Context) ([]*WorkView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*WorkView
	for _, v := range m.wk().views {
		out = append(out, cp(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (m *Memory) DeleteView(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wk().views, id)
	return nil
}

func (m *Memory) PutWebhook(_ context.Context, h *Webhook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.wk().hooks == nil {
		m.wk().hooks = map[string]*Webhook{}
	}
	m.wk().hooks[h.ID] = cp(h)
	return nil
}
func (m *Memory) ListWebhooks(context.Context) ([]*Webhook, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Webhook
	for _, h := range m.wk().hooks {
		out = append(out, cp(h))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
func (m *Memory) DeleteWebhook(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wk().hooks, id)
	return nil
}
func (m *Memory) ListOutbox(_ context.Context, kind, status string, limit int) ([]*OutboxMsg, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*OutboxMsg
	for _, o := range m.wk().outbox {
		if (kind == "" || o.Kind == kind) && (status == "" || o.Status == status) {
			out = append(out, cp(o))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NextRunAt.After(out[j].NextRunAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) PutAssignRule(_ context.Context, r *AssignRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.wk().rules == nil {
		m.wk().rules = map[string]*AssignRule{}
	}
	m.wk().rules[r.ID] = cp(r)
	return nil
}
func (m *Memory) ListAssignRules(context.Context) ([]*AssignRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*AssignRule
	for _, r := range m.wk().rules {
		out = append(out, cp(r))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
func (m *Memory) DeleteAssignRule(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.wk().rules, id)
	return nil
}

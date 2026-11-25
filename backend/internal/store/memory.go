package store

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
)

// Memory is an in-process store for development and tests. Values are deep
// copied through JSON so callers never share pointers with the store.
type Memory struct {
	mu       sync.Mutex
	txMu     sync.Mutex
	procs    map[string]*ProcessDef
	versions map[string]*Version
	binds    map[string][]Binding
	forms    map[string]*Form
	screens  map[string]*Screen
	cases    map[string]*Case
	work     map[string]*Work
	timers   map[string]*engine.Timer
	audit    []*Audit
	imports  []*ImportJob
	seq      map[string]int
}

func NewMemory() *Memory {
	return &Memory{procs: map[string]*ProcessDef{}, versions: map[string]*Version{}, binds: map[string][]Binding{},
		forms: map[string]*Form{}, screens: map[string]*Screen{}, cases: map[string]*Case{},
		work: map[string]*Work{}, timers: map[string]*engine.Timer{}, seq: map[string]int{}}
}

func cp[T any](v T) T {
	var out T
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, &out)
	return out
}

type memTxKey struct{}

// Tx serialises writers. It is re-entrant through ctx; it does not roll back.
func (m *Memory) Tx(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx.Value(memTxKey{}) == m {
		return fn(ctx)
	}
	m.txMu.Lock()
	defer m.txMu.Unlock()
	return fn(context.WithValue(ctx, memTxKey{}, m))
}

func (m *Memory) PutProcess(_ context.Context, p *ProcessDef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.procs[p.Key] = cp(p)
	return nil
}
func (m *Memory) GetProcess(_ context.Context, key string) (*ProcessDef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.procs[key]; ok {
		return cp(p), nil
	}
	return nil, ErrNotFound
}
func (m *Memory) ListProcesses(context.Context) ([]*ProcessDef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*ProcessDef
	for _, p := range m.procs {
		out = append(out, cp(p))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (m *Memory) PutVersion(_ context.Context, v *Version) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.versions[v.ID] = cp(v)
	return nil
}
func (m *Memory) GetVersion(_ context.Context, id string) (*Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.versions[id]; ok {
		return cp(v), nil
	}
	return nil, ErrNotFound
}
func (m *Memory) ListVersions(_ context.Context, key string) ([]*Version, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Version
	for _, v := range m.versions {
		if key == "" || v.ProcessKey == key {
			out = append(out, cp(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].No > out[j].No })
	return out, nil
}

func (m *Memory) PutBindings(_ context.Context, vid string, b []Binding) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.binds[vid] = cp(b)
	return nil
}
func (m *Memory) GetBindings(_ context.Context, vid string) ([]Binding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cp(m.binds[vid]), nil
}

func (m *Memory) PutForm(_ context.Context, f *Form) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forms[f.Key] = cp(f)
	return nil
}
func (m *Memory) GetForm(_ context.Context, key string) (*Form, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.forms[key]; ok {
		return cp(f), nil
	}
	return nil, ErrNotFound
}
func (m *Memory) ListForms(context.Context) ([]*Form, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Form
	for _, f := range m.forms {
		out = append(out, cp(f))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (m *Memory) PutScreen(_ context.Context, s *Screen) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.screens[s.Key] = cp(s)
	return nil
}
func (m *Memory) GetScreen(_ context.Context, key string) (*Screen, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.screens[key]; ok {
		return cp(s), nil
	}
	return nil, ErrNotFound
}
func (m *Memory) ListScreens(context.Context) ([]*Screen, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Screen
	for _, s := range m.screens {
		out = append(out, cp(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (m *Memory) PutCase(_ context.Context, c *Case) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cases[c.ID] = cp(c)
	return nil
}
func (m *Memory) GetCase(_ context.Context, id string) (*Case, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cases[id]; ok {
		return cp(c), nil
	}
	return nil, ErrNotFound
}
func (m *Memory) ListCases(_ context.Context, key, vid string, limit int) ([]*Case, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Case
	for _, c := range m.cases {
		if (key == "" || c.ProcessKey == key) && (vid == "" || c.VersionID == vid) {
			out = append(out, cp(c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *Memory) NextCaseNumber(_ context.Context, prefix string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq[prefix]++
	return m.seq[prefix], nil
}

func (m *Memory) PutWork(_ context.Context, w *Work) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.work[w.ID] = cp(w)
	return nil
}
func (m *Memory) GetWork(_ context.Context, id string) (*Work, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.work[id]; ok {
		return cp(w), nil
	}
	return nil, ErrNotFound
}
func (m *Memory) ListWork(_ context.Context, f WorkFilter) ([]*Work, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Work
	for _, w := range m.work {
		if matchWork(w, f) {
			out = append(out, cp(w))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func matchWork(w *Work, f WorkFilter) bool {
	if f.CaseID != "" && w.CaseID != f.CaseID {
		return false
	}
	if len(f.Status) > 0 {
		ok := false
		for _, s := range f.Status {
			ok = ok || w.Status == s
		}
		if !ok {
			return false
		}
	}
	if f.Assignee == "" && len(f.Roles) == 0 {
		return true
	}
	if f.Assignee != "" && w.Assignee == f.Assignee {
		return true
	}
	if w.Assignee == "" {
		for _, r := range f.Roles {
			if r == w.Role || r == "*" {
				return true
			}
		}
	}
	return false
}

func (m *Memory) PutTimer(_ context.Context, t *engine.Timer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timers[t.ID] = cp(t)
	return nil
}
func (m *Memory) DeleteTimersByToken(_ context.Context, iid, tid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.timers {
		if t.InstanceID == iid && t.TokenID == tid {
			delete(m.timers, id)
		}
	}
	return nil
}
func (m *Memory) DueTimers(_ context.Context, now time.Time, limit int) ([]*engine.Timer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*engine.Timer
	for _, t := range m.timers {
		if !t.DueAt.After(now) {
			out = append(out, cp(t))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DueAt.Before(out[j].DueAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (m *Memory) DeleteTimer(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.timers, id)
	return nil
}

func (m *Memory) AddAudit(_ context.Context, a *Audit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, cp(a))
	return nil
}
func (m *Memory) ListAudit(_ context.Context, ref string, limit int) ([]*Audit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Audit
	for i := len(m.audit) - 1; i >= 0 && (limit <= 0 || len(out) < limit); i-- {
		if ref == "" || m.audit[i].Ref == ref {
			out = append(out, cp(m.audit[i]))
		}
	}
	return out, nil
}

func (m *Memory) PutImport(_ context.Context, j *ImportJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, x := range m.imports {
		if x.ID == j.ID {
			m.imports[i] = cp(j)
			return nil
		}
	}
	m.imports = append(m.imports, cp(j))
	return nil
}
func (m *Memory) ListImports(context.Context) ([]*ImportJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cp(m.imports), nil
}

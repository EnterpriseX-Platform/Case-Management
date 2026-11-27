package app

// Work distribution rules: who gets new work, decided by rules supervisors
// build without code. Rules are checked in order; the first that matches
// sets the assignee (or queue / role), can cap open items per person, add
// urgency and replace the SLA. Steps no rule matches keep the routing set
// in the studio.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

var assignModes = map[string]bool{"keep": true, "role": true, "queue": true, "user": true, "workload": true, "roundrobin": true, "skill": true}

type ruleCache struct {
	mu    sync.Mutex
	at    time.Time
	rules []*store.AssignRule
}

func (s *Service) assignRules(ctx context.Context) []*store.AssignRule {
	s.rules.mu.Lock()
	defer s.rules.mu.Unlock()
	if time.Since(s.rules.at) > 10*time.Second {
		if rs, err := s.S.ListAssignRules(ctx); err == nil {
			s.rules.rules, s.rules.at = rs, time.Now()
		}
	}
	return s.rules.rules
}

func (s *Service) flushRules() {
	s.rules.mu.Lock()
	s.rules.at = time.Time{}
	s.rules.mu.Unlock()
}

// ruleFacts are the values a rule condition can test.
type ruleFacts struct {
	CaseType, Step, StepName, Stage, Role string
	Urgency                               int
	Data                                  map[string]any
}

func (f ruleFacts) get(field string) any {
	switch field {
	case "caseType":
		return f.CaseType
	case "step":
		return f.Step
	case "stepName":
		return f.StepName
	case "stage":
		return f.Stage
	case "role":
		return f.Role
	case "urgency":
		return float64(f.Urgency)
	}
	if strings.HasPrefix(field, "data.") {
		return f.Data[strings.TrimPrefix(field, "data.")]
	}
	return nil
}

// testRule evaluates one condition row (same operators as the screens).
func testRule(r store.ViewRule, f ruleFacts) bool {
	v := f.get(r.Field)
	empty := v == nil || fmt.Sprint(v) == ""
	switch r.Op {
	case "empty":
		return empty
	case "filled":
		return !empty
	case "contains":
		return strings.Contains(strings.ToLower(fmt.Sprint(v)), strings.ToLower(r.Value))
	case "in":
		for _, x := range strings.Split(r.Value, ",") {
			if strings.TrimSpace(x) == fmt.Sprint(v) {
				return true
			}
		}
		return false
	case "gt", "gte", "lt", "lte":
		a, err1 := strconv.ParseFloat(fmt.Sprint(v), 64)
		b, err2 := strconv.ParseFloat(r.Value, 64)
		if empty || err1 != nil || err2 != nil {
			return false
		}
		switch r.Op {
		case "gt":
			return a > b
		case "gte":
			return a >= b
		case "lt":
			return a < b
		}
		return a <= b
	case "ne":
		return fmt.Sprint(v) != r.Value && !(v == nil && r.Value == "")
	default: // eq
		if b, ok := v.(bool); ok {
			return strconv.FormatBool(b) == r.Value
		}
		if n, ok := v.(float64); ok {
			if w, err := strconv.ParseFloat(r.Value, 64); err == nil {
				return n == w
			}
		}
		return fmt.Sprint(v) == r.Value
	}
}

func ruleMatches(r *store.AssignRule, f ruleFacts) bool {
	if len(r.When) == 0 {
		return true // a rule without conditions catches everything (put it last)
	}
	for _, w := range r.When {
		ok := testRule(w, f)
		if r.Any && ok {
			return true
		}
		if !r.Any && !ok {
			return false
		}
	}
	return !r.Any
}

func firstRule(rules []*store.AssignRule, f ruleFacts) *store.AssignRule {
	for _, r := range rules {
		if r.Active && ruleMatches(r, f) {
			return r
		}
	}
	return nil
}

// applyRule changes a new work item by the matching rule. It returns the
// routing to use (nil = keep the step's own routing).
func (s *Service) applyRule(ctx context.Context, w *store.Work, c *store.Case, vars map[string]any, now time.Time) (*store.Routing, int) {
	f := ruleFacts{CaseType: c.ProcessKey, Step: w.Node, StepName: w.Name, Stage: c.Stage, Role: w.Role, Urgency: w.Urgency, Data: vars}
	r := firstRule(s.assignRules(ctx), f)
	if r == nil {
		return nil, 0
	}
	w.AssignedBy = r.Name
	a := r.Then
	w.Urgency += a.Urgency
	cal := s.calendar(ctx)
	if a.SLA != "" {
		if d, err := engine.ParseDuration(a.SLA); err == nil {
			due := cal.Add(now, d)
			w.SLA, w.DueAt = a.SLA, &due
		}
	}
	if a.Goal != "" {
		if d, err := engine.ParseDuration(a.Goal); err == nil {
			g := cal.Add(now, d)
			w.GoalAt = &g
		}
	}
	switch a.Mode {
	case "keep", "":
		return nil, a.MaxPerPerson
	case "role":
		w.Role, w.Queue, w.Assignee = a.Role, "", ""
		return &store.Routing{Mode: "role"}, a.MaxPerPerson
	default:
		return &store.Routing{Mode: a.Mode, Queue: a.Queue, User: a.User, Skill: a.Skill}, a.MaxPerPerson
	}
}

// ---- admin

func canManageRules(by Actor) bool { return by.Has(RoleSupervisor) || by.Has(RoleAdmin) }

func (s *Service) AssignRules(ctx context.Context, by Actor) ([]*store.AssignRule, error) {
	if !canManageRules(by) {
		return nil, ErrForbidden
	}
	rs, err := s.S.ListAssignRules(ctx)
	if rs == nil {
		rs = []*store.AssignRule{}
	}
	return rs, err
}

func (s *Service) SaveAssignRule(ctx context.Context, by Actor, r *store.AssignRule) (*store.AssignRule, error) {
	if !canManageRules(by) {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(r.Name) == "" {
		return nil, invalid("a rule needs a name")
	}
	if !assignModes[r.Then.Mode] {
		return nil, invalid("choose who gets the work")
	}
	switch r.Then.Mode {
	case "queue", "workload", "roundrobin":
		if r.Then.Queue == "" {
			return nil, invalid("choose a work queue")
		}
	case "skill":
		if r.Then.Queue == "" || r.Then.Skill == "" {
			return nil, invalid("choose a work queue and a skill")
		}
	case "user":
		if r.Then.User == "" {
			return nil, invalid("choose a person")
		}
	case "role":
		if r.Then.Role == "" {
			return nil, invalid("choose a role")
		}
	}
	for _, d := range []string{r.Then.SLA, r.Then.Goal} {
		if d != "" {
			if _, err := engine.ParseDuration(d); err != nil {
				return nil, invalid("durations look like P3D or PT4H")
			}
		}
	}
	if r.Then.MaxPerPerson < 0 || r.Then.Urgency < -100 || r.Then.Urgency > 100 {
		return nil, invalid("cap must be 0 or more and urgency between -100 and 100")
	}
	if r.ID == "" {
		r.ID = id.New()
		if r.Order == 0 {
			rs, _ := s.S.ListAssignRules(ctx)
			r.Order = len(rs) + 1
		}
	}
	r.UpdatedAt = s.Now()
	if err := s.S.PutAssignRule(ctx, r); err != nil {
		return nil, err
	}
	s.flushRules()
	s.audit(ctx, by, "assignrule.save", "assignrule", r.ID, map[string]any{"name": r.Name, "then": r.Then, "when": r.When})
	return r, nil
}

func (s *Service) DeleteAssignRule(ctx context.Context, by Actor, ruleID string) error {
	if !canManageRules(by) {
		return ErrForbidden
	}
	s.flushRules()
	s.audit(ctx, by, "assignrule.delete", "assignrule", ruleID, nil)
	return s.S.DeleteAssignRule(ctx, ruleID)
}

// OrderAssignRules sets the order rules are checked in.
func (s *Service) OrderAssignRules(ctx context.Context, by Actor, ids []string) error {
	if !canManageRules(by) {
		return ErrForbidden
	}
	rs, err := s.S.ListAssignRules(ctx)
	if err != nil {
		return err
	}
	pos := map[string]int{}
	for i, x := range ids {
		pos[x] = i + 1
	}
	for _, r := range rs {
		if p, ok := pos[r.ID]; ok && r.Order != p {
			r.Order = p
			if err := s.S.PutAssignRule(ctx, r); err != nil {
				return err
			}
		}
	}
	s.flushRules()
	return nil
}

// AssignPreview shows which rule would route a step and to whom, without
// assigning anything (round-robin is not advanced).
type AssignPreview struct {
	Rule     string   `json:"rule,omitempty"`
	Assignee string   `json:"assignee,omitempty"`
	Queue    string   `json:"queue,omitempty"`
	Role     string   `json:"role,omitempty"`
	Urgency  int      `json:"urgency"`
	SLA      string   `json:"sla,omitempty"`
	Pool     []string `json:"pool,omitempty"` // people who could get it
	Note     string   `json:"note,omitempty"`
}

func (s *Service) PreviewAssign(ctx context.Context, by Actor, caseType, step, stage, role string, data map[string]any, rules []*store.AssignRule) (*AssignPreview, error) {
	if !canManageRules(by) {
		return nil, ErrForbidden
	}
	if rules == nil {
		rules, _ = s.S.ListAssignRules(ctx)
	}
	f := ruleFacts{CaseType: caseType, Step: step, Stage: stage, Role: role, Urgency: defaultUrgency, Data: data}
	out := &AssignPreview{Urgency: defaultUrgency, Role: role}
	r := firstRule(rules, f)
	if r == nil {
		out.Note = "no rule matches: the step's own routing is used"
		return out, nil
	}
	out.Rule, out.Urgency, out.SLA = r.Name, defaultUrgency+r.Then.Urgency, r.Then.SLA
	a := r.Then
	switch a.Mode {
	case "keep":
		out.Note = "routing unchanged"
	case "role":
		out.Role = a.Role
	case "queue":
		out.Queue, out.Role = a.Queue, queuePrefix+a.Queue
	case "user":
		out.Assignee = a.User
	case "workload", "roundrobin", "skill":
		out.Queue = a.Queue
		list := s.candidates(ctx, a.Queue, role, a.Skill)
		counts, _ := s.S.OpenCounts(ctx)
		type c struct {
			id string
			n  int
		}
		var pool []c
		for _, u := range list {
			n := counts[u.ID]
			if (u.Capacity > 0 && n >= u.Capacity) || (a.MaxPerPerson > 0 && n >= a.MaxPerPerson) {
				continue
			}
			pool = append(pool, c{u.ID, n})
		}
		sort.SliceStable(pool, func(i, j int) bool { return pool[i].n < pool[j].n })
		for _, p := range pool {
			out.Pool = append(out.Pool, fmt.Sprintf("%s (%d)", p.id, p.n))
		}
		switch {
		case len(pool) == 0:
			out.Note = "nobody available: the item waits in the queue"
		case a.Mode == "roundrobin":
			out.Note = "next person in turn"
		default:
			out.Assignee = pool[0].id
		}
	}
	return out, nil
}

// WorkStep is a human step of a published case type (for rule pickers).
type WorkStep struct {
	CaseType string `json:"caseType"`
	Step     string `json:"step"`
	Name     string `json:"name"`
	Role     string `json:"role,omitempty"`
}

// WorkSteps lists the human steps of every published case type.
func (s *Service) WorkSteps(ctx context.Context) ([]WorkStep, error) {
	ps, err := s.S.ListProcesses(ctx)
	if err != nil {
		return nil, err
	}
	out := []WorkStep{}
	for _, p := range ps {
		v, err := s.ResolveVersion(ctx, p.Key, 0)
		if err != nil {
			continue
		}
		proc, _, err := s.Process(ctx, v.ID)
		if err != nil {
			continue
		}
		bs, _ := s.S.GetBindings(ctx, v.ID)
		roles := map[string]string{}
		for _, b := range bs {
			roles[b.NodeID] = b.Role
		}
		for nid, n := range proc.Nodes {
			if n.Type != "userTask" && n.Type != "manualTask" {
				continue
			}
			out = append(out, WorkStep{CaseType: p.Key, Step: nid, Name: firstNonEmpty(n.Name, nid), Role: firstNonEmpty(roles[nid], n.Ext["role"])})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CaseType != out[j].CaseType {
			return out[i].CaseType < out[j].CaseType
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

package app

// Case model: the case-management layer on top of the BPMN flow.
//
//   - Stages group flow steps into the phases a business user talks about
//     (Request → Review → Approval → Fulfilment). Primary stages form the
//     happy path; alternate stages (Withdrawn, On hold) are entered by an
//     action; resolution stages close the case.
//   - The data model gives every case typed fields with validation and
//     calculated values, shared by all forms of the case type.
//   - Actions are things a user may do at any time in a stage, outside the
//     flow: update data, jump to an alternate stage, open a child case.
//
// The model is stored with the version (immutable once published) and is
// fully optional: a version without one behaves as a single stage.

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

type CaseModel = store.CaseModel

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var fieldPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var dataTypes = map[string]bool{"text": true, "longtext": true, "number": true, "money": true, "date": true,
	"boolean": true, "enum": true, "list": true, "user": true, "org": true, "reference": true}

// ValidateModel checks a case model against the flow it belongs to.
func ValidateModel(m *CaseModel, p *bpmn.Process) error {
	if m == nil {
		return nil
	}
	seen := map[string]bool{}
	owner := map[string]string{}
	primary := 0
	for i, st := range m.Stages {
		if !keyPattern.MatchString(st.Key) {
			return invalid("stage #%d: key %q must be lowercase letters, digits, - or _", i+1, st.Key)
		}
		if seen[st.Key] {
			return invalid("stage %q is duplicated", st.Key)
		}
		seen[st.Key] = true
		switch st.Kind {
		case "primary":
			primary++
		case "alternate", "resolution":
		default:
			return invalid("stage %s: kind must be primary, alternate or resolution", st.Key)
		}
		if st.Entry != "" && p.Nodes[st.Entry] == nil {
			return invalid("stage %s: entry step %q is not in the flow", st.Key, st.Entry)
		}
		if st.Kind == "alternate" && st.Entry == "" {
			return invalid("alternate stage %s needs an entry step", st.Key)
		}
		for _, n := range st.Steps {
			if p.Nodes[n] == nil {
				return invalid("stage %s: step %q is not in the flow", st.Key, n)
			}
			if o, ok := owner[n]; ok {
				return invalid("step %s is in both stage %s and %s", n, o, st.Key)
			}
			owner[n] = st.Key
		}
		if st.SLA != "" {
			if _, err := engine.ParseDuration(st.SLA); err != nil {
				return invalid("stage %s: %v", st.Key, err)
			}
		}
	}
	if len(m.Stages) > 0 && primary == 0 {
		return invalid("at least one primary stage is required")
	}
	fields := map[string]bool{}
	for i, f := range m.Data {
		if !fieldPattern.MatchString(f.Key) {
			return invalid("field #%d: key %q must be an identifier", i+1, f.Key)
		}
		if fields[f.Key] {
			return invalid("field %q is duplicated", f.Key)
		}
		fields[f.Key] = true
		if !dataTypes[f.Type] {
			return invalid("field %s: unknown type %q", f.Key, f.Type)
		}
		if f.Type == "enum" && len(f.Options) == 0 {
			return invalid("field %s: enum needs options", f.Key)
		}
	}
	acts := map[string]bool{}
	for i, a := range m.Actions {
		if !keyPattern.MatchString(a.Key) {
			return invalid("action #%d: key %q must be lowercase", i+1, a.Key)
		}
		if acts[a.Key] {
			return invalid("action %q is duplicated", a.Key)
		}
		acts[a.Key] = true
		for _, s := range a.Stages {
			if !seen[s] {
				return invalid("action %s: unknown stage %q", a.Key, s)
			}
		}
		switch a.Kind {
		case "update":
		case "stage":
			if !seen[a.Target] {
				return invalid("action %s: target stage %q does not exist", a.Key, a.Target)
			}
		case "child":
			if a.Target == "" {
				return invalid("action %s: child case type is required", a.Key)
			}
		default:
			return invalid("action %s: kind must be update, stage or child", a.Key)
		}
	}
	return nil
}

// stageOf returns the stage key that owns a flow step.
func stageOf(m *CaseModel, p *bpmn.Process, node string) string {
	if m == nil || len(m.Stages) == 0 {
		return ""
	}
	for _, st := range m.Stages {
		for _, n := range st.Steps {
			if n == node {
				return st.Key
			}
		}
	}
	// nodes inside a sub-process belong to the stage of the sub-process
	if n := p.Nodes[node]; n != nil && n.Parent != "" {
		return stageOf(m, p, n.Parent)
	}
	if n := p.Nodes[node]; n != nil && n.Ext["stage"] != "" {
		return n.Ext["stage"]
	}
	return ""
}

// currentStage derives the stage from the case's tokens (the furthest
// primary stage wins when parallel branches span stages).
func currentStage(m *CaseModel, p *bpmn.Process, c *store.Case) string {
	if m == nil || len(m.Stages) == 0 {
		return ""
	}
	if c.Instance.Status == engine.Completed || c.Instance.Status == engine.Cancelled {
		if c.Stage != "" {
			if st := m.Stage(c.Stage); st != nil && st.Kind == "resolution" {
				return c.Stage
			}
		}
		for _, st := range m.Stages {
			if st.Kind == "resolution" {
				return st.Key
			}
		}
		return c.Stage
	}
	best, bestIdx := "", -1
	for _, t := range c.Instance.Tokens {
		k := stageOf(m, p, t.Node)
		if k == "" {
			continue
		}
		for i, st := range m.Stages {
			if st.Key == k && i > bestIdx {
				best, bestIdx = k, i
			}
		}
	}
	if best == "" {
		return c.Stage
	}
	return best
}

// applyData enforces the data model on submitted values: type coercion,
// required fields (when the step is the last of a stage or forced),
// validation rules and calculated fields.
func applyData(m *CaseModel, vars map[string]any, submitted map[string]any) error {
	if m == nil {
		return nil
	}
	var errs []string
	for _, f := range m.Data {
		v, ok := submitted[f.Key]
		if !ok || v == nil {
			continue
		}
		cv, err := coerce(f, v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f.Label, err))
			continue
		}
		submitted[f.Key] = cv
	}
	if len(errs) > 0 {
		return invalid("%s", strings.Join(errs, "; "))
	}
	merged := map[string]any{}
	for k, v := range vars {
		merged[k] = v
	}
	for k, v := range submitted {
		merged[k] = v
	}
	recalc(m, merged)
	for _, f := range m.Data {
		if f.Calc != "" {
			submitted[f.Key] = merged[f.Key]
		}
	}
	for _, r := range m.Rules {
		if r.When != "" {
			if ok, err := engine.EvalBool(r.When, merged); err != nil || !ok {
				continue
			}
		}
		ok, err := engine.EvalBool(r.Check, merged)
		if err != nil {
			continue // fields not filled yet
		}
		if !ok {
			errs = append(errs, r.Message)
		}
	}
	if len(errs) > 0 {
		return invalid("%s", strings.Join(errs, "; "))
	}
	return nil
}

func recalc(m *CaseModel, vars map[string]any) {
	for pass := 0; pass < 3; pass++ { // calc fields may depend on each other
		for _, f := range m.Data {
			if f.Calc == "" {
				continue
			}
			if v, err := engine.EvalAny(f.Calc, vars); err == nil {
				vars[f.Key] = v
			}
		}
	}
}

// MissingRequired lists required data fields that are still empty.
func MissingRequired(m *CaseModel, vars map[string]any, stage string) []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, f := range m.Data {
		if !f.Required || f.Calc != "" {
			continue
		}
		if len(f.RequiredIn) > 0 && !contains(f.RequiredIn, stage) {
			continue
		}
		if v, ok := vars[f.Key]; !ok || v == nil || fmt.Sprint(v) == "" {
			out = append(out, f.Label)
		}
	}
	return out
}

func coerce(f store.DataField, v any) (any, error) {
	switch f.Type {
	case "number", "money":
		switch x := v.(type) {
		case float64:
			return x, nil
		case int:
			return float64(x), nil
		case string:
			var n float64
			if _, err := fmt.Sscan(strings.ReplaceAll(x, ",", ""), &n); err != nil {
				return nil, fmt.Errorf("must be a number")
			}
			return n, nil
		}
		return nil, fmt.Errorf("must be a number")
	case "boolean":
		if b, ok := v.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("must be yes or no")
	case "date":
		s := fmt.Sprint(v)
		if _, err := time.Parse("2006-01-02", s); err != nil {
			if _, err := time.Parse(time.RFC3339, s); err != nil {
				return nil, fmt.Errorf("must be a date (YYYY-MM-DD)")
			}
		}
		return s, nil
	case "enum":
		s := fmt.Sprint(v)
		for _, o := range f.Options {
			if o.Value == s {
				return s, nil
			}
		}
		return nil, fmt.Errorf("%q is not an allowed value", s)
	case "list":
		if l, ok := v.([]any); ok {
			return l, nil
		}
		return nil, fmt.Errorf("must be a list")
	default:
		return v, nil
	}
}

// ---------------------------------------------------------------- service

// PutModel saves the case model of a draft version.
func (s *Service) PutModel(ctx context.Context, by Actor, vid string, m *CaseModel) (*store.Version, error) {
	if !by.Has(RoleDesigner) {
		return nil, ErrForbidden
	}
	v, err := s.S.GetVersion(ctx, vid)
	if err != nil {
		return nil, err
	}
	if v.Status != store.Draft {
		return nil, conflict("version %d is %s; clone it to change the case model", v.No, v.Status)
	}
	p, _, err := s.Process(ctx, vid)
	if err != nil {
		return nil, err
	}
	if err := ValidateModel(m, p); err != nil {
		return nil, err
	}
	v.Model = m
	s.audit(ctx, by, "model.update", "version", vid, map[string]any{"stages": len(m.Stages), "fields": len(m.Data), "actions": len(m.Actions)})
	return v, s.S.PutVersion(ctx, v)
}

// StageView is one stage as shown on the case's stage bar.
type StageView struct {
	Key     string     `json:"key"`
	Name    string     `json:"name"`
	Kind    string     `json:"kind"`
	State   string     `json:"state"` // done | current | upcoming | skipped
	Entered *time.Time `json:"entered,omitempty"`
	Left    *time.Time `json:"left,omitempty"`
	DueAt   *time.Time `json:"dueAt,omitempty"`
}

// ActionView is an action the caller may run now.
type ActionView struct {
	store.CaseAction
	Allowed bool `json:"allowed"`
}

// CaseOverview returns the stage bar and available actions of a case.
func (s *Service) CaseOverview(ctx context.Context, by Actor, c *store.Case) ([]StageView, []ActionView, []string) {
	v, err := s.S.GetVersion(ctx, c.VersionID)
	if err != nil || v.Model == nil || len(v.Model.Stages) == 0 {
		return nil, nil, nil
	}
	m := v.Model
	var stages []StageView
	visited := map[string]*StageView{}
	for _, h := range c.StageHistory {
		sv := visited[h.Stage]
		if sv == nil {
			sv = &StageView{}
			visited[h.Stage] = sv
		}
		at := h.At
		if sv.Entered == nil {
			sv.Entered = &at
		}
		if h.Left != nil {
			l := *h.Left
			sv.Left = &l
		} else {
			sv.Left = nil
		}
	}
	curIdx := -1
	for i, st := range m.Stages {
		if st.Key == c.Stage {
			curIdx = i
		}
	}
	for i, st := range m.Stages {
		x := StageView{Key: st.Key, Name: st.Name, Kind: st.Kind, State: "upcoming"}
		if h := visited[st.Key]; h != nil {
			x.Entered, x.Left = h.Entered, h.Left
			x.State = "done"
		}
		if st.Key == c.Stage {
			x.State = "current"
			if st.SLA != "" && x.Entered != nil {
				if d, err := engine.ParseDuration(st.SLA); err == nil {
					due := x.Entered.Add(d)
					x.DueAt = &due
				}
			}
		} else if x.State == "upcoming" && st.Kind == "primary" && i < curIdx {
			x.State = "skipped"
		}
		// alternate / resolution stages only show when reached
		if st.Kind != "primary" && x.State == "upcoming" {
			continue
		}
		stages = append(stages, x)
	}
	var acts []ActionView
	running := c.Instance.Status == engine.Running
	for _, a := range m.Actions {
		ok := running && (len(a.Stages) == 0 || contains(a.Stages, c.Stage))
		if ok && len(a.Roles) > 0 {
			ok = false
			for _, r := range a.Roles {
				ok = ok || by.Has(r)
			}
		}
		if ok || by.Has(RoleSupervisor) {
			acts = append(acts, ActionView{CaseAction: a, Allowed: ok})
		}
	}
	return stages, acts, MissingRequired(m, c.Instance.Vars, c.Stage)
}

// trackStage updates the case's current stage and stage history.
func (s *Service) trackStage(ctx context.Context, c *store.Case, p *bpmn.Process) {
	v, err := s.S.GetVersion(ctx, c.VersionID)
	if err != nil || v.Model == nil {
		return
	}
	next := currentStage(v.Model, p, c)
	if next == "" || next == c.Stage {
		return
	}
	now := s.Now()
	for i := range c.StageHistory {
		if c.StageHistory[i].Left == nil {
			c.StageHistory[i].Left = &now
		}
	}
	c.StageHistory = append(c.StageHistory, store.StageEntry{Stage: next, At: now})
	_ = s.S.AppendEvents(ctx, []store.EventRec{{CaseID: c.ID, At: now, Type: "stage", Node: next, Data: map[string]any{"from": c.Stage}}})
	c.Stage = next
}

// RunAction executes a case action for the caller.
func (s *Service) RunAction(ctx context.Context, by Actor, caseID, key string, data map[string]any) (*store.Case, error) {
	c, err := s.S.GetCase(ctx, caseID)
	if err != nil {
		return nil, err
	}
	v, err := s.S.GetVersion(ctx, c.VersionID)
	if err != nil {
		return nil, err
	}
	if v.Model == nil {
		return nil, invalid("case type has no actions")
	}
	var act *store.CaseAction
	for i := range v.Model.Actions {
		if v.Model.Actions[i].Key == key {
			act = &v.Model.Actions[i]
		}
	}
	if act == nil {
		return nil, invalid("unknown action %q", key)
	}
	_, allowed, _ := s.CaseOverview(ctx, by, c)
	ok := false
	for _, a := range allowed {
		ok = ok || (a.Key == key && a.Allowed)
	}
	if !ok {
		return nil, fmt.Errorf("%w: action %q is not available", ErrForbidden, act.Label)
	}
	if act.FormKey != "" && act.Kind != "child" {
		if f, err := s.S.GetForm(ctx, act.FormKey); err == nil {
			if err := ValidateForm(f, data); err != nil {
				return nil, err
			}
		}
	}

	switch act.Kind {
	case "update":
		return s.withCase(ctx, caseID, func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error) {
			if err := applyData(v.Model, c.Instance.Vars, data); err != nil {
				return nil, err
			}
			for k, val := range data {
				c.Instance.Vars[k] = val
			}
			s.audit(ctx, by, "action."+key, "case", c.ID, map[string]any{"label": act.Label, "data": data})
			fx, _, err := s.E.Signal(ctx, p, c.Instance, "data.changed", nil) // re-evaluates conditional events
			return fx, err
		}, by)

	case "stage":
		target := v.Model.Stage(act.Target)
		return s.withCase(ctx, caseID, func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error) {
			for k, val := range data {
				c.Instance.Vars[k] = val
			}
			s.audit(ctx, by, "action."+key, "case", c.ID, map[string]any{"label": act.Label, "stage": target.Key, "data": data})
			if target.Entry == "" {
				return &engine.Effects{}, nil
			}
			return s.E.Jump(ctx, p, c.Instance, target.Entry, by.ID)
		}, by)

	case "child":
		vars := map[string]any{}
		for _, k := range act.Pass {
			vars[k] = c.Instance.Vars[k]
		}
		for k, val := range data {
			vars[k] = val
		}
		child, err := s.StartCase(ctx, by, StartInput{ProcessKey: act.Target, FiscalYear: c.FiscalYear,
			Title: firstNonEmpty(fmt.Sprint(data["title"]), c.Title+" / "+act.Label), Vars: vars, ParentID: c.ID})
		if err != nil {
			return nil, err
		}
		s.audit(ctx, by, "action."+key, "case", c.ID, map[string]any{"label": act.Label, "child": child.Number})
		return child, nil
	}
	return nil, invalid("unknown action kind")
}

// notifyParent tells a parent case that a child case closed: message
// "child.completed" for each child and "children.completed" once all of
// its children are closed (wait-for-all), so the parent flow can wait for
// either with a receive task.
func (s *Service) notifyParent(ctx context.Context, by Actor, child *store.Case) error {
	if child.ParentID == "" || child.Instance.ParentToken != "" {
		return nil // call activities resume their parent directly
	}
	parent, err := s.S.GetCase(ctx, child.ParentID)
	if err != nil || parent.Instance.Status != engine.Running {
		return nil
	}
	vars := map[string]any{"lastChild": child.Number, "lastChildStatus": child.Status}
	if err := s.deliverTo(ctx, by, parent, "child.completed", vars); err != nil {
		return err
	}
	kids, err := s.children(ctx, parent.ID)
	if err != nil {
		return err
	}
	for _, k := range kids {
		if k.Instance.Status == engine.Running {
			return nil
		}
	}
	parent, err = s.S.GetCase(ctx, parent.ID)
	if err != nil {
		return err
	}
	return s.deliverTo(ctx, by, parent, "children.completed", vars)
}

func (s *Service) deliverTo(ctx context.Context, by Actor, c *store.Case, name string, vars map[string]any) error {
	p, _, err := s.Process(ctx, c.VersionID)
	if err != nil {
		return err
	}
	fx, n, err := s.E.Message(ctx, p, c.Instance, name, vars)
	if err != nil || n == 0 {
		return err
	}
	return s.apply(ctx, by, c, p, fx)
}

func (s *Service) children(ctx context.Context, parentID string) ([]*store.Case, error) {
	all, err := s.S.ListCases(ctx, "", "", 0)
	if err != nil {
		return nil, err
	}
	var out []*store.Case
	for _, c := range all {
		if c.ParentID == parentID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// Relate links (or unlinks) two cases both ways.
func (s *Service) Relate(ctx context.Context, by Actor, caseID, otherID string, link bool) error {
	if caseID == otherID {
		return invalid("a case cannot relate to itself")
	}
	return s.tx(ctx, func(ctx context.Context) error {
		for _, pair := range [][2]string{{caseID, otherID}, {otherID, caseID}} {
			c, err := s.S.GetCase(ctx, pair[0])
			if err != nil {
				return err
			}
			var ids []string
			for _, id := range c.RelatedIDs {
				if id != pair[1] {
					ids = append(ids, id)
				}
			}
			if link {
				ids = append(ids, pair[1])
			}
			c.RelatedIDs = ids
			if err := s.S.PutCase(ctx, c); err != nil {
				return err
			}
		}
		s.audit(ctx, by, map[bool]string{true: "case.relate", false: "case.unrelate"}[link], "case", caseID, map[string]any{"other": otherID})
		return nil
	})
}

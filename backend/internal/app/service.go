// Package app is the application layer: it wires the BPMN engine to storage
// and implements versioning, cases, work items, screen bindings and migration.
package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

var (
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid request")
	ErrConflict  = errors.New("conflict")
)

func invalid(f string, a ...any) error { return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(f, a...)) }
func conflict(f string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(f, a...))
}

// Actor is the authenticated caller (resolved from the IAM / OIDC gateway in production).
type Actor struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

func (a Actor) Has(role string) bool {
	for _, r := range a.Roles {
		if r == role || r == RoleAdmin {
			return true
		}
	}
	return false
}

// Platform roles. Business roles (requester, approver, finance ...) are free-form
// and come from lanes / bindings.
const (
	RoleAdmin      = "cm:admin"      // tenant administration, import, migration
	RoleDesigner   = "cm:designer"   // case types, flows, forms, screens
	RoleApprover   = "cm:approver"   // approves / publishes versions
	RoleSupervisor = "cm:supervisor" // queues, reassign, move, dashboards
)

type Service struct {
	S      store.Store
	E      *engine.Engine
	Secret []byte // signs external-screen callback tokens
	Now    func() time.Time

	mu    sync.Mutex
	cache map[string]*bpmn.Process // published versions only (immutable)
}

func New(s store.Store, h engine.Handlers, secret []byte) *Service {
	e := engine.New(h, id.New)
	return &Service{S: s, E: e, Secret: secret, Now: time.Now, cache: map[string]*bpmn.Process{}}
}

func (s *Service) audit(ctx context.Context, by Actor, action, entity, ref string, data map[string]any) {
	_ = s.S.AddAudit(ctx, &store.Audit{At: s.Now(), Actor: by.ID, Action: action, Entity: entity, Ref: ref, Data: data})
}

// Process returns the parsed BPMN of a version.
func (s *Service) Process(ctx context.Context, versionID string) (*bpmn.Process, *store.Version, error) {
	v, err := s.S.GetVersion(ctx, versionID)
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	p, ok := s.cache[v.ID+v.Checksum]
	s.mu.Unlock()
	if ok {
		return p, v, nil
	}
	ps, err := bpmn.ParseString(v.BPMN)
	if err != nil {
		return nil, nil, err
	}
	p = ps[0]
	if v.Status == store.Published || v.Status == store.Deprecated {
		s.mu.Lock()
		s.cache[v.ID+v.Checksum] = p
		s.mu.Unlock()
	}
	return p, v, nil
}

func checksum(x string) string {
	h := sha256.Sum256([]byte(x))
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------- cases

type StartInput struct {
	ProcessKey string         `json:"processKey"`
	FiscalYear int            `json:"fiscalYear"`
	VersionID  string         `json:"versionId,omitempty"` // test-run a specific (even draft) version
	Title      string         `json:"title"`
	StartEvent string         `json:"startEvent,omitempty"`
	Vars       map[string]any `json:"vars"`
	ParentID   string         `json:"parentId,omitempty"`
	Test       bool           `json:"test,omitempty"` // studio test run
	number     int            // pre-allocated outside the transaction
	RelatedIDs []string       `json:"relatedIds,omitempty"`
}

func (s *Service) StartCase(ctx context.Context, by Actor, in StartInput) (*store.Case, error) {
	// Allocate the case number in its own short statement: the per-type
	// counter row would otherwise stay locked for the whole transaction and
	// serialise every case creation of that type (numbers may skip on rollback).
	if in.VersionID == "" {
		if v, err := s.ResolveVersion(ctx, in.ProcessKey, in.FiscalYear); err == nil {
			fy := in.FiscalYear
			if fy == 0 {
				fy = v.FiscalYear
			}
			if n, err := s.S.NextCaseNumber(ctx, fmt.Sprintf("%s-%d", v.ProcessKey, s.numberYear(fy))); err == nil {
				in.number = n
			}
		}
	}
	var out *store.Case
	err := s.S.Tx(ctx, func(ctx context.Context) error {
		c, err := s.startCase(ctx, by, in, "")
		out = c
		return err
	})
	return out, err
}

func (s *Service) startCase(ctx context.Context, by Actor, in StartInput, parentToken string) (*store.Case, error) {
	vid := in.VersionID
	if vid == "" {
		v, err := s.ResolveVersion(ctx, in.ProcessKey, in.FiscalYear)
		if err != nil {
			return nil, err
		}
		vid = v.ID
	} else if !by.Has(RoleDesigner) && !strings.HasPrefix(by.ID, "system") {
		if v, err := s.S.GetVersion(ctx, vid); err != nil || v.Status != store.Published {
			return nil, fmt.Errorf("%w: only designers can run unpublished versions", ErrForbidden)
		}
	}
	p, v, err := s.Process(ctx, vid)
	if err != nil {
		return nil, err
	}
	fy := in.FiscalYear
	if fy == 0 {
		fy = v.FiscalYear
	}
	n := in.number
	if n == 0 {
		var err error
		if n, err = s.S.NextCaseNumber(ctx, fmt.Sprintf("%s-%d", v.ProcessKey, s.numberYear(fy))); err != nil {
			return nil, err
		}
	}
	now := s.Now()
	c := &store.Case{ID: id.New(), Number: fmt.Sprintf("%s-%d-%05d", strings.ToUpper(v.ProcessKey), s.numberYear(fy), n),
		ProcessKey: v.ProcessKey, VersionID: v.ID, Title: in.Title, FiscalYear: fy, ParentID: in.ParentID,
		RelatedIDs: in.RelatedIDs, CreatedBy: by.ID, CreatedAt: now, UpdatedAt: now, Test: in.Test}
	if in.Test {
		if !by.Has(RoleDesigner) {
			return nil, ErrForbidden
		}
		c.Number = "TEST-" + c.Number
	}
	if c.Title == "" {
		c.Title = c.Number
	}
	vars := map[string]any{"fiscalYear": fy, "caseNumber": c.Number, "initiator": by.ID}
	for k, val := range in.Vars {
		vars[k] = val
	}
	inst, fx, err := s.E.Start(ctx, p, c.ID, v.ID, in.StartEvent, vars)
	if err != nil {
		return nil, err
	}
	if parentToken != "" {
		inst.ParentID, inst.ParentToken = in.ParentID, parentToken
	}
	c.Instance = inst
	s.audit(ctx, by, "case.start", "case", c.ID, map[string]any{"version": v.No, "number": c.Number})
	return c, s.apply(ctx, by, c, p, fx)
}

// ResolveVersion picks the published version for a fiscal year; with no
// fiscal year (or none published for it) the latest published version.
func (s *Service) ResolveVersion(ctx context.Context, key string, fy int) (*store.Version, error) {
	vs, err := s.S.ListVersions(ctx, key)
	if err != nil {
		return nil, err
	}
	var latest *store.Version
	for _, v := range vs {
		if v.Status != store.Published {
			continue
		}
		if fy != 0 && v.FiscalYear == fy {
			return v, nil
		}
		if latest == nil || v.No > latest.No {
			latest = v
		}
	}
	if latest == nil {
		return nil, invalid("case type %q has no published version", key)
	}
	if fy != 0 && latest.FiscalYear != 0 && latest.FiscalYear != fy {
		return nil, invalid("case type %q has no version published for fiscal year %d", key, fy)
	}
	return latest, nil
}

// apply persists the case and executes engine side effects.
func (s *Service) apply(ctx context.Context, by Actor, c *store.Case, p *bpmn.Process, fx *engine.Effects) error {
	for guard := 0; fx != nil; guard++ {
		if guard > 1000 {
			return errors.New("too many automatic step skips")
		}
		next, err := s.applyOnce(ctx, by, c, p, fx)
		if err != nil {
			return err
		}
		fx = next
	}
	return nil
}

func (s *Service) applyOnce(ctx context.Context, by Actor, c *store.Case, p *bpmn.Process, fx *engine.Effects) (*engine.Effects, error) {
	in := c.Instance
	now := s.Now()

	// cancellations first: they refer to tokens that no longer exist
	if len(fx.CancelWork) > 0 {
		open, err := s.S.ListWork(ctx, store.WorkFilter{CaseID: c.ID, Status: []store.WorkStatus{store.WorkOpen, store.WorkClaimed}})
		if err != nil {
			return nil, err
		}
		gone := map[string]bool{}
		for _, t := range fx.CancelWork {
			gone[t] = true
		}
		for _, w := range open {
			if gone[w.TokenID] && !tokenWaiting(in, w.TokenID) {
				w.Status = store.WorkCancelled
				if err := s.S.PutWork(ctx, w); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, t := range fx.CancelTimers {
		if !tokenAlive(in, t) {
			if err := s.S.DeleteTimersByToken(ctx, in.ID, t); err != nil {
				return nil, err
			}
		}
	}
	for _, tm := range fx.NewTimers {
		tm.ID = id.New()
		if err := s.S.PutTimer(ctx, &tm); err != nil {
			return nil, err
		}
	}
	if err := s.applyOps(ctx, c, p, fx); err != nil {
		return nil, err
	}

	binds, err := s.S.GetBindings(ctx, c.VersionID)
	if err != nil {
		return nil, err
	}
	bmap := map[string]store.Binding{}
	for _, b := range binds {
		bmap[b.NodeID] = b
	}
	var skipped *engine.Effects
	for _, wi := range fx.NewWork {
		w := &store.Work{WorkItem: wi, CaseID: c.ID, CaseNumber: c.Number, CaseTitle: c.Title, Status: store.WorkOpen, CreatedAt: now, Test: c.Test}
		if b, ok := bmap[wi.Node]; ok {
			if b.UI != "" {
				w.UI = b.UI
			}
			w.FormKey = firstNonEmpty(b.FormKey, w.FormKey)
			w.ScreenKey = b.ScreenKey
			w.URL = firstNonEmpty(b.URL, w.URL)
			w.Embed = b.Embed
			w.Role = firstNonEmpty(b.Role, w.Role)
			if b.SLA != "" {
				if d, err := engine.ParseDuration(b.SLA); err == nil {
					due := now.Add(d)
					w.SLA, w.DueAt = b.SLA, &due
				}
			}
			// step toggle: disabled (or condition false) human steps are skipped
			if skip(b, in.Vars) {
				ef, err := s.E.CompleteTask(ctx, p, in, wi.TokenID, "system:toggle", nil)
				if err != nil {
					return nil, err
				}
				s.audit(ctx, by, "step.skip", "case", c.ID, map[string]any{"node": wi.Node})
				skipped = mergeFx(skipped, ef)
				continue
			}
		}
		b := bmap[wi.Node]
		if err := s.prepareWork(ctx, w, &b, in.Vars, now); err != nil {
			return nil, err
		}
		if err := s.S.PutWork(ctx, w); err != nil {
			return nil, err
		}
		if w.Assignee != "" && !w.Test {
			s.notify(ctx, w.Assignee, "assigned", w.CaseNumber+" · "+w.Name, "/work/"+w.ID)
		}
	}

	c.Status = string(in.Status)
	c.UpdatedAt = now
	if err := s.persist(ctx, c, p); err != nil {
		return nil, err
	}

	for _, sc := range fx.StartChildren {
		_, err := s.startCase(ctx, Actor{ID: "system:call", Roles: by.Roles}, StartInput{ProcessKey: sc.ProcessKey,
			FiscalYear: c.FiscalYear, Title: c.Title + " / " + sc.ProcessKey, Vars: sc.Vars, ParentID: c.ID}, sc.ParentToken)
		if err != nil {
			return nil, fmt.Errorf("call activity %s: %w", sc.ProcessKey, err)
		}
	}
	for _, pt := range fx.CancelChild {
		kids, _ := s.S.ListCases(ctx, "", "", 0)
		for _, k := range kids {
			if k.ParentID == c.ID && k.Instance != nil && k.Instance.ParentToken == pt && k.Instance.Status == engine.Running {
				kp, _, err := s.Process(ctx, k.VersionID)
				if err == nil {
					_ = s.apply(ctx, by, k, kp, s.E.Cancel(ctx, kp, k.Instance, by.ID))
				}
			}
		}
	}
	for _, ob := range fx.Outbound {
		if err := s.deliver(ctx, by, c, ob); err != nil {
			return nil, err
		}
	}
	if fx.Completed && c.ParentID != "" && in.ParentToken == "" {
		if err := s.notifyParent(ctx, by, c); err != nil {
			return nil, err
		}
	}
	if fx.Completed && c.ParentID != "" && in.ParentToken != "" {
		parent, err := s.S.GetCase(ctx, c.ParentID)
		if err == nil && parent.Instance.Status == engine.Running {
			pp, _, err := s.Process(ctx, parent.VersionID)
			if err != nil {
				return nil, err
			}
			pfx, err := s.E.ChildCompleted(ctx, pp, parent.Instance, in.ParentToken, in.Vars)
			if err != nil && !errors.Is(err, engine.ErrNotWaiting) {
				return nil, err
			}
			if pfx != nil {
				if err := s.apply(ctx, by, parent, pp, pfx); err != nil {
					return nil, err
				}
			}
		}
	}
	return skipped, nil
}

func skip(b store.Binding, vars map[string]any) bool {
	if b.Enabled != nil && !*b.Enabled {
		return true
	}
	if b.Condition != "" {
		ok, err := engine.EvalBool(b.Condition, vars)
		return err == nil && !ok
	}
	return false
}

func mergeFx(a, b *engine.Effects) *engine.Effects {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	a.NewWork = append(a.NewWork, b.NewWork...)
	a.CancelWork = append(a.CancelWork, b.CancelWork...)
	a.NewTimers = append(a.NewTimers, b.NewTimers...)
	a.CancelTimers = append(a.CancelTimers, b.CancelTimers...)
	a.StartChildren = append(a.StartChildren, b.StartChildren...)
	a.CancelChild = append(a.CancelChild, b.CancelChild...)
	a.Outbound = append(a.Outbound, b.Outbound...)
	a.Completed = a.Completed || b.Completed
	return a
}

func tokenAlive(in *engine.Instance, tok string) bool {
	for _, t := range in.Tokens {
		if t.ID == tok {
			return true
		}
	}
	return false
}

func tokenWaiting(in *engine.Instance, tok string) bool {
	for _, t := range in.Tokens {
		if t.ID == tok && t.Wait == engine.WaitUser {
			return true
		}
	}
	return false
}

// deliver routes thrown messages to related cases (parent, children,
// related) and signals to every waiting case, using the subscription index.
func (s *Service) deliver(ctx context.Context, by Actor, from *store.Case, ob engine.Outbound) error {
	subs, err := s.S.FindSubscriptions(ctx, ob.Kind, ob.Name)
	if err != nil {
		return err
	}
	done := map[string]bool{}
	for _, sub := range subs {
		if sub.CaseID == from.ID || done[sub.CaseID] {
			continue
		}
		done[sub.CaseID] = true
		c, err := s.S.GetCase(ctx, sub.CaseID)
		if err != nil || c.Instance == nil || c.Instance.Status != engine.Running {
			continue
		}
		related := c.ID == from.ParentID || c.ParentID == from.ID || contains(from.RelatedIDs, c.ID) || contains(c.RelatedIDs, from.ID)
		if ob.Kind == "message" && !related {
			continue
		}
		p, _, err := s.Process(ctx, c.VersionID)
		if err != nil {
			return err
		}
		var fx *engine.Effects
		var n int
		if ob.Kind == "signal" {
			fx, n, err = s.E.Signal(ctx, p, c.Instance, ob.Name, ob.Vars)
		} else {
			fx, n, err = s.E.Message(ctx, p, c.Instance, ob.Name, ob.Vars)
		}
		if err != nil {
			return err
		}
		if n > 0 {
			if err := s.apply(ctx, by, c, p, fx); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- work

func (s *Service) canWork(by Actor, w *store.Work) bool {
	if by.Has(RoleSupervisor) || strings.HasPrefix(by.ID, "system") || (w.Test && by.Has(RoleDesigner)) {
		return true
	}
	if w.Assignee != "" {
		return w.Assignee == by.ID
	}
	if strings.HasPrefix(w.Role, queuePrefix) {
		return contains(s.effectiveRoles(context.Background(), by), w.Role)
	}
	return w.Role == "" || by.Has(w.Role)
}

func (s *Service) Inbox(ctx context.Context, by Actor, all bool) ([]*store.Work, error) {
	// most urgent first; the worklist is paged (get-next-work in Phase 3)
	f := store.WorkFilter{Status: []store.WorkStatus{store.WorkOpen, store.WorkClaimed}, Limit: 200}
	if !(all && by.Has(RoleSupervisor)) {
		f.Assignee, f.Roles = by.ID, by.Roles
	}
	if f.Roles != nil {
		f.Roles = s.effectiveRoles(ctx, by)
		f.Assignee = by.ID
	}
	ws, err := s.S.ListWork(ctx, f)
	out := ws[:0]
	for _, w := range ws {
		if !w.Test { // studio test runs never show in real work lists
			out = append(out, w)
		}
	}
	sortWork(out)
	return out, err
}

func (s *Service) Claim(ctx context.Context, by Actor, workID string) (*store.Work, error) {
	var out *store.Work
	err := s.S.Tx(ctx, func(ctx context.Context) error {
		w, err := s.S.GetWork(ctx, workID)
		if err != nil {
			return err
		}
		if w.Status != store.WorkOpen && w.Assignee != by.ID {
			return conflict("work item already %s by %s", w.Status, w.Assignee)
		}
		if !s.canWork(by, w) {
			return ErrForbidden
		}
		w.Assignee, w.Status = by.ID, store.WorkClaimed
		out = w
		s.audit(ctx, by, "work.claim", "case", w.CaseID, map[string]any{"work": w.ID})
		return s.S.PutWork(ctx, w)
	})
	return out, err
}

// Reassign (supervisor) gives a work item to a user or back to the role queue.
func (s *Service) Reassign(ctx context.Context, by Actor, workID, assignee, role string) (*store.Work, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	var out *store.Work
	err := s.S.Tx(ctx, func(ctx context.Context) error {
		w, err := s.S.GetWork(ctx, workID)
		if err != nil {
			return err
		}
		w.Assignee = assignee
		if role != "" {
			w.Role = role
		}
		w.Status = store.WorkOpen
		if assignee != "" {
			w.Status = store.WorkClaimed
		}
		out = w
		s.audit(ctx, by, "work.reassign", "case", w.CaseID, map[string]any{"work": w.ID, "to": assignee, "role": role})
		return s.S.PutWork(ctx, w)
	})
	return out, err
}

// Complete finishes a work item with the submitted form data.
func (s *Service) Complete(ctx context.Context, by Actor, workID string, data map[string]any) (*store.Case, error) {
	var out *store.Case
	err := s.tx(ctx, func(ctx context.Context) error {
		w, err := s.S.GetWork(ctx, workID)
		if err != nil {
			return err
		}
		if w.Status == store.WorkDone || w.Status == store.WorkCancelled {
			return conflict("work item is %s", w.Status)
		}
		if !s.canWork(by, w) {
			return ErrForbidden
		}
		if w.FormKey != "" {
			if f, err := s.S.GetForm(ctx, w.FormKey); err == nil {
				if err := ValidateForm(f, data); err != nil {
					return err
				}
			}
		}
		c, err := s.S.GetCase(ctx, w.CaseID)
		if err != nil {
			return err
		}
		p, _, err := s.Process(ctx, c.VersionID)
		if err != nil {
			return err
		}
		if v, err := s.S.GetVersion(ctx, c.VersionID); err == nil {
			if err := applyData(v.Model, c.Instance.Vars, data); err != nil {
				return err
			}
		}
		fx, err := s.E.CompleteTask(ctx, p, c.Instance, w.TokenID, by.ID, data)
		if errors.Is(err, engine.ErrNotWaiting) {
			// someone else completed (or moved) this step a moment earlier
			return conflict("work item was already completed")
		}
		if err != nil {
			return err
		}
		now := s.Now()
		w.Status, w.CompletedBy, w.CompletedAt = store.WorkDone, by.ID, &now
		if err := s.S.PutWork(ctx, w); err != nil {
			return err
		}
		s.audit(ctx, by, "work.complete", "case", c.ID, map[string]any{"work": w.ID, "node": w.Node, "data": data})
		out = c
		return s.apply(ctx, by, c, p, fx)
	})
	return out, err
}

// CallbackToken signs a work item id for an external screen. The external
// system completes the step by POSTing to /api/callback/{work}?token=...
func (s *Service) CallbackToken(workID string) string {
	m := hmac.New(sha256.New, s.Secret)
	m.Write([]byte(workID))
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Service) CompleteByCallback(ctx context.Context, workID, token string, data map[string]any) (*store.Case, error) {
	if !hmac.Equal([]byte(token), []byte(s.CallbackToken(workID))) {
		return nil, ErrForbidden
	}
	return s.Complete(ctx, Actor{ID: "system:callback"}, workID, data)
}

// ---------------------------------------------------------------- case ops

func (s *Service) withCase(ctx context.Context, caseID string, fn func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error), by Actor) (*store.Case, error) {
	var out *store.Case
	err := s.tx(ctx, func(ctx context.Context) error {
		c, err := s.S.GetCase(ctx, caseID)
		if err != nil {
			return err
		}
		p, _, err := s.Process(ctx, c.VersionID)
		if err != nil {
			return err
		}
		fx, err := fn(ctx, c, p)
		if err != nil {
			return err
		}
		out = c
		return s.apply(ctx, by, c, p, fx)
	})
	return out, err
}

func (s *Service) Cancel(ctx context.Context, by Actor, caseID, reason string) (*store.Case, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	return s.withCase(ctx, caseID, func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error) {
		s.audit(ctx, by, "case.cancel", "case", c.ID, map[string]any{"reason": reason})
		fx := s.E.Cancel(ctx, p, c.Instance, by.ID)
		return fx, nil
	}, by)
}

// Move sends a case token to another step (send back / skip ahead).
func (s *Service) Move(ctx context.Context, by Actor, caseID, tokenID, toNode, reason string) (*store.Case, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	return s.withCase(ctx, caseID, func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error) {
		s.audit(ctx, by, "case.move", "case", c.ID, map[string]any{"token": tokenID, "to": toNode, "reason": reason})
		return s.E.MoveToken(ctx, p, c.Instance, tokenID, toNode, by.ID)
	}, by)
}

// Message correlates a message to one case (caseID) or, when empty, to every
// running case whose variable correlationKey matches.
func (s *Service) Message(ctx context.Context, by Actor, caseID, name, correlationKey string, vars map[string]any) (int, error) {
	cases := []*store.Case{}
	if caseID != "" {
		c, err := s.S.GetCase(ctx, caseID)
		if err != nil {
			return 0, err
		}
		cases = append(cases, c)
	} else {
		subs, err := s.S.FindSubscriptions(ctx, "message", name)
		if err != nil {
			return 0, err
		}
		seen := map[string]bool{}
		for _, sub := range subs {
			if seen[sub.CaseID] {
				continue
			}
			seen[sub.CaseID] = true
			c, err := s.S.GetCase(ctx, sub.CaseID)
			if err == nil && (correlationKey == "" || fmt.Sprint(c.Instance.Vars["correlationKey"]) == correlationKey) {
				cases = append(cases, c)
			}
		}
	}
	total := 0
	for _, c := range cases {
		if c.Instance.Status != engine.Running {
			continue
		}
		var n int
		_, err := s.withCase(ctx, c.ID, func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error) {
			fx, k, err := s.E.Message(ctx, p, c.Instance, name, vars)
			n = k
			return fx, err
		}, by)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (s *Service) Signal(ctx context.Context, by Actor, name string, vars map[string]any) error {
	return s.S.Tx(ctx, func(ctx context.Context) error {
		return s.deliver(ctx, by, &store.Case{ID: "-"}, engine.Outbound{Kind: "signal", Name: name, Vars: vars})
	})
}

// RunTimers fires due timers; called periodically by the worker.
func (s *Service) RunTimers(ctx context.Context) (int, error) {
	fired := 0
	for {
		var batch int
		err := s.tx(ctx, func(ctx context.Context) error {
			ts, err := s.S.DueTimers(ctx, s.Now(), 50)
			if err != nil {
				return err
			}
			batch = len(ts)
			for _, tm := range ts {
				if err := s.S.DeleteTimer(ctx, tm.ID); err != nil {
					return err
				}
				c, err := s.S.GetCase(ctx, tm.InstanceID)
				if err != nil || c.Instance.Status != engine.Running {
					continue
				}
				p, _, err := s.Process(ctx, c.VersionID)
				if err != nil {
					return err
				}
				fx, err := s.E.FireTimer(ctx, p, c.Instance, *tm)
				if err != nil {
					return err
				}
				if err := s.apply(ctx, Actor{ID: "system:timer"}, c, p, fx); err != nil {
					return err
				}
				fired++
				Metrics.TimersFired.Add(1)
			}
			return nil
		})
		if err != nil || batch < 50 {
			return fired, err
		}
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// numberYear is the year shown in case numbers: the fiscal year for case
// types versioned by year, otherwise the calendar year.
func (s *Service) numberYear(fy int) int {
	if fy != 0 {
		return fy
	}
	return s.Now().Year()
}

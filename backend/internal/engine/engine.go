package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
)

// BPMNError is returned by handlers to raise a catchable BPMN error.
type BPMNError struct{ Code, Message string }

func (e *BPMNError) Error() string { return "bpmn error " + e.Code + ": " + e.Message }

// Handlers connect automated tasks to the outside world.
type Handlers interface {
	// Service runs serviceTask / sendTask connectors (cm:connector, cm:url ...).
	Service(ctx context.Context, n *bpmn.Node, vars map[string]any) (map[string]any, error)
	// Rule runs businessRuleTask decisions (rule engine execute API, cm:rule).
	Rule(ctx context.Context, n *bpmn.Node, vars map[string]any) (map[string]any, error)
}

type NopHandlers struct{}

func (NopHandlers) Service(context.Context, *bpmn.Node, map[string]any) (map[string]any, error) {
	return nil, nil
}
func (NopHandlers) Rule(context.Context, *bpmn.Node, map[string]any) (map[string]any, error) {
	return nil, nil
}

type Engine struct {
	H   Handlers
	Now func() time.Time
	// NewID generates unique ids (CUID in production).
	NewID func() string
}

func New(h Handlers, newID func() string) *Engine {
	if h == nil {
		h = NopHandlers{}
	}
	return &Engine{H: h, Now: time.Now, NewID: newID}
}

// run carries the state of one engine call.
type run struct {
	e   *Engine
	ctx context.Context
	p   *bpmn.Process
	in  *Instance
	fx  *Effects
	by  string
	q   []*Token // tokens ready to move
}

var ErrNotWaiting = errors.New("engine: token is not waiting for this trigger")

// Start creates a new instance at the given start event ("" = the none start).
func (e *Engine) Start(ctx context.Context, p *bpmn.Process, id, versionID, startNode string, vars map[string]any) (*Instance, *Effects, error) {
	in := &Instance{ID: id, ProcessID: p.ID, VersionID: versionID, Status: Running, Vars: map[string]any{}}
	for k, v := range vars {
		in.Vars[k] = v
	}
	r := e.newRun(ctx, p, in, "")
	var start *bpmn.Node
	for _, s := range p.StartNodes("") {
		if (startNode == "" && s.Event == bpmn.EvNone) || s.ID == startNode {
			start = s
		}
	}
	if start == nil {
		if ss := p.StartNodes(""); len(ss) == 1 && startNode == "" {
			start = ss[0]
		} else {
			return nil, nil, fmt.Errorf("engine: start event %q not found", startNode)
		}
	}
	r.log(start.ID, "enter", "")
	r.leave(&Token{ID: r.tid(), Node: start.ID})
	return in, r.fx, r.drain()
}

func (e *Engine) newRun(ctx context.Context, p *bpmn.Process, in *Instance, by string) *run {
	return &run{e: e, ctx: ctx, p: p, in: in, fx: &Effects{}, by: by}
}

func (r *run) tid() string { r.in.Seq++; return fmt.Sprintf("t%d", r.in.Seq) }

func (r *run) log(node, ev, tok string) {
	r.in.History = append(r.in.History, HistoryEntry{At: r.e.Now(), Node: node, Event: ev, Token: tok, By: r.by})
}

func (r *run) vars(t *Token) map[string]any {
	if len(t.Locals) == 0 {
		return r.in.Vars
	}
	m := make(map[string]any, len(r.in.Vars)+len(t.Locals))
	for k, v := range r.in.Vars {
		m[k] = v
	}
	for k, v := range t.Locals {
		m[k] = v
	}
	return m
}

func (r *run) merge(out map[string]any) {
	for k, v := range out {
		r.in.Vars[k] = v
	}
}

func (r *run) findToken(id string) *Token {
	for _, t := range r.in.Tokens {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (r *run) addWaiting(t *Token, w WaitKind) {
	t.Wait = w
	if r.findToken(t.ID) == nil {
		r.in.Tokens = append(r.in.Tokens, t)
	}
}

func (r *run) remove(t *Token) {
	for i, x := range r.in.Tokens {
		if x.ID == t.ID {
			r.in.Tokens = append(r.in.Tokens[:i], r.in.Tokens[i+1:]...)
			return
		}
	}
}

// drain processes queued tokens until all are waiting or consumed.
func (r *run) drain() error {
	for guard := 0; len(r.q) > 0; guard++ {
		if guard > 100000 {
			return errors.New("engine: step limit exceeded (infinite loop?)")
		}
		t := r.q[0]
		r.q = r.q[1:]
		if r.in.Status != Running {
			return nil
		}
		if err := r.enter(t); err != nil {
			var be *BPMNError
			if errors.As(err, &be) {
				if err := r.throwError(t, be.Code, be.Message); err != nil {
					return err
				}
				continue
			}
			// Technical failure: park the token as an incident instead of
			// failing the whole case. Operators retry, fix data or move it.
			r.raiseIncident(t, err)
		}
	}
	r.evalConditions()
	return nil
}

// leave moves token t along the outgoing flows of its node.
func (r *run) leave(t *Token) {
	n := r.p.Nodes[t.Node]
	r.log(n.ID, "leave", t.ID)
	outs := n.Outgoing
	if len(outs) == 0 {
		r.scopeTokenDone(t)
		return
	}
	// Activities: conditional outgoing flows behave like an inclusive split.
	for i, fid := range outs {
		f := r.p.Flows[fid]
		if bpmn.IsActivity(n.Type) && f.Condition != "" && fid != n.DefaultFlow {
			ok, _ := evalBool(f.Condition, r.vars(t))
			if !ok {
				continue
			}
		}
		nt := t
		if i > 0 {
			nt = &Token{ID: r.tid(), Scope: t.Scope, Locals: t.Locals}
		}
		nt.Node, nt.Wait = f.Target, WaitNone
		r.q = append(r.q, nt)
	}
}

func (r *run) take(t *Token, flowIDs []string) {
	for i, fid := range flowIDs {
		nt := t
		if i > 0 {
			nt = &Token{ID: r.tid(), Scope: t.Scope, Locals: t.Locals}
		}
		nt.Node, nt.Wait = r.p.Flows[fid].Target, WaitNone
		r.q = append(r.q, nt)
	}
}

func (r *run) enter(t *Token) error {
	n := r.p.Nodes[t.Node]
	if n == nil {
		return fmt.Errorf("engine: unknown node %s", t.Node)
	}
	r.log(n.ID, "enter", t.ID)

	if n.MultiInstance != nil && !t.MIInner && bpmn.IsActivity(n.Type) {
		return r.enterMulti(t, n)
	}

	switch n.Type {
	case bpmn.StartEvent:
		r.leave(t)

	case bpmn.EndEvent:
		return r.enterEnd(t, n)

	case bpmn.UserTask, bpmn.ManualTask, bpmn.Task:
		r.addWaiting(t, WaitUser)
		r.fx.NewWork = append(r.fx.NewWork, r.workItem(t, n))
		r.armBoundaries(t, n)

	case bpmn.ReceiveTask:
		t.Message = firstNonEmpty(n.MessageRef, n.Ext["message"], n.Name)
		r.addWaiting(t, WaitMessage)
		r.armBoundaries(t, n)

	case bpmn.ServiceTask, bpmn.SendTask, bpmn.BusinessRuleTask:
		if n.Type == bpmn.SendTask {
			r.fx.Outbound = append(r.fx.Outbound, Outbound{Kind: "message", Name: firstNonEmpty(n.MessageRef, n.Ext["message"], n.Name), Vars: r.vars(t)})
		}
		kind := JobService
		if n.Type == bpmn.BusinessRuleTask {
			kind = JobRule
		}
		if n.Ext["url"] == "" && n.Ext["rule"] == "" && n.Ext["connector"] == "" {
			r.complete(t) // nothing to call: pass-through step
			return nil
		}
		// External work never runs inside the engine step: it becomes a job
		// executed (and retried) by a worker, which reports back.
		r.addWaiting(t, WaitJob)
		r.armBoundaries(t, n)
		r.fx.NewJobs = append(r.fx.NewJobs, Job{InstanceID: r.in.ID, TokenID: t.ID, Node: n.ID, Kind: kind, Vars: copyVars(r.vars(t))})

	case bpmn.ScriptTask:
		out, err := runScript(n.Script, r.vars(t))
		if err != nil {
			return err
		}
		r.merge(out)
		r.complete(t)

	case bpmn.SubProcess, bpmn.Transaction, bpmn.AdHocSubProcess:
		r.addWaiting(t, WaitScope)
		r.armBoundaries(t, n)
		starts := r.p.StartNodes(n.ID)
		if len(starts) == 0 {
			return fmt.Errorf("engine: sub-process %s has no start event", n.ID)
		}
		for _, s := range starts {
			if s.Event == bpmn.EvNone {
				r.q = append(r.q, &Token{ID: r.tid(), Node: s.ID, Scope: t.ID, Locals: t.Locals})
			}
		}

	case bpmn.CallActivity:
		r.addWaiting(t, WaitCall)
		r.armBoundaries(t, n)
		r.fx.StartChildren = append(r.fx.StartChildren, StartChild{ParentID: r.in.ID, ParentToken: t.ID,
			ProcessKey: firstNonEmpty(n.CalledElem, n.Ext["processKey"]), Vars: copyVars(r.vars(t))})

	case bpmn.ExclusiveGateway:
		return r.exclusive(t, n)

	case bpmn.ParallelGateway:
		if len(n.Incoming) > 1 {
			if !r.join(t, n, len(n.Incoming)) {
				return nil
			}
		}
		r.take(t, n.Outgoing)

	case bpmn.InclusiveGateway:
		if len(n.Incoming) > 1 {
			if !r.inclusiveJoin(t, n) {
				return nil
			}
		}
		return r.inclusiveSplit(t, n)

	case bpmn.EventBasedGateway:
		r.addWaiting(t, WaitEventGW)
		for _, fid := range n.Outgoing {
			tgt := r.p.Nodes[r.p.Flows[fid].Target]
			if tgt.Event == bpmn.EvTimer {
				if due, err := r.due(tgt.TimerExpr); err == nil {
					r.fx.NewTimers = append(r.fx.NewTimers, Timer{InstanceID: r.in.ID, TokenID: t.ID, Node: tgt.ID, DueAt: due})
				}
			}
		}

	case bpmn.IntermediateCatchEvent:
		switch n.Event {
		case bpmn.EvTimer:
			due, err := r.due(n.TimerExpr)
			if err != nil {
				return err
			}
			r.addWaiting(t, WaitTimer)
			r.fx.NewTimers = append(r.fx.NewTimers, Timer{InstanceID: r.in.ID, TokenID: t.ID, Node: n.ID, DueAt: due})
		case bpmn.EvMessage:
			t.Message = firstNonEmpty(n.MessageRef, n.Name)
			r.addWaiting(t, WaitMessage)
		case bpmn.EvSignal:
			t.Signal = firstNonEmpty(n.SignalRef, n.Name)
			r.addWaiting(t, WaitSignal)
		case bpmn.EvConditional:
			if ok, _ := evalBool(n.ConditionExp, r.vars(t)); ok {
				r.leave(t)
			} else {
				t.Cond = n.ConditionExp
				r.addWaiting(t, WaitCondition)
			}
		case bpmn.EvLink: // a catch link is a pass-through target
			r.leave(t)
		default:
			return fmt.Errorf("engine: intermediate catch %s not executable", n.Event)
		}

	case bpmn.IntermediateThrowEvent:
		switch n.Event {
		case bpmn.EvLink:
			for _, c := range r.p.Nodes {
				if c.Type == bpmn.IntermediateCatchEvent && c.Event == bpmn.EvLink &&
					c.MessageRef == n.MessageRef && c.Parent == n.Parent {
					t.Node = c.ID
					r.q = append(r.q, t)
					return nil
				}
			}
			return fmt.Errorf("engine: link target %q not found", n.MessageRef)
		case bpmn.EvMessage:
			r.fx.Outbound = append(r.fx.Outbound, Outbound{Kind: "message", Name: firstNonEmpty(n.MessageRef, n.Name), Vars: r.vars(t)})
		case bpmn.EvSignal:
			r.fx.Outbound = append(r.fx.Outbound, Outbound{Kind: "signal", Name: firstNonEmpty(n.SignalRef, n.Name), Vars: r.vars(t)})
		case bpmn.EvEscalation:
			r.escalate(t, n.ErrorCode)
		}
		r.leave(t)

	default:
		return fmt.Errorf("engine: element %s (%s) is not executable yet", n.ID, n.Type)
	}
	return nil
}

// complete finishes an activity token: multi-instance inner tokens report to
// their container, everything else leaves normally.
func (r *run) complete(t *Token) {
	r.disarm(t)
	if t.MIInner {
		r.remove(t)
		c := r.findToken(t.Scope)
		if c == nil {
			return
		}
		c.MIOpen--
		n := r.p.Nodes[c.Node]
		if n.MultiInstance.Sequential && c.MINext < len(c.MIItems) {
			r.spawnInner(c, n)
			return
		}
		if c.MIOpen == 0 && c.MINext >= len(c.MIItems) {
			r.remove(c)
			r.leave(c)
		}
		return
	}
	r.remove(t)
	r.leave(t)
}

func (r *run) workItem(t *Token, n *bpmn.Node) WorkItem {
	w := WorkItem{ID: r.e.NewID(), InstanceID: r.in.ID, TokenID: t.ID, Node: n.ID,
		Name: firstNonEmpty(n.Name, n.ID), Role: n.Ext["role"], Assignee: n.Ext["assignee"],
		UI: UIMode(firstNonEmpty(n.Ext["ui"], string(UIForm))), FormKey: n.Ext["form"], URL: n.Ext["url"],
		SLA: n.Ext["sla"], Input: copyVars(t.Locals)}
	if w.Role == "" {
		if l := r.p.LaneOf(n.ID); l != nil {
			w.Role = firstNonEmpty(l.Role, l.Name)
		}
	}
	if strings.HasPrefix(w.Assignee, "${") {
		if v, err := evalAny(w.Assignee, r.vars(t)); err == nil {
			w.Assignee = fmt.Sprint(v)
		}
	}
	if w.SLA != "" {
		if d, err := ParseDuration(w.SLA); err == nil {
			due := r.e.Now().Add(d)
			w.DueAt = &due
		}
	}
	if n.Type == bpmn.ManualTask && w.UI == UIForm && w.FormKey == "" {
		w.UI = UINone
	}
	return w
}

func (r *run) enterMulti(t *Token, n *bpmn.Node) error {
	mi := n.MultiInstance
	var items []any
	if mi.Collection != "" {
		v, err := evalAny(mi.Collection, r.vars(t))
		if err != nil {
			return err
		}
		switch x := v.(type) {
		case []any:
			items = x
		case []string:
			for _, s := range x {
				items = append(items, s)
			}
		default:
			return fmt.Errorf("engine: multi-instance collection %q is not a list", mi.Collection)
		}
	} else if mi.Cardinality != "" {
		v, err := evalAny(mi.Cardinality, r.vars(t))
		if err != nil {
			return err
		}
		k, _ := toInt(v)
		for i := 0; i < k; i++ {
			items = append(items, i)
		}
	}
	if len(items) == 0 {
		r.leave(t)
		return nil
	}
	t.MIItems, t.MINext, t.MIOpen = items, 0, 0
	r.addWaiting(t, WaitMulti)
	r.armBoundaries(t, n)
	if mi.Sequential {
		r.spawnInner(t, n)
	} else {
		for range items {
			r.spawnInner(t, n)
		}
	}
	return nil
}

func (r *run) spawnInner(c *Token, n *bpmn.Node) {
	i := c.MINext
	c.MINext++
	c.MIOpen++
	loc := map[string]any{"loopCounter": i}
	for k, v := range c.Locals {
		loc[k] = v
	}
	if ev := n.MultiInstance.ElementVar; ev != "" {
		loc[ev] = c.MIItems[i]
	}
	r.q = append(r.q, &Token{ID: r.tid(), Node: n.ID, Scope: c.ID, Locals: loc, MIInner: true})
}

func (r *run) exclusive(t *Token, n *bpmn.Node) error {
	if len(n.Incoming) > 1 && len(n.Outgoing) == 1 { // merge
		r.take(t, n.Outgoing)
		return nil
	}
	for _, fid := range n.Outgoing {
		f := r.p.Flows[fid]
		if fid == n.DefaultFlow {
			continue
		}
		if f.Condition == "" {
			r.take(t, []string{fid})
			return nil
		}
		ok, err := evalBool(f.Condition, r.vars(t))
		if err != nil {
			return fmt.Errorf("engine: gateway %s flow %s: %w", n.ID, fid, err)
		}
		if ok {
			r.take(t, []string{fid})
			return nil
		}
	}
	if n.DefaultFlow != "" {
		r.take(t, []string{n.DefaultFlow})
		return nil
	}
	return fmt.Errorf("engine: exclusive gateway %s: no condition matched and no default flow", n.ID)
}

func (r *run) inclusiveSplit(t *Token, n *bpmn.Node) error {
	var chosen []string
	for _, fid := range n.Outgoing {
		f := r.p.Flows[fid]
		if fid == n.DefaultFlow {
			continue
		}
		ok := f.Condition == ""
		if !ok {
			var err error
			if ok, err = evalBool(f.Condition, r.vars(t)); err != nil {
				return err
			}
		}
		if ok {
			chosen = append(chosen, fid)
		}
	}
	if len(chosen) == 0 && n.DefaultFlow != "" {
		chosen = []string{n.DefaultFlow}
	}
	if len(chosen) == 0 {
		return fmt.Errorf("engine: inclusive gateway %s: no outgoing flow selected", n.ID)
	}
	r.take(t, chosen)
	return nil
}

// join parks t at gateway n; returns true (with t continuing) when `need`
// tokens of the same scope have arrived. Arrived tokens are merged into t.
func (r *run) join(t *Token, n *bpmn.Node, need int) bool {
	r.addWaiting(t, WaitJoin)
	var here []*Token
	for _, x := range r.in.Tokens {
		if x.Node == n.ID && x.Wait == WaitJoin && x.Scope == t.Scope {
			here = append(here, x)
		}
	}
	if len(here) < need {
		return false
	}
	for _, x := range here {
		r.remove(x)
	}
	t.Wait = WaitNone
	return true
}

func (r *run) inclusiveJoin(t *Token, n *bpmn.Node) bool {
	r.addWaiting(t, WaitJoin)
	// fire only when no other live token of the scope can still reach n
	for _, x := range r.in.Tokens {
		if x.Scope != t.Scope || x.Node == n.ID {
			continue
		}
		if r.canReach(x.Node, n.ID) {
			return false
		}
	}
	for _, x := range r.q {
		if x.Scope == t.Scope && x.Node != n.ID && r.canReach(x.Node, n.ID) {
			return false
		}
	}
	for _, x := range append([]*Token(nil), r.in.Tokens...) {
		if x.Node == n.ID && x.Wait == WaitJoin && x.Scope == t.Scope {
			r.remove(x)
		}
	}
	t.Wait = WaitNone
	return true
}

func (r *run) canReach(from, to string) bool {
	seen := map[string]bool{}
	stack := []string{from}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, fid := range r.p.Nodes[id].Outgoing {
			tg := r.p.Flows[fid].Target
			if tg == to {
				return true
			}
			stack = append(stack, tg)
		}
	}
	return false
}

func (r *run) enterEnd(t *Token, n *bpmn.Node) error {
	switch n.Event {
	case bpmn.EvTerminate:
		if t.Scope == "" {
			r.cancelScope("")
			r.finishInstance()
			return nil
		}
		c := r.findToken(t.Scope)
		r.cancelScope(t.Scope)
		if c != nil {
			r.remove(c)
			r.disarm(c)
			r.leave(c)
		}
		return nil
	case bpmn.EvError:
		return r.throwError(t, firstNonEmpty(n.ErrorCode, n.ID), n.Name)
	case bpmn.EvMessage:
		r.fx.Outbound = append(r.fx.Outbound, Outbound{Kind: "message", Name: firstNonEmpty(n.MessageRef, n.Name), Vars: r.vars(t)})
	case bpmn.EvSignal:
		r.fx.Outbound = append(r.fx.Outbound, Outbound{Kind: "signal", Name: firstNonEmpty(n.SignalRef, n.Name), Vars: r.vars(t)})
	case bpmn.EvEscalation:
		r.escalate(t, n.ErrorCode)
	}
	r.scopeTokenDone(t)
	return nil
}

// scopeTokenDone consumes t and completes its scope when it was the last token.
func (r *run) scopeTokenDone(t *Token) {
	r.remove(t)
	for _, x := range r.in.Tokens {
		if x.Scope == t.Scope {
			return
		}
	}
	for _, x := range r.q {
		if x.Scope == t.Scope {
			return
		}
	}
	if t.Scope == "" {
		r.finishInstance()
		return
	}
	if c := r.findToken(t.Scope); c != nil {
		if c.Wait == WaitMulti { // inner token of a multi-instance sub-process
			return
		}
		r.complete(c)
	}
}

func (r *run) finishInstance() {
	r.in.Status = Completed
	r.in.Tokens = nil
	r.q = nil
	r.fx.Completed = true
}

// cancelScope removes every token (recursively) inside container scope.
func (r *run) cancelScope(scope string) {
	for _, x := range append([]*Token(nil), r.in.Tokens...) {
		if x.Scope == scope {
			r.cancelToken(x)
		}
	}
	var q []*Token
	for _, x := range r.q {
		if x.Scope != scope {
			q = append(q, x)
		}
	}
	r.q = q
}

func (r *run) cancelToken(t *Token) {
	r.cancelScope(t.ID)
	r.disarm(t)
	if t.Wait == WaitCall {
		r.fx.CancelChild = append(r.fx.CancelChild, t.ID)
	}
	r.remove(t)
	r.log(t.Node, "cancel", t.ID)
}

// armBoundaries schedules timers for boundary events of a waiting activity.
func (r *run) armBoundaries(t *Token, n *bpmn.Node) {
	if t.MIInner {
		return // boundaries belong to the multi-instance container
	}
	for _, b := range r.p.Boundaries(n.ID) {
		if b.Event == bpmn.EvTimer {
			if due, err := r.due(b.TimerExpr); err == nil {
				r.fx.NewTimers = append(r.fx.NewTimers, Timer{InstanceID: r.in.ID, TokenID: t.ID, Node: b.ID, DueAt: due})
			}
		}
	}
}

func (r *run) disarm(t *Token) {
	r.fx.CancelWork = append(r.fx.CancelWork, t.ID)
	r.fx.CancelTimers = append(r.fx.CancelTimers, t.ID)
}

// fireBoundary triggers boundary event b attached to the activity held by t.
func (r *run) fireBoundary(t *Token, b *bpmn.Node) {
	r.log(b.ID, "enter", t.ID)
	nt := &Token{ID: r.tid(), Node: b.ID, Scope: t.Scope, Locals: t.Locals}
	if b.CancelActivity {
		r.cancelToken(t)
	}
	r.leave(nt)
}

// throwError propagates a BPMN error outward from token t.
func (r *run) throwError(t *Token, code, msg string) error {
	r.log(t.Node, "error", t.ID)
	cur := t
	for {
		// boundary on the activity the token sits on (a task raising the
		// error) or on an enclosing sub-process / multi-instance container
		if bpmn.IsActivity(r.p.Nodes[cur.Node].Type) {
			if b := r.matchBoundary(cur.Node, bpmn.EvError, code); b != nil {
				r.fireBoundary(cur, b)
				return nil
			}
		}
		if cur == t {
			r.remove(t)
		}
		if cur.Scope == "" {
			r.cancelScope("")
			r.in.Status, r.in.Error = Failed, fmt.Sprintf("uncaught BPMN error %s %s", code, msg)
			r.q = nil
			return nil
		}
		c := r.findToken(cur.Scope)
		if c == nil {
			return fmt.Errorf("engine: lost scope %s", cur.Scope)
		}
		cur = c
	}
}

func (r *run) escalate(t *Token, code string) {
	for s := t.Scope; s != ""; {
		c := r.findToken(s)
		if c == nil {
			return
		}
		if b := r.matchBoundary(c.Node, bpmn.EvEscalation, code); b != nil {
			r.fireBoundary(c, b)
			return
		}
		s = c.Scope
	}
}

func (r *run) matchBoundary(activity string, ev bpmn.EventKind, code string) *bpmn.Node {
	var catchAll *bpmn.Node
	for _, b := range r.p.Boundaries(activity) {
		if b.Event != ev {
			continue
		}
		if b.ErrorCode == code {
			return b
		}
		if b.ErrorCode == "" {
			catchAll = b
		}
	}
	return catchAll
}

func (r *run) evalConditions() {
	for again := true; again && r.in.Status == Running; {
		again = false
		for _, t := range append([]*Token(nil), r.in.Tokens...) {
			if t.Wait == WaitCondition {
				if ok, _ := evalBool(t.Cond, r.vars(t)); ok {
					r.remove(t)
					t.Cond = ""
					r.leave(t)
					again = true
				}
			}
		}
		if again {
			_ = r.drainNoCond()
		}
	}
}

func (r *run) drainNoCond() error {
	q := r.q
	r.q = nil
	for len(q) > 0 {
		t := q[0]
		q = q[1:]
		if err := r.enter(t); err != nil {
			return err
		}
		q = append(q, r.q...)
		r.q = nil
	}
	return nil
}

func (r *run) due(expr string) (time.Time, error) {
	kind, val, _ := strings.Cut(expr, ":")
	if val == "" {
		kind, val = "timeDuration", expr
	}
	if strings.HasPrefix(val, "${") {
		v, err := evalAny(val, r.in.Vars)
		if err != nil {
			return time.Time{}, err
		}
		val = fmt.Sprint(v)
	}
	switch kind {
	case "timeDate":
		return time.Parse(time.RFC3339, val)
	case "timeCycle": // R3/PT10M -> first occurrence
		if i := strings.LastIndex(val, "/"); i >= 0 {
			val = val[i+1:]
		}
	}
	d, err := ParseDuration(val)
	if err != nil {
		return time.Time{}, err
	}
	return r.e.Now().Add(d), nil
}

func (r *run) raiseIncident(t *Token, err error) {
	t.Error = err.Error()
	r.addWaiting(t, WaitIncident)
	r.log(t.Node, "incident", t.ID)
	r.fx.Incidents = append(r.fx.Incidents, Incident{InstanceID: r.in.ID, TokenID: t.ID, Node: t.Node, Message: err.Error()})
}

// ---------- external triggers ----------

// CompleteJob reports a successful job (service / rule task) with its output.
func (e *Engine) CompleteJob(ctx context.Context, p *bpmn.Process, in *Instance, tokenID string, out map[string]any) (*Effects, error) {
	r, err := e.load(ctx, p, in, "system:job")
	if err != nil {
		return nil, err
	}
	t := r.findToken(tokenID)
	if t == nil || t.Wait != WaitJob {
		return nil, ErrNotWaiting
	}
	r.merge(out)
	r.complete(t)
	return r.fx, r.drain()
}

// FailJob reports a job that gave up. A *BPMNError is thrown into the model
// (catchable by boundary events); anything else becomes an incident.
func (e *Engine) FailJob(ctx context.Context, p *bpmn.Process, in *Instance, tokenID string, jobErr error) (*Effects, error) {
	r, err := e.load(ctx, p, in, "system:job")
	if err != nil {
		return nil, err
	}
	t := r.findToken(tokenID)
	if t == nil || t.Wait != WaitJob {
		return nil, ErrNotWaiting
	}
	var be *BPMNError
	if errors.As(jobErr, &be) {
		r.disarm(t)
		if err := r.throwError(t, be.Code, be.Message); err != nil {
			return nil, err
		}
		return r.fx, r.drain()
	}
	r.raiseIncident(t, jobErr)
	return r.fx, nil
}

// Retry re-executes the step of a token parked as an incident.
func (e *Engine) Retry(ctx context.Context, p *bpmn.Process, in *Instance, tokenID, by string) (*Effects, error) {
	r, err := e.load(ctx, p, in, by)
	if err != nil {
		return nil, err
	}
	t := r.findToken(tokenID)
	if t == nil || t.Wait != WaitIncident {
		return nil, ErrNotWaiting
	}
	r.remove(t)
	t.Wait, t.Error = WaitNone, ""
	r.q = append(r.q, t)
	return r.fx, r.drain()
}

func (e *Engine) load(ctx context.Context, p *bpmn.Process, in *Instance, by string) (*run, error) {
	if in.Status != Running {
		return nil, fmt.Errorf("engine: instance %s is %s", in.ID, in.Status)
	}
	return e.newRun(ctx, p, in, by), nil
}

// CompleteTask completes the user/manual task held by token tokenID.
func (e *Engine) CompleteTask(ctx context.Context, p *bpmn.Process, in *Instance, tokenID, by string, out map[string]any) (*Effects, error) {
	r, err := e.load(ctx, p, in, by)
	if err != nil {
		return nil, err
	}
	t := r.findToken(tokenID)
	if t == nil || t.Wait != WaitUser {
		return nil, ErrNotWaiting
	}
	r.merge(out)
	r.complete(t)
	return r.fx, r.drain()
}

// FireTimer handles a due timer (catch event, boundary or event gateway).
func (e *Engine) FireTimer(ctx context.Context, p *bpmn.Process, in *Instance, tm Timer) (*Effects, error) {
	r, err := e.load(ctx, p, in, "system:timer")
	if err != nil {
		return nil, err
	}
	t := r.findToken(tm.TokenID)
	if t == nil {
		return r.fx, nil // token already moved on; stale timer
	}
	n := p.Nodes[tm.Node]
	switch {
	case n.Type == bpmn.BoundaryEvent:
		r.fireBoundary(t, n)
	case t.Wait == WaitEventGW:
		r.resolveEventGW(t, n.ID)
	case t.Wait == WaitTimer && t.Node == n.ID:
		r.remove(t)
		r.leave(t)
	default:
		return r.fx, nil
	}
	return r.fx, r.drain()
}

func (r *run) resolveEventGW(t *Token, catchNode string) {
	r.remove(t)
	r.fx.CancelTimers = append(r.fx.CancelTimers, t.ID)
	t.Node, t.Wait = catchNode, WaitNone
	r.leave(t)
}

// Message correlates a message to waiting receive tasks, catch events,
// event-gateway branches and boundary message events. Returns matched count.
func (e *Engine) Message(ctx context.Context, p *bpmn.Process, in *Instance, name string, vars map[string]any) (*Effects, int, error) {
	return e.trigger(ctx, p, in, bpmn.EvMessage, name, vars)
}

// Signal is like Message but delivered to every waiting catcher.
func (e *Engine) Signal(ctx context.Context, p *bpmn.Process, in *Instance, name string, vars map[string]any) (*Effects, int, error) {
	return e.trigger(ctx, p, in, bpmn.EvSignal, name, vars)
}

func (e *Engine) trigger(ctx context.Context, p *bpmn.Process, in *Instance, ev bpmn.EventKind, name string, vars map[string]any) (*Effects, int, error) {
	r, err := e.load(ctx, p, in, "system:"+string(ev))
	if err != nil {
		return nil, 0, err
	}
	matched := 0
	for _, t := range append([]*Token(nil), in.Tokens...) {
		if r.findToken(t.ID) == nil {
			continue
		}
		hit := false
		switch {
		case ev == bpmn.EvMessage && t.Wait == WaitMessage && t.Message == name,
			ev == bpmn.EvSignal && t.Wait == WaitSignal && t.Signal == name:
			r.remove(t)
			r.disarm(t)
			t.Message, t.Signal = "", ""
			if p.Nodes[t.Node].Type == bpmn.ReceiveTask {
				r.leave(t)
			} else {
				r.leave(t)
			}
			hit = true
		case t.Wait == WaitEventGW:
			for _, fid := range p.Nodes[t.Node].Outgoing {
				tg := p.Nodes[p.Flows[fid].Target]
				ref := tg.MessageRef
				if ev == bpmn.EvSignal {
					ref = tg.SignalRef
				}
				if tg.Event == ev && firstNonEmpty(ref, tg.Name) == name {
					r.resolveEventGW(t, tg.ID)
					hit = true
					break
				}
			}
		}
		if !hit {
			for _, b := range p.Boundaries(t.Node) {
				ref := b.MessageRef
				if ev == bpmn.EvSignal {
					ref = b.SignalRef
				}
				if b.Event == ev && firstNonEmpty(ref, b.Name) == name && !t.MIInner {
					r.fireBoundary(t, b)
					hit = true
					break
				}
			}
		}
		if hit {
			matched++
			if ev == bpmn.EvMessage {
				break // a message is consumed once
			}
		}
	}
	if matched > 0 {
		r.merge(vars)
	}
	return r.fx, matched, r.drain()
}

// ChildCompleted resumes a call activity when its child case finishes.
func (e *Engine) ChildCompleted(ctx context.Context, p *bpmn.Process, in *Instance, tokenID string, out map[string]any) (*Effects, error) {
	r, err := e.load(ctx, p, in, "system:child")
	if err != nil {
		return nil, err
	}
	t := r.findToken(tokenID)
	if t == nil || t.Wait != WaitCall {
		return nil, ErrNotWaiting
	}
	r.merge(out)
	r.complete(t)
	return r.fx, r.drain()
}

// Cancel stops an instance and every waiting token.
func (e *Engine) Cancel(ctx context.Context, p *bpmn.Process, in *Instance, by string) *Effects {
	r := e.newRun(ctx, p, in, by)
	r.cancelScope("")
	in.Status = Cancelled
	return r.fx
}

// MoveToken relocates a waiting token (used by version migration and by
// supervisors to send a case back to an earlier step).
func (e *Engine) MoveToken(ctx context.Context, p *bpmn.Process, in *Instance, tokenID, toNode, by string) (*Effects, error) {
	r, err := e.load(ctx, p, in, by)
	if err != nil {
		return nil, err
	}
	t := r.findToken(tokenID)
	if t == nil {
		return nil, ErrNotWaiting
	}
	if p.Nodes[toNode] == nil {
		return nil, fmt.Errorf("engine: unknown node %s", toNode)
	}
	r.cancelToken(t)
	r.log(toNode, "migrate", t.ID)
	r.q = append(r.q, &Token{ID: r.tid(), Node: toNode, Scope: t.Scope, Locals: t.Locals})
	return r.fx, r.drain()
}

func copyVars(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	o := make(map[string]any, len(m))
	for k, v := range m {
		o[k] = v
	}
	return o
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	}
	return 0, false
}

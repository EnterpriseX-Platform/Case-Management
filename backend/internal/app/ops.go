package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

// Counters exported on /metrics.
var Metrics struct {
	Steps, JobsDone, JobsRetried, JobsFailed, Incidents, Conflicts, TimersFired atomic.Int64
}

// tx runs fn in a transaction and transparently re-runs it when another
// writer changed the same case first (optimistic locking).
func (s *Service) tx(ctx context.Context, fn func(ctx context.Context) error) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = s.S.Tx(ctx, fn)
		if !errors.Is(err, store.ErrStale) {
			return err
		}
		Metrics.Conflicts.Add(1)
		time.Sleep(time.Duration(attempt*attempt*5) * time.Millisecond)
	}
	return conflict("case is busy, try again")
}

// applyOps persists jobs and incidents requested by the engine and drops
// jobs of tokens that no longer exist.
func (s *Service) applyOps(ctx context.Context, c *store.Case, p *bpmn.Process, fx *engine.Effects) error {
	in := c.Instance
	now := s.Now()
	for _, t := range fx.CancelTimers { // disarm also covers jobs of the token
		if !tokenAlive(in, t) {
			if err := s.S.DeleteJobsByToken(ctx, in.ID, t); err != nil {
				return err
			}
		}
	}
	for _, j := range fx.NewJobs {
		max := 5
		if n := p.Nodes[j.Node]; n != nil {
			fmt.Sscan(n.Ext["retries"], &max)
		}
		rec := &store.JobRec{ID: id.New(), Job: j, CaseID: c.ID, Status: store.JobPending, MaxAttempts: max, NextRunAt: now, CreatedAt: now}
		if err := s.S.PutJob(ctx, rec); err != nil {
			return err
		}
	}
	for _, inc := range fx.Incidents {
		name := inc.Node
		if n := p.Nodes[inc.Node]; n != nil && n.Name != "" {
			name = n.Name
		}
		rec := &store.IncidentRec{ID: id.New(), CaseID: c.ID, CaseNumber: c.Number, ProcessKey: c.ProcessKey, TokenID: inc.TokenID,
			Node: inc.Node, NodeName: name, Message: inc.Message, Status: store.IncidentOpen, CreatedAt: now}
		if err := s.S.PutIncident(ctx, rec); err != nil {
			return err
		}
		Metrics.Incidents.Add(1)
	}
	return nil
}

// persist writes the case with optimistic locking, moves the engine history
// into the append-only event log, refreshes the subscription index and
// closes incidents whose token was moved or cancelled.
func (s *Service) persist(ctx context.Context, c *store.Case, p *bpmn.Process) error {
	in := c.Instance
	if len(in.History) > 0 {
		ev := make([]store.EventRec, 0, len(in.History))
		for _, h := range in.History {
			ev = append(ev, store.EventRec{CaseID: c.ID, At: h.At, Type: h.Event, Node: h.Node, Token: h.Token, By: h.By})
		}
		Metrics.Steps.Add(int64(len(ev)))
		if err := s.S.AppendEvents(ctx, ev); err != nil {
			return err
		}
		in.History = nil
	}
	if err := s.S.PutCase(ctx, c); err != nil {
		return err
	}
	if err := s.S.ReplaceSubscriptions(ctx, c.ID, subscriptions(c, p)); err != nil {
		return err
	}
	open, err := s.S.ListIncidents(ctx, store.IncidentOpen, c.ID)
	if err != nil {
		return err
	}
	for _, inc := range open {
		if t := findToken(in, inc.TokenID); t == nil || t.Wait != engine.WaitIncident {
			now := s.Now()
			inc.Status, inc.ResolvedAt = store.IncidentResolved, &now
			if inc.Resolution == "" {
				inc.Resolution = "cleared"
				if in.Status == engine.Cancelled {
					inc.Resolution = "cancelled"
				}
			}
			if err := s.S.PutIncident(ctx, inc); err != nil {
				return err
			}
		}
	}
	return nil
}

func findToken(in *engine.Instance, id string) *engine.Token {
	for _, t := range in.Tokens {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// subscriptions lists every message / signal the case currently waits for.
func subscriptions(c *store.Case, p *bpmn.Process) []store.Subscription {
	if c.Instance.Status != engine.Running {
		return nil
	}
	var out []store.Subscription
	add := func(tok, kind, name string) {
		if name != "" {
			out = append(out, store.Subscription{CaseID: c.ID, TokenID: tok, Kind: kind, Name: name})
		}
	}
	ref := func(n *bpmn.Node) (string, string) {
		switch n.Event {
		case bpmn.EvMessage:
			return "message", firstNonEmpty(n.MessageRef, n.Name)
		case bpmn.EvSignal:
			return "signal", firstNonEmpty(n.SignalRef, n.Name)
		}
		return "", ""
	}
	for _, t := range c.Instance.Tokens {
		switch t.Wait {
		case engine.WaitMessage:
			add(t.ID, "message", t.Message)
		case engine.WaitSignal:
			add(t.ID, "signal", t.Signal)
		case engine.WaitEventGW:
			for _, fid := range p.Nodes[t.Node].Outgoing {
				if k, n := ref(p.Nodes[p.Flows[fid].Target]); k != "" {
					add(t.ID, k, n)
				}
			}
		}
		if !t.MIInner {
			for _, b := range p.Boundaries(t.Node) {
				if k, n := ref(b); k != "" {
					add(t.ID, k, n)
				}
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- jobs

// RunJobs leases due jobs and executes them. External calls happen outside
// any database transaction; the result is applied to the case afterwards.
// Connectors receive the job id as idempotency key, so a job re-run after a
// crash can be de-duplicated by the remote system.
func (s *Service) RunJobs(ctx context.Context, worker string, limit int) (int, error) {
	jobs, err := s.S.LeaseJobs(ctx, worker, s.Now(), 2*time.Minute, limit)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		if err := s.runJob(ctx, j); err != nil {
			return 0, err
		}
	}
	return len(jobs), nil
}

func (s *Service) runJob(ctx context.Context, j *store.JobRec) error {
	c, err := s.S.GetCase(ctx, j.CaseID)
	if err != nil {
		return s.S.DeleteJob(ctx, j.ID)
	}
	p, _, err := s.Process(ctx, c.VersionID)
	if err != nil {
		return err
	}
	n := p.Nodes[j.Node]
	if n == nil {
		return s.S.DeleteJob(ctx, j.ID)
	}
	jctx := WithIdempotencyKey(ctx, j.ID)
	var out map[string]any
	var callErr error
	if j.Kind == engine.JobRule {
		out, callErr = s.E.H.Rule(jctx, n, j.Vars)
	} else {
		out, callErr = s.E.H.Service(jctx, n, j.Vars)
	}
	var be *engine.BPMNError
	if callErr != nil && !errors.As(callErr, &be) && j.Attempts+1 < j.MaxAttempts {
		j.Attempts++
		j.Status, j.LastError, j.LeaseUntil = store.JobPending, callErr.Error(), nil
		j.NextRunAt = s.Now().Add(backoff(j.Attempts))
		Metrics.JobsRetried.Add(1)
		return s.S.PutJob(ctx, j)
	}
	err = s.tx(ctx, func(ctx context.Context) error {
		c, err := s.S.GetCase(ctx, j.CaseID)
		if err != nil {
			return err
		}
		if c.Instance.Status != engine.Running {
			return s.S.DeleteJob(ctx, j.ID)
		}
		var fx *engine.Effects
		if callErr == nil {
			fx, err = s.E.CompleteJob(ctx, p, c.Instance, j.TokenID, out)
			Metrics.JobsDone.Add(1)
		} else {
			if be != nil {
				fx, err = s.E.FailJob(ctx, p, c.Instance, j.TokenID, be)
			} else {
				fx, err = s.E.FailJob(ctx, p, c.Instance, j.TokenID, fmt.Errorf("%w (after %d attempts)", callErr, j.Attempts+1))
			}
			Metrics.JobsFailed.Add(1)
		}
		if errors.Is(err, engine.ErrNotWaiting) { // token moved on meanwhile
			return s.S.DeleteJob(ctx, j.ID)
		}
		if err != nil {
			return err
		}
		if err := s.S.DeleteJob(ctx, j.ID); err != nil {
			return err
		}
		return s.apply(ctx, Actor{ID: "system:job"}, c, p, fx)
	})
	return err
}

func backoff(attempt int) time.Duration {
	d := time.Duration(math.Pow(2, float64(attempt))) * time.Second
	if d > 10*time.Minute {
		d = 10 * time.Minute
	}
	return d
}

type idemKey struct{}

func WithIdempotencyKey(ctx context.Context, k string) context.Context {
	return context.WithValue(ctx, idemKey{}, k)
}

func IdempotencyKey(ctx context.Context) string { s, _ := ctx.Value(idemKey{}).(string); return s }

// ---------------------------------------------------------------- incidents

// ResolveIncident lets an operator fix a stuck step: retry it (optionally
// with corrected variables), move the token to another step, or cancel the case.
func (s *Service) ResolveIncident(ctx context.Context, by Actor, incidentID, action, toNode string, vars map[string]any, note string) (*store.IncidentRec, error) {
	if !by.Has(RoleSupervisor) && !by.Has(RoleAdmin) {
		return nil, ErrForbidden
	}
	inc, err := s.S.GetIncident(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	if inc.Status != store.IncidentOpen {
		return nil, conflict("incident is already %s", inc.Status)
	}
	_, err = s.withCase(ctx, inc.CaseID, func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error) {
		for k, v := range vars {
			c.Instance.Vars[k] = v
		}
		s.audit(ctx, by, "incident."+action, "case", c.ID, map[string]any{"incident": inc.ID, "node": inc.Node, "to": toNode, "vars": vars, "note": note})
		mark := func(res string) {
			now := s.Now()
			inc.Status, inc.ResolvedAt, inc.ResolvedBy, inc.Resolution = store.IncidentResolved, &now, by.ID, res
			_ = s.S.PutIncident(ctx, inc)
		}
		switch action {
		case "retry":
			mark("retried")
			return s.E.Retry(ctx, p, c.Instance, inc.TokenID, by.ID)
		case "move":
			mark("moved")
			return s.E.MoveToken(ctx, p, c.Instance, inc.TokenID, toNode, by.ID)
		case "cancel":
			mark("cancelled")
			return s.E.Cancel(ctx, p, c.Instance, by.ID), nil
		}
		return nil, invalid("unknown action %q", action)
	}, by)
	if err != nil {
		return nil, err
	}
	return s.S.GetIncident(ctx, incidentID)
}

// RetryAll retries every open incident (e.g. after a remote system recovered).
func (s *Service) RetryAll(ctx context.Context, by Actor) (int, error) {
	open, err := s.S.ListIncidents(ctx, store.IncidentOpen, "")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, inc := range open {
		if _, err := s.ResolveIncident(ctx, by, inc.ID, "retry", "", nil, "bulk retry"); err == nil {
			n++
		}
	}
	return n, nil
}

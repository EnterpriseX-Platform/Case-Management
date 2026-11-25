package app

import (
	"context"
	"fmt"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

// MigrationRequest moves running cases from one version to another.
// Mapping maps source step id -> target step id; steps whose id exists in the
// target version map to themselves automatically.
type MigrationRequest struct {
	From    string            `json:"from"`
	To      string            `json:"to"`
	Mapping map[string]string `json:"mapping"`
	CaseIDs []string          `json:"caseIds,omitempty"` // empty = all running cases of From
	DryRun  bool              `json:"dryRun"`
}

type CaseMigration struct {
	CaseID string            `json:"caseId"`
	Number string            `json:"number"`
	Moves  map[string]string `json:"moves"` // from node -> to node
	Error  string            `json:"error,omitempty"`
}

type MigrationResult struct {
	JobID      string          `json:"jobId,omitempty"`
	DryRun     bool            `json:"dryRun"`
	Diff       *Diff           `json:"diff"`
	Unmapped   []string        `json:"unmapped"` // removed steps that still hold cases
	Cases      []CaseMigration `json:"cases"`
	OK, Failed int
}

func (s *Service) Migrate(ctx context.Context, by Actor, req MigrationRequest) (*MigrationResult, error) {
	if !by.Has(RoleAdmin) && !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	src, sv, err := s.Process(ctx, req.From)
	if err != nil {
		return nil, err
	}
	dst, dv, err := s.Process(ctx, req.To)
	if err != nil {
		return nil, err
	}
	if sv.ProcessKey != dv.ProcessKey {
		return nil, invalid("versions belong to different case types")
	}
	if dv.Status != store.Published && dv.Status != store.Approved {
		return nil, invalid("target version must be approved or published")
	}
	res := &MigrationResult{DryRun: req.DryRun, Diff: DiffProcesses(req.From, req.To, src, dst), Unmapped: []string{}}
	cases, err := s.S.ListCases(ctx, sv.ProcessKey, req.From, 0)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, c := range req.CaseIDs {
		want[c] = true
	}
	target := func(node string) (string, bool) {
		if t, ok := req.Mapping[node]; ok {
			return t, dst.Nodes[t] != nil
		}
		if n := dst.Nodes[node]; n != nil && n.Type == src.Nodes[node].Type {
			return node, true
		}
		return "", false
	}
	unm := map[string]bool{}
	var todo []*store.Case
	for _, c := range cases {
		if c.Instance == nil || c.Instance.Status != engine.Running || (len(want) > 0 && !want[c.ID]) {
			continue
		}
		cm := CaseMigration{CaseID: c.ID, Number: c.Number, Moves: map[string]string{}}
		for _, t := range c.Instance.Tokens {
			to, ok := target(t.Node)
			if !ok {
				unm[t.Node] = true
				cm.Error = fmt.Sprintf("step %s has no mapping", t.Node)
				continue
			}
			cm.Moves[t.Node] = to
		}
		res.Cases = append(res.Cases, cm)
		todo = append(todo, c)
	}
	for n := range unm {
		res.Unmapped = append(res.Unmapped, n)
	}
	if req.DryRun || len(unm) > 0 {
		for _, c := range res.Cases {
			if c.Error != "" {
				res.Failed++
			} else {
				res.OK++
			}
		}
		res.DryRun = true
		return res, nil
	}
	res.JobID = id.New()
	for i, c := range todo {
		err := s.S.Tx(ctx, func(ctx context.Context) error {
			before := *c.Instance
			s.audit(ctx, by, "case.migrate", "migration", res.JobID, map[string]any{"case": c.ID, "fromVersion": req.From, "snapshot": before})
			return s.migrateCase(ctx, by, c, dst, req.To, res.Cases[i].Moves)
		})
		if err != nil {
			res.Cases[i].Error = err.Error()
			res.Failed++
		} else {
			res.OK++
		}
	}
	s.audit(ctx, by, "migration.run", "version", req.To, map[string]any{"job": res.JobID, "ok": res.OK, "failed": res.Failed})
	return res, nil
}

func (s *Service) migrateCase(ctx context.Context, by Actor, c *store.Case, dst *bpmn.Process, toVersion string, moves map[string]string) error {
	in := c.Instance
	in.VersionID, c.VersionID = toVersion, toVersion
	var all *engine.Effects
	for _, t := range append([]*engine.Token(nil), in.Tokens...) {
		to := moves[t.Node]
		if to == t.Node {
			in.History = append(in.History, engine.HistoryEntry{At: s.Now(), Node: t.Node, Event: "migrate", Token: t.ID, By: by.ID})
			continue // same step exists: keep token and its open work item
		}
		fx, err := s.E.MoveToken(ctx, dst, in, t.ID, to, by.ID)
		if err != nil {
			return err
		}
		all = mergeFx(all, fx)
	}
	if all == nil {
		return s.S.PutCase(ctx, c)
	}
	return s.apply(ctx, by, c, dst, all)
}

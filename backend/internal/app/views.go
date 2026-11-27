package app

// Saved work list views. Everyone can keep personal views; supervisors can
// share a view with roles and make it the default for them.

import (
	"context"
	"sort"
	"strings"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

// Views returns the caller's own views and the views shared with them.
func (s *Service) Views(ctx context.Context, by Actor) ([]*store.WorkView, error) {
	vs, err := s.S.ListViews(ctx)
	if err != nil {
		return nil, err
	}
	out := []*store.WorkView{}
	for _, v := range vs {
		if v.Owner == by.ID || (v.Shared && (len(v.Roles) == 0 || anyRole(by, v.Roles))) {
			out = append(out, v)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Shared != out[j].Shared {
			return !out[i].Shared
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// SaveView creates or updates a view. Only its owner (or a supervisor for a
// shared view) may change it; only supervisors may share.
func (s *Service) SaveView(ctx context.Context, by Actor, v *store.WorkView) (*store.WorkView, error) {
	if strings.TrimSpace(v.Name) == "" {
		return nil, invalid("a view needs a name")
	}
	if len(v.Columns) == 0 {
		return nil, invalid("a view needs at least one column")
	}
	if v.Shared && !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	if v.ID == "" {
		v.ID, v.Owner = id.New(), by.ID
	} else {
		vs, err := s.S.ListViews(ctx)
		if err != nil {
			return nil, err
		}
		var old *store.WorkView
		for _, x := range vs {
			if x.ID == v.ID {
				old = x
			}
		}
		if old == nil {
			v.Owner = by.ID
		} else {
			if old.Owner != by.ID && !(old.Shared && by.Has(RoleSupervisor)) {
				return nil, ErrForbidden
			}
			v.Owner = old.Owner
		}
	}
	if !v.Shared {
		v.Roles, v.Default = nil, false
	}
	v.UpdatedAt = s.Now()
	if err := s.S.PutView(ctx, v); err != nil {
		return nil, err
	}
	s.audit(ctx, by, "view.save", "view", v.ID, map[string]any{"name": v.Name, "shared": v.Shared})
	return v, nil
}

func (s *Service) DeleteView(ctx context.Context, by Actor, viewID string) error {
	vs, err := s.S.ListViews(ctx)
	if err != nil {
		return err
	}
	for _, v := range vs {
		if v.ID == viewID {
			if v.Owner != by.ID && !(v.Shared && by.Has(RoleSupervisor)) {
				return ErrForbidden
			}
			return s.S.DeleteView(ctx, viewID)
		}
	}
	return store.ErrNotFound
}

// WorkFields lists case data fields that can be shown as work list columns,
// from the published versions of every case type.
func (s *Service) WorkFields(ctx context.Context) ([]store.DataField, error) {
	ps, err := s.S.ListProcesses(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []store.DataField{}
	for _, p := range ps {
		vs, err := s.S.ListVersions(ctx, p.Key)
		if err != nil {
			continue
		}
		for _, v := range vs {
			if v.Status != store.Published || v.Model == nil {
				continue
			}
			for _, f := range v.Model.Data {
				if !seen[f.Key] && f.Type != "list" {
					seen[f.Key] = true
					out = append(out, f)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}

// WithCaseData adds the requested case data fields to work items.
func (s *Service) WithCaseData(ctx context.Context, ws []*store.Work, fields []string) []map[string]any {
	out := make([]map[string]any, 0, len(ws))
	cases := map[string]map[string]any{}
	for _, w := range ws {
		vars, ok := cases[w.CaseID]
		if !ok {
			if c, err := s.S.GetCase(ctx, w.CaseID); err == nil {
				vars = map[string]any{}
				for _, f := range fields {
					if v, ok := c.Instance.Vars[f]; ok {
						vars[f] = v
					}
				}
				vars["_processKey"], vars["_stage"] = c.ProcessKey, c.Stage
			}
			cases[w.CaseID] = vars
		}
		out = append(out, map[string]any{"work": w, "data": vars})
	}
	return out
}

func anyRole(by Actor, roles []string) bool {
	for _, r := range roles {
		if by.Has(r) {
			return true
		}
	}
	return false
}

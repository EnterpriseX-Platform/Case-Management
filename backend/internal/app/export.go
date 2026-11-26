package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// ExportApp packages a case type version as a solution pack (.cmpack) that
// the importer (or `cm push`) reads back: flow, case model, step screens and
// every form / screen they reference. Use it to move an app between
// environments or keep it in git.
func (s *Service) ExportApp(ctx context.Context, key, vid string) ([]byte, string, error) {
	pd, err := s.S.GetProcess(ctx, key)
	if err != nil {
		return nil, "", err
	}
	v, err := s.S.GetVersion(ctx, vid)
	if err != nil || v.ProcessKey != key {
		return nil, "", invalid("version does not belong to %s", key)
	}
	binds, err := s.S.GetBindings(ctx, vid)
	if err != nil {
		return nil, "", err
	}
	forms := map[string]bool{}
	screens := map[string]bool{}
	for _, b := range binds {
		if b.FormKey != "" {
			forms[b.FormKey] = true
		}
		if b.ScreenKey != "" {
			screens[b.ScreenKey] = true
		}
	}
	if p, _, err := s.Process(ctx, vid); err == nil {
		for _, n := range p.Nodes {
			if f := n.Ext["form"]; f != "" {
				forms[f] = true
			}
		}
	}
	if v.Model != nil {
		for _, a := range v.Model.Actions {
			if a.FormKey != "" {
				forms[a.FormKey] = true
			}
		}
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	put := func(name string, v any) error {
		w, err := z.Create(name)
		if err != nil {
			return err
		}
		if s, ok := v.(string); ok {
			_, err = w.Write([]byte(s))
			return err
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	proc := map[string]any{"key": key, "name": pd.Name, "category": pd.Category, "parentKey": pd.ParentKey,
		"fiscalYear": v.FiscalYear, "bpmn": "flows/" + key + ".bpmn"}
	if err := put("flows/"+key+".bpmn", v.BPMN); err != nil {
		return nil, "", err
	}
	if len(binds) > 0 {
		proc["bindings"] = "bindings/" + key + ".json"
		if err := put("bindings/"+key+".json", binds); err != nil {
			return nil, "", err
		}
	}
	if v.Model != nil {
		proc["model"] = "models/" + key + ".json"
		if err := put("models/"+key+".json", v.Model); err != nil {
			return nil, "", err
		}
	}
	for k := range forms {
		if f, err := s.S.GetForm(ctx, k); err == nil {
			if err := put("forms/"+k+".json", map[string]any{"key": f.Key, "name": f.Name, "schema": f.Schema}); err != nil {
				return nil, "", err
			}
		}
	}
	for k := range screens {
		if sc, err := s.S.GetScreen(ctx, k); err == nil {
			if err := put("screens/"+k+".json", map[string]any{"key": sc.Key, "name": sc.Name, "kind": sc.Kind, "roles": sc.Roles, "layout": sc.Layout}); err != nil {
				return nil, "", err
			}
		}
	}
	if err := put("manifest.json", map[string]any{"name": pd.Name, "version": fmt.Sprintf("v%d", v.No),
		"processes": []any{proc}, "forms": []string{"forms/*.json"}, "screens": []string{"screens/*.json"}}); err != nil {
		return nil, "", err
	}
	if err := z.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), fmt.Sprintf("%s-v%d.cmpack", key, v.No), nil
}

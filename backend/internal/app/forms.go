package app

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

// Low-code form schema (stored in Form.Schema):
//
//	{ "fields": [ { "key":"amount", "label":"วงเงิน", "type":"number", "required":true,
//	                "min":0, "max":1e12, "options":[{"value":"a","label":"A"}],
//	                "visibleIf":"type == 'x'", "readOnly":false, "width":6 } ],
//	  "sections": [ { "title":"...", "fields":["amount", ...] } ] }
//
// Field types: text, textarea, number, money, date, select, multiselect,
// checkbox, radio, file, table (rows of sub-fields), user, org, display.
var fieldTypes = map[string]bool{"text": true, "textarea": true, "number": true, "money": true, "date": true,
	"select": true, "multiselect": true, "checkbox": true, "radio": true, "file": true, "table": true,
	"user": true, "org": true, "display": true, "section": true}

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func fields(f *store.Form) []map[string]any {
	raw, _ := f.Schema["fields"].([]any)
	var out []map[string]any
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func CheckFormSchema(f *store.Form) error {
	seen := map[string]bool{}
	for i, fd := range fields(f) {
		k, _ := fd["key"].(string)
		t, _ := fd["type"].(string)
		if !keyRe.MatchString(k) {
			return invalid("field #%d: key %q must be an identifier", i+1, k)
		}
		if seen[k] {
			return invalid("field key %q is duplicated", k)
		}
		seen[k] = true
		if !fieldTypes[t] {
			return invalid("field %s: unknown type %q", k, t)
		}
	}
	return nil
}

// ValidateForm checks submitted data against required / type rules.
func ValidateForm(f *store.Form, data map[string]any) error {
	var errs []string
	// pages skipped by their condition are not filled in, so not checked
	skipped := map[string]bool{}
	pages, _ := f.Schema["pages"].([]any)
	for _, pg := range pages {
		if m, ok := pg.(map[string]any); ok {
			if cond, _ := m["skipIf"].(string); cond != "" {
				if yes, err := evalVisible(cond, data); err == nil && yes {
					skipped[fmt.Sprint(m["key"])] = true
				}
			}
		}
	}
	for _, fd := range fields(f) {
		k, _ := fd["key"].(string)
		t, _ := fd["type"].(string)
		label := fmt.Sprint(fd["label"])
		v, present := data[k]
		if pg, _ := fd["page"].(string); pg != "" && skipped[pg] {
			continue
		}
		if cond, _ := fd["visibleIf"].(string); cond != "" {
			if ok, err := evalVisible(cond, data); err == nil && !ok {
				continue
			}
		}
		if req, _ := fd["required"].(bool); req && t != "display" {
			if !present || v == nil || fmt.Sprint(v) == "" {
				errs = append(errs, label+": required")
				continue
			}
		}
		if !present || v == nil {
			continue
		}
		msg := func(def string) string {
			if m, _ := fd["message"].(string); m != "" {
				return label + ": " + m
			}
			return label + ": " + def
		}
		if sv, ok := v.(string); ok && (t == "text" || t == "textarea") {
			n := len([]rune(sv))
			if mn, ok := fd["minLen"].(float64); ok && sv != "" && n < int(mn) {
				errs = append(errs, msg(fmt.Sprintf("at least %d characters", int(mn))))
			}
			if mx, ok := fd["maxLen"].(float64); ok && n > int(mx) {
				errs = append(errs, msg(fmt.Sprintf("at most %d characters", int(mx))))
			}
			if pat, _ := fd["pattern"].(string); pat != "" && sv != "" {
				if re, err := regexp.Compile("^(?:" + pat + ")$"); err == nil && !re.MatchString(sv) {
					errs = append(errs, msg("wrong format"))
				}
			}
		}
		switch t {
		case "number", "money":
			n, ok := v.(float64)
			if !ok {
				errs = append(errs, label+": must be a number")
				continue
			}
			if mn, ok := fd["min"].(float64); ok && n < mn {
				errs = append(errs, fmt.Sprintf("%s: must be ≥ %v", label, mn))
			}
			if mx, ok := fd["max"].(float64); ok && n > mx {
				errs = append(errs, fmt.Sprintf("%s: must be ≤ %v", label, mx))
			}
		case "select", "radio":
			if opts, ok := fd["options"].([]any); ok && len(opts) > 0 {
				found := false
				for _, o := range opts {
					if om, ok := o.(map[string]any); ok && fmt.Sprint(om["value"]) == fmt.Sprint(v) {
						found = true
					}
				}
				if !found {
					errs = append(errs, label+": not an allowed option")
				}
			}
		}
	}
	if len(errs) > 0 {
		return invalid("%s", strings.Join(errs, "; "))
	}
	return nil
}

func evalVisible(cond string, data map[string]any) (bool, error) {
	return engine.EvalBool(cond, data)
}

func (s *Service) PutForm(ctx context.Context, by Actor, f store.Form) (*store.Form, error) {
	if !by.Has(RoleDesigner) {
		return nil, ErrForbidden
	}
	if f.Key == "" || f.Name == "" {
		return nil, invalid("key and name are required")
	}
	if f.Schema == nil {
		f.Schema = map[string]any{"fields": []any{}}
	}
	if err := CheckFormSchema(&f); err != nil {
		return nil, err
	}
	old, err := s.S.GetForm(ctx, f.Key)
	if err == nil {
		f.ID, f.Version = old.ID, old.Version+1
	} else {
		f.ID, f.Version = id.New(), 1
	}
	f.UpdatedAt = s.Now()
	s.audit(ctx, by, "form.save", "form", f.Key, map[string]any{"version": f.Version})
	return &f, s.S.PutForm(ctx, &f)
}

// Screen layout (stored in Screen.Layout):
//
//	{ "rows": [ { "cols": [ { "width":8, "widget":"form" },
//	                         { "width":4, "widget":"caseInfo" } ] },
//	            { "cols": [ { "width":12, "widget":"history" } ] } ] }
//
// Widgets: form, caseInfo, history, attachments, relatedCases, subCases,
// comments, iframe(url), markdown(text), kpi(metric), worklist, chart.
var widgets = map[string]bool{"form": true, "caseInfo": true, "history": true, "attachments": true,
	"relatedCases": true, "subCases": true, "comments": true, "iframe": true, "markdown": true,
	"kpi": true, "worklist": true, "chart": true, "flow": true, "myCases": true, "newCase": true}

func (s *Service) PutScreen(ctx context.Context, by Actor, sc store.Screen) (*store.Screen, error) {
	if !by.Has(RoleDesigner) {
		return nil, ErrForbidden
	}
	if sc.Key == "" || sc.Name == "" {
		return nil, invalid("key and name are required")
	}
	if sc.Kind == "" {
		sc.Kind = "task"
	}
	rows, _ := sc.Layout["rows"].([]any)
	for _, r := range rows {
		rm, _ := r.(map[string]any)
		cols, _ := rm["cols"].([]any)
		for _, c := range cols {
			cm, _ := c.(map[string]any)
			if w, _ := cm["widget"].(string); !widgets[w] {
				return nil, invalid("unknown widget %q", w)
			}
		}
	}
	old, err := s.S.GetScreen(ctx, sc.Key)
	if err == nil {
		sc.ID = old.ID
	} else {
		sc.ID = id.New()
	}
	sc.UpdatedAt = s.Now()
	s.audit(ctx, by, "screen.save", "screen", sc.Key, nil)
	return &sc, s.S.PutScreen(ctx, &sc)
}

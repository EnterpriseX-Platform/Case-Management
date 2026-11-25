// Package importer brings external artefacts into the platform. Every source
// is parsed into a Bundle (the platform's neutral model), previewed, then
// committed in one go.
//
// Sources: BPMN 2.0 XML (any modeller), solution packs (.zip with
// manifest.json), forms (JSON), cases / master data (CSV, XLSX).
package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
	"github.com/xuri/excelize/v2"
)

type ProcessItem struct {
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	Category   string          `json:"category,omitempty"`
	ParentKey  string          `json:"parentKey,omitempty"`
	FiscalYear int             `json:"fiscalYear,omitempty"`
	BPMN       string          `json:"-"`
	Bindings   []store.Binding `json:"bindings,omitempty"`
}

type Bundle struct {
	Processes []ProcessItem    `json:"processes"`
	Forms     []store.Form     `json:"forms"`
	Screens   []store.Screen   `json:"screens"`
	Cases     []app.StartInput `json:"cases"`
	Warnings  []string         `json:"warnings"`
}

// Options steer row imports (CSV/XLSX).
type Options struct {
	Kind       string // bpmn | pack | form | cases (auto-detected when empty)
	ProcessKey string // cases: target case type; bpmn: key to create/use
	Name       string
	FiscalYear int
}

// Parse auto-detects the source format and builds a Bundle.
func Parse(fileName string, data []byte, o Options) (*Bundle, error) {
	ext := strings.ToLower(path.Ext(fileName))
	kind := o.Kind
	if kind == "" {
		switch {
		case ext == ".bpmn" || (ext == ".xml" && bytes.Contains(data, []byte("definitions"))):
			kind = "bpmn"
		case ext == ".zip" || ext == ".cmpack":
			kind = "pack"
		case ext == ".json":
			kind = "form"
		case ext == ".csv" || ext == ".xlsx":
			kind = "cases"
		default:
			return nil, fmt.Errorf("cannot detect import type of %q", fileName)
		}
	}
	b := &Bundle{}
	switch kind {
	case "bpmn":
		p, warn, err := app.ValidateBPMN(string(data))
		if err != nil {
			return nil, err
		}
		key := firstNonEmpty(o.ProcessKey, strings.ToLower(p.ID))
		b.Processes = append(b.Processes, ProcessItem{Key: key, Name: firstNonEmpty(o.Name, p.Name, key), FiscalYear: o.FiscalYear, BPMN: string(data)})
		b.Warnings = warn
	case "pack":
		return parsePack(data)
	case "form":
		var f store.Form
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("form json: %w", err)
		}
		b.Forms = append(b.Forms, f)
	case "cases":
		if o.ProcessKey == "" {
			return nil, fmt.Errorf("case import needs the target case type")
		}
		rows, err := readRows(ext, data)
		if err != nil {
			return nil, err
		}
		if len(rows) < 2 {
			return nil, fmt.Errorf("file has no data rows")
		}
		head := rows[0]
		for i, r := range rows[1:] {
			in := app.StartInput{ProcessKey: o.ProcessKey, FiscalYear: o.FiscalYear, Vars: map[string]any{}}
			for j, h := range head {
				if j >= len(r) {
					break
				}
				h = strings.TrimSpace(h)
				v := strings.TrimSpace(r[j])
				switch h {
				case "title":
					in.Title = v
				case "fiscalYear":
					if n, err := strconv.Atoi(v); err == nil {
						in.FiscalYear = n
					} else if v != "" {
						b.Warnings = append(b.Warnings, fmt.Sprintf("row %d: fiscalYear %q is not a number", i+2, v))
					}
				case "":
				default:
					in.Vars[h] = typed(v)
				}
			}
			b.Cases = append(b.Cases, in)
		}
	default:
		return nil, fmt.Errorf("unknown import kind %q", kind)
	}
	return b, nil
}

func typed(v string) any {
	if n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", ""), 64); err == nil && v != "" {
		return n
	}
	switch strings.ToLower(v) {
	case "true":
		return true
	case "false":
		return false
	}
	if strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{") {
		var x any
		if json.Unmarshal([]byte(v), &x) == nil {
			return x
		}
	}
	return v
}

func readRows(ext string, data []byte) ([][]string, error) {
	if ext == ".xlsx" {
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.GetRows(f.GetSheetName(0))
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	r.FieldsPerRecord = -1
	return r.ReadAll()
}

// Pack manifest (manifest.json at the zip root):
//
//	{ "processes": [ { "key":"purchase-request", "name":"...", "bpmn":"flows/purchase-request.bpmn",
//	                   "fiscalYear":2027, "bindings":"bindings/purchase-request.json" } ],
//	  "forms": ["forms/*.json"], "screens": ["screens/*.json"] }
type manifest struct {
	Processes []struct {
		ProcessItem
		BPMNFile    string `json:"bpmn"`
		BindingFile string `json:"bindings"`
	} `json:"processes"`
	Forms   []string `json:"forms"`
	Screens []string `json:"screens"`
}

func parsePack(data []byte) (*Bundle, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pack: %w", err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || f.UncompressedSize64 > 20<<20 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 20<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		files[path.Clean(f.Name)] = b
	}
	raw, ok := files["manifest.json"]
	if !ok {
		return nil, fmt.Errorf("pack: manifest.json missing")
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("pack manifest: %w", err)
	}
	b := &Bundle{}
	glob := func(pat string) []string {
		var out []string
		for name := range files {
			if ok, _ := path.Match(pat, name); ok {
				out = append(out, name)
			}
		}
		return out
	}
	for _, p := range m.Processes {
		x, ok := files[path.Clean(p.BPMNFile)]
		if !ok {
			return nil, fmt.Errorf("pack: %s not found", p.BPMNFile)
		}
		if _, warn, err := app.ValidateBPMN(string(x)); err != nil {
			return nil, fmt.Errorf("pack %s: %w", p.BPMNFile, err)
		} else {
			b.Warnings = append(b.Warnings, warn...)
		}
		item := p.ProcessItem
		item.BPMN = string(x)
		if p.BindingFile != "" {
			if err := json.Unmarshal(files[path.Clean(p.BindingFile)], &item.Bindings); err != nil {
				return nil, fmt.Errorf("pack %s: %w", p.BindingFile, err)
			}
		}
		b.Processes = append(b.Processes, item)
	}
	for _, pat := range m.Forms {
		for _, name := range glob(pat) {
			var f store.Form
			if err := json.Unmarshal(files[name], &f); err != nil {
				return nil, fmt.Errorf("pack %s: %w", name, err)
			}
			b.Forms = append(b.Forms, f)
		}
	}
	for _, pat := range m.Screens {
		for _, name := range glob(pat) {
			var s store.Screen
			if err := json.Unmarshal(files[name], &s); err != nil {
				return nil, fmt.Errorf("pack %s: %w", name, err)
			}
			b.Screens = append(b.Screens, s)
		}
	}
	return b, nil
}

// Summary describes what a commit would create.
func (b *Bundle) Summary() map[string]any {
	keys := []string{}
	for _, p := range b.Processes {
		keys = append(keys, p.Key)
	}
	return map[string]any{"processes": keys, "forms": len(b.Forms), "screens": len(b.Screens), "cases": len(b.Cases)}
}

// Commit writes the bundle: forms and screens first (bindings refer to
// them), then case types with a new draft version each, then cases.
func Commit(ctx context.Context, s *app.Service, by app.Actor, b *Bundle, fileName string) (*store.ImportJob, error) {
	job := &store.ImportJob{ID: id.New(), Kind: "bundle", FileName: fileName, Status: "committed",
		Summary: b.Summary(), CreatedBy: by.ID, CreatedAt: s.Now()}
	err := s.S.Tx(ctx, func(ctx context.Context) error {
		for _, f := range b.Forms {
			if _, err := s.PutForm(ctx, by, f); err != nil {
				return fmt.Errorf("form %s: %w", f.Key, err)
			}
		}
		for _, sc := range b.Screens {
			if _, err := s.PutScreen(ctx, by, sc); err != nil {
				return fmt.Errorf("screen %s: %w", sc.Key, err)
			}
		}
		versions := []string{}
		for _, p := range b.Processes {
			if _, err := s.S.GetProcess(ctx, p.Key); err != nil {
				if _, err := s.CreateProcess(ctx, by, store.ProcessDef{Key: p.Key, Name: p.Name, Category: p.Category, ParentKey: p.ParentKey}); err != nil {
					return fmt.Errorf("case type %s: %w", p.Key, err)
				}
			}
			v, _, err := s.CreateDraft(ctx, by, p.Key, p.BPMN, "imported from "+fileName, p.FiscalYear)
			if err != nil {
				return fmt.Errorf("case type %s: %w", p.Key, err)
			}
			if len(p.Bindings) > 0 {
				if err := s.PutBindings(ctx, by, v.ID, p.Bindings); err != nil {
					return fmt.Errorf("bindings %s: %w", p.Key, err)
				}
			}
			versions = append(versions, v.ID)
		}
		job.Summary["versions"] = versions
		created := 0
		for i, c := range b.Cases {
			if _, err := s.StartCase(ctx, by, c); err != nil {
				return fmt.Errorf("row %d: %w", i+2, err)
			}
			created++
		}
		job.Summary["casesCreated"] = created
		return nil
	})
	if err != nil {
		job.Status, job.Issues = "failed", []string{err.Error()}
	}
	_ = s.S.PutImport(ctx, job)
	return job, err
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Package api exposes the REST API used by the low-code front-end and by
// external systems. All mutating routes use POST/PUT with JSON bodies;
// clients behind WAFs that block PUT may send POST + X-HTTP-Method-Override.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/importer"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

var httpRequests atomic.Int64

type Server struct {
	S *app.Service
	// Auth resolves the caller. The default trusts X-User / X-Roles headers,
	// which must be set by the identity gateway (IAM / OIDC proxy).
	Auth func(r *http.Request) (app.Actor, error)
	// PublicURL is used to build callback links for external screens.
	PublicURL string
}

func HeaderAuth(r *http.Request) (app.Actor, error) {
	u := r.Header.Get("X-User")
	if u == "" {
		return app.Actor{}, app.ErrForbidden
	}
	var roles []string
	for _, x := range strings.Split(r.Header.Get("X-Roles"), ",") {
		if x = strings.TrimSpace(x); x != "" {
			roles = append(roles, x)
		}
	}
	return app.Actor{ID: u, Name: r.Header.Get("X-User-Name"), Roles: roles}, nil
}

type ctxKey struct{}

func actor(r *http.Request) app.Actor { a, _ := r.Context().Value(ctxKey{}).(app.Actor); return a }

func (s *Server) Handler() http.Handler {
	if s.Auth == nil {
		s.Auth = HeaderAuth
	}
	m := http.NewServeMux()
	h := func(pattern string, fn func(w http.ResponseWriter, r *http.Request) (any, error)) {
		m.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			v, err := fn(w, r)
			if err != nil {
				writeErr(w, err)
				return
			}
			if v != nil {
				writeJSON(w, http.StatusOK, v)
			}
		})
	}

	h("GET /api/me", func(w http.ResponseWriter, r *http.Request) (any, error) { return actor(r), nil })

	// work management
	h("GET /api/me/profile", func(w http.ResponseWriter, r *http.Request) (any, error) {
		u, err := s.S.S.GetUser(r.Context(), actor(r).ID)
		if err != nil {
			return &store.UserProfile{ID: actor(r).ID, Name: actor(r).Name, Roles: actor(r).Roles, Active: true}, nil
		}
		return u, nil
	})
	h("PUT /api/me/profile", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var u store.UserProfile
		if err := decode(r, &u); err != nil {
			return nil, err
		}
		u.ID = actor(r).ID
		return s.S.SaveProfile(r.Context(), actor(r), &u)
	})
	h("GET /api/users", func(w http.ResponseWriter, r *http.Request) (any, error) {
		us, err := s.S.S.ListUsers(r.Context())
		if us == nil {
			us = []*store.UserProfile{}
		}
		return us, err
	})
	h("PUT /api/users/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var u store.UserProfile
		if err := decode(r, &u); err != nil {
			return nil, err
		}
		u.ID = r.PathValue("id")
		return s.S.SaveProfile(r.Context(), actor(r), &u)
	})
	h("GET /api/queues", func(w http.ResponseWriter, r *http.Request) (any, error) {
		qs, err := s.S.S.ListQueues(r.Context())
		out := []*store.Queue{}
		for _, q := range qs {
			if !strings.HasPrefix(q.Key, "_") {
				out = append(out, q)
			}
		}
		return out, err
	})
	h("PUT /api/queues/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var q store.Queue
		if err := decode(r, &q); err != nil {
			return nil, err
		}
		q.Key = r.PathValue("key")
		return q, s.S.SaveQueue(r.Context(), actor(r), &q)
	})
	h("DELETE /api/queues/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !actor(r).Has(app.RoleAdmin) && !actor(r).Has(app.RoleSupervisor) {
			return nil, app.ErrForbidden
		}
		return map[string]any{"ok": true}, s.S.S.DeleteQueue(r.Context(), r.PathValue("key"))
	})
	h("GET /api/settings", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.S.S.GetSettings(r.Context()) })
	h("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var st store.Settings
		if err := decode(r, &st); err != nil {
			return nil, err
		}
		return st, s.S.SaveSettings(r.Context(), actor(r), &st)
	})
	h("GET /api/notifications", func(w http.ResponseWriter, r *http.Request) (any, error) {
		ns, err := s.S.S.ListNotifications(r.Context(), actor(r).ID, 50)
		if ns == nil {
			ns = []*store.Notification{}
		}
		return ns, err
	})
	h("POST /api/notifications/read", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs []string `json:"ids"`
		}
		_ = decode(r, &in)
		return map[string]any{"ok": true}, s.S.S.MarkNotificationsRead(r.Context(), actor(r).ID, in.IDs)
	})
	h("POST /api/work/next", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.S.NextWork(r.Context(), actor(r)) })
	h("POST /api/work/bulk", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			IDs    []string `json:"ids"`
			Op     string   `json:"op"`
			Target string   `json:"target"`
			Amount int      `json:"amount"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		n, err := s.S.Bulk(r.Context(), actor(r), in.IDs, in.Op, in.Target, in.Amount)
		return map[string]any{"updated": n}, err
	})

	// case types & versions
	h("GET /api/processes", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.S.S.ListProcesses(r.Context()) })
	h("POST /api/processes", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var p store.ProcessDef
		if err := decode(r, &p); err != nil {
			return nil, err
		}
		return s.S.CreateProcess(r.Context(), actor(r), p)
	})
	h("GET /api/processes/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.S.GetProcess(r.Context(), r.PathValue("key"))
	})
	h("GET /api/processes/{key}/versions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.S.ListVersions(r.Context(), r.PathValue("key"))
	})
	h("POST /api/processes/{key}/versions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			BPMN       string `json:"bpmn"`
			Note       string `json:"note"`
			FiscalYear int    `json:"fiscalYear"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		v, warn, err := s.S.CreateDraft(r.Context(), actor(r), r.PathValue("key"), in.BPMN, in.Note, in.FiscalYear)
		return map[string]any{"version": v, "warnings": warn}, err
	})
	h("GET /api/versions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.S.GetVersion(r.Context(), r.PathValue("id"))
	})
	h("PUT /api/versions/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			BPMN       string `json:"bpmn"`
			Note       string `json:"note"`
			FiscalYear int    `json:"fiscalYear"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		v, warn, err := s.S.UpdateDraft(r.Context(), actor(r), r.PathValue("id"), in.BPMN, in.FiscalYear, in.Note)
		return map[string]any{"version": v, "warnings": warn}, err
	})
	h("POST /api/versions/{id}/{action}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		a := r.PathValue("action")
		if a == "clone" {
			var in struct {
				FiscalYear int `json:"fiscalYear"`
			}
			_ = decode(r, &in)
			return s.S.Clone(r.Context(), actor(r), r.PathValue("id"), in.FiscalYear)
		}
		return s.S.Transition(r.Context(), actor(r), r.PathValue("id"), a)
	})
	h("GET /api/versions/{id}/diff", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.Diff(r.Context(), r.PathValue("id"), r.URL.Query().Get("to"))
	})
	h("GET /api/versions/{id}/model", func(w http.ResponseWriter, r *http.Request) (any, error) {
		v, err := s.S.S.GetVersion(r.Context(), r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		return v.Model.Normalized(), nil
	})
	h("PUT /api/versions/{id}/model", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var m store.CaseModel
		if err := decode(r, &m); err != nil {
			return nil, err
		}
		v, err := s.S.PutModel(r.Context(), actor(r), r.PathValue("id"), &m)
		if err != nil {
			return nil, err
		}
		return v.Model.Normalized(), nil
	})
	h("POST /api/cases/{id}/actions/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in map[string]any
		_ = decode(r, &in)
		if in == nil {
			in = map[string]any{}
		}
		return s.S.RunAction(r.Context(), actor(r), r.PathValue("id"), r.PathValue("key"), in)
	})
	h("POST /api/cases/{id}/related", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Other string `json:"other"`
			Link  bool   `json:"link"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		other := in.Other
		if c, err := s.S.S.GetCase(r.Context(), other); err != nil { // allow case number
			cs, _ := s.S.S.ListCases(r.Context(), "", "", 0)
			for _, x := range cs {
				if strings.EqualFold(x.Number, other) {
					other = x.ID
				}
			}
		} else {
			other = c.ID
		}
		return map[string]any{"ok": true}, s.S.Relate(r.Context(), actor(r), r.PathValue("id"), other, in.Link)
	})
	h("GET /api/versions/{id}/lifecycle", func(w http.ResponseWriter, r *http.Request) (any, error) {
		v, err := s.S.S.GetVersion(r.Context(), r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		if v.Model == nil || v.Model.Lifecycle == nil {
			return map[string]any{"stages": []any{}}, nil
		}
		return v.Model.Lifecycle, nil
	})
	h("PUT /api/versions/{id}/lifecycle", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var l store.Lifecycle
		if err := decode(r, &l); err != nil {
			return nil, err
		}
		v, warn, err := s.S.SaveLifecycle(r.Context(), actor(r), r.PathValue("id"), &l)
		return map[string]any{"version": v, "warnings": warn}, err
	})
	h("POST /api/versions/{id}/decisions/{key}/test", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in map[string]any
		_ = decode(r, &in)
		return s.S.TestDecision(r.Context(), actor(r), r.PathValue("id"), r.PathValue("key"), in)
	})
	h("POST /api/connectors/test", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Connector store.Connector `json:"connector"`
			Vars      map[string]any  `json:"vars"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.S.TestConnector(r.Context(), actor(r), &in.Connector, in.Vars)
	})
	m.HandleFunc("GET /api/processes/{key}/export", func(w http.ResponseWriter, r *http.Request) {
		if !actor(r).Has(app.RoleDesigner) && !actor(r).Has(app.RoleAdmin) {
			writeErr(w, app.ErrForbidden)
			return
		}
		vid := r.URL.Query().Get("version")
		if vid == "" {
			if vs, err := s.S.S.ListVersions(r.Context(), r.PathValue("key")); err == nil && len(vs) > 0 {
				vid = vs[0].ID
			}
		}
		b, name, err := s.S.ExportApp(r.Context(), r.PathValue("key"), vid)
		if err != nil {
			writeErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Write(b)
	})
	h("GET /api/versions/{id}/steps", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.Steps(r.Context(), r.PathValue("id"))
	})
	h("PUT /api/versions/{id}/bindings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in []store.Binding
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if err := s.S.PutBindings(r.Context(), actor(r), r.PathValue("id"), in); err != nil {
			return nil, err
		}
		return s.S.Steps(r.Context(), r.PathValue("id"))
	})
	h("POST /api/validate/bpmn", func(w http.ResponseWriter, r *http.Request) (any, error) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if err != nil {
			return nil, err
		}
		p, warn, err := app.ValidateBPMN(string(b))
		if err != nil {
			return nil, err
		}
		return map[string]any{"process": p.ID, "nodes": len(p.Nodes), "warnings": warn}, nil
	})
	h("POST /api/migrations", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in app.MigrationRequest
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.S.Migrate(r.Context(), actor(r), in)
	})

	// low-code artefacts
	h("GET /api/forms", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.S.S.ListForms(r.Context()) })
	h("GET /api/forms/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.S.GetForm(r.Context(), r.PathValue("key"))
	})
	h("PUT /api/forms/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var f store.Form
		if err := decode(r, &f); err != nil {
			return nil, err
		}
		f.Key = r.PathValue("key")
		return s.S.PutForm(r.Context(), actor(r), f)
	})
	h("GET /api/screens", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.S.S.ListScreens(r.Context()) })
	h("GET /api/screens/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.S.GetScreen(r.Context(), r.PathValue("key"))
	})
	h("PUT /api/screens/{key}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var sc store.Screen
		if err := decode(r, &sc); err != nil {
			return nil, err
		}
		sc.Key = r.PathValue("key")
		return s.S.PutScreen(r.Context(), actor(r), sc)
	})

	// cases
	h("GET /api/cases", func(w http.ResponseWriter, r *http.Request) (any, error) {
		q := r.URL.Query()
		lim, _ := strconv.Atoi(q.Get("limit"))
		cs, err := s.S.S.ListCases(r.Context(), q.Get("process"), q.Get("version"), lim)
		if err != nil {
			return nil, err
		}
		fy, _ := strconv.Atoi(q.Get("fiscalYear"))
		st := q.Get("status")
		mine := q.Get("mine") == "1"
		tests := q.Get("test") == "1"
		out := []*store.Case{}
		for _, c := range cs {
			if c.Test != tests {
				continue
			}
			if (fy == 0 || c.FiscalYear == fy) && (st == "" || c.Status == st) && (!mine || c.CreatedBy == actor(r).ID) {
				out = append(out, c)
			}
		}
		return out, nil
	})
	h("POST /api/cases", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in app.StartInput
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.S.StartCase(r.Context(), actor(r), in)
	})
	h("GET /api/cases/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		ctx := r.Context()
		c, err := s.S.S.GetCase(ctx, r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		work, _ := s.S.S.ListWork(ctx, store.WorkFilter{CaseID: c.ID})
		audit, _ := s.S.S.ListAudit(ctx, c.ID, 200)
		v, _ := s.S.S.GetVersion(ctx, c.VersionID)
		all, _ := s.S.S.ListCases(ctx, "", "", 0)
		var kids []*store.Case
		for _, k := range all {
			if k.ParentID == c.ID {
				kids = append(kids, k)
			}
		}
		ev, _ := s.S.S.ListEvents(ctx, c.ID, 5000)
		for _, e := range ev {
			c.Instance.History = append(c.Instance.History, engine.HistoryEntry{At: e.At, Node: e.Node, Event: e.Type, Token: e.Token, By: e.By})
		}
		inc, _ := s.S.S.ListIncidents(ctx, "", c.ID)
		stages, actions, missing := s.S.CaseOverview(ctx, actor(r), c)
		var related []*store.Case
		for _, rid := range c.RelatedIDs {
			if rc, err := s.S.S.GetCase(ctx, rid); err == nil {
				related = append(related, rc)
			}
		}
		var parent *store.Case
		if c.ParentID != "" {
			parent, _ = s.S.S.GetCase(ctx, c.ParentID)
		}
		return map[string]any{"case": c, "work": work, "audit": audit, "version": v, "children": kids, "incidents": inc,
			"stages": stages, "actions": actions, "missing": missing, "related": related, "parent": parent, "events": ev}, nil
	})
	h("POST /api/cases/{id}/cancel", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Reason string }
		_ = decode(r, &in)
		return s.S.Cancel(r.Context(), actor(r), r.PathValue("id"), in.Reason)
	})
	h("POST /api/cases/{id}/move", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Token, To, Reason string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.S.Move(r.Context(), actor(r), r.PathValue("id"), in.Token, in.To, in.Reason)
	})

	// work
	h("GET /api/work", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.Inbox(r.Context(), actor(r), r.URL.Query().Get("all") == "1")
	})
	h("GET /api/work/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		ctx := r.Context()
		wk, err := s.S.S.GetWork(ctx, r.PathValue("id"))
		if err != nil {
			return nil, err
		}
		out := map[string]any{"work": wk}
		if wk.FormKey != "" {
			out["form"], _ = s.S.S.GetForm(ctx, wk.FormKey)
		}
		if wk.ScreenKey != "" {
			out["screen"], _ = s.S.S.GetScreen(ctx, wk.ScreenKey)
		}
		if c, err := s.S.S.GetCase(ctx, wk.CaseID); err == nil {
			out["vars"] = c.Instance.Vars
		}
		if wk.UI == engine.UIExternal {
			out["callback"] = strings.TrimRight(s.PublicURL, "/") + "/api/callback/" + wk.ID + "?token=" + s.S.CallbackToken(wk.ID)
		}
		return out, nil
	})
	h("POST /api/work/{id}/claim", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.Claim(r.Context(), actor(r), r.PathValue("id"))
	})
	h("POST /api/work/{id}/reassign", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct{ Assignee, Role string }
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.S.Reassign(r.Context(), actor(r), r.PathValue("id"), in.Assignee, in.Role)
	})
	h("POST /api/work/{id}/complete", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in map[string]any
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return s.S.Complete(r.Context(), actor(r), r.PathValue("id"), in)
	})

	// events
	h("POST /api/messages", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Name, CaseID, CorrelationKey string
			Vars                         map[string]any
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		n, err := s.S.Message(r.Context(), actor(r), in.CaseID, in.Name, in.CorrelationKey, in.Vars)
		return map[string]any{"matched": n}, err
	})
	h("POST /api/signals", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Name string
			Vars map[string]any
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, s.S.Signal(r.Context(), actor(r), in.Name, in.Vars)
	})

	// import
	h("GET /api/imports", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.S.S.ListImports(r.Context()) })
	h("POST /api/imports", func(w http.ResponseWriter, r *http.Request) (any, error) {
		by := actor(r)
		if !by.Has(app.RoleAdmin) && !by.Has(app.RoleDesigner) {
			return nil, app.ErrForbidden
		}
		if err := r.ParseMultipartForm(50 << 20); err != nil {
			return nil, err
		}
		f, hdr, err := r.FormFile("file")
		if err != nil {
			return nil, err
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 50<<20))
		if err != nil {
			return nil, err
		}
		fy, _ := strconv.Atoi(r.FormValue("fiscalYear"))
		b, err := importer.Parse(hdr.Filename, data, importer.Options{Kind: r.FormValue("kind"),
			ProcessKey: r.FormValue("processKey"), Name: r.FormValue("name"), FiscalYear: fy})
		if err != nil {
			return nil, err
		}
		if r.FormValue("commit") != "1" {
			return map[string]any{"preview": true, "summary": b.Summary(), "warnings": b.Warnings, "bundle": b}, nil
		}
		return importer.Commit(r.Context(), s.S, by, b, hdr.Filename)
	})

	// supervisor dashboard
	h("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) (any, error) { return s.dashboard(r.Context(), r) })
	h("GET /api/audit", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !actor(r).Has(app.RoleAdmin) {
			return nil, app.ErrForbidden
		}
		return s.S.S.ListAudit(r.Context(), r.URL.Query().Get("ref"), 500)
	})

	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	m.HandleFunc("GET /metrics", metrics)
	m.HandleFunc("GET /api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(openapiSpec)
	})

	// operations: incidents, jobs, event log
	h("GET /api/incidents", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !actor(r).Has(app.RoleSupervisor) {
			return nil, app.ErrForbidden
		}
		st := store.IncidentStatus(r.URL.Query().Get("status"))
		return s.S.S.ListIncidents(r.Context(), st, r.URL.Query().Get("case"))
	})
	h("POST /api/incidents/{id}/{action}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			To   string         `json:"to"`
			Vars map[string]any `json:"vars"`
			Note string         `json:"note"`
		}
		_ = decode(r, &in)
		return s.S.ResolveIncident(r.Context(), actor(r), r.PathValue("id"), r.PathValue("action"), in.To, in.Vars, in.Note)
	})
	h("POST /api/incidents-retry-all", func(w http.ResponseWriter, r *http.Request) (any, error) {
		n, err := s.S.RetryAll(r.Context(), actor(r))
		return map[string]any{"retried": n}, err
	})
	h("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if !actor(r).Has(app.RoleSupervisor) {
			return nil, app.ErrForbidden
		}
		return s.S.S.ListJobs(r.Context(), r.URL.Query().Get("case"))
	})
	h("GET /api/cases/{id}/events", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return s.S.S.ListEvents(r.Context(), r.PathValue("id"), 5000)
	})

	// external screens: no user session, authorised by signed token
	callback := func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := decode(r, &in); err != nil {
			writeErr(w, err)
			return
		}
		c, err := s.S.CompleteByCallback(r.Context(), r.PathValue("id"), r.URL.Query().Get("token"), in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"caseId": c.ID, "status": c.Status})
	}

	return s.middleware(m, callback)
}

func (s *Server) middleware(m *http.ServeMux, callback http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if o := r.Header.Get("X-HTTP-Method-Override"); o != "" && r.Method == http.MethodPost {
			r.Method = strings.ToUpper(o)
		}
		if strings.HasPrefix(r.URL.Path, "/api/callback/") && r.Method == http.MethodPost {
			r.SetPathValue("id", strings.TrimPrefix(r.URL.Path, "/api/callback/"))
			callback(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/openapi.yaml" {
			a, err := s.Auth(r)
			if err != nil {
				writeErr(w, err)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, a))
			s.S.TouchUser(r.Context(), a)
		}
		httpRequests.Add(1)
		m.ServeHTTP(w, r)
		slog.Debug("http", "method", r.Method, "path", r.URL.Path, "ms", time.Since(start).Milliseconds())
	})
}

type stepCount struct {
	Version string `json:"version"`
	No      int    `json:"no"`
	Node    string `json:"node"`
	Name    string `json:"name"`
	Open    int    `json:"open"`
	Overdue int    `json:"overdue"`
}

func (s *Server) dashboard(ctx context.Context, r *http.Request) (any, error) {
	if !actor(r).Has(app.RoleSupervisor) {
		return nil, app.ErrForbidden
	}
	key := r.URL.Query().Get("process")
	fy, _ := strconv.Atoi(r.URL.Query().Get("fiscalYear"))
	cases, err := s.S.S.ListCases(ctx, key, "", 0)
	if err != nil {
		return nil, err
	}
	byStatus := map[string]int{}
	caseVersion := map[string]string{}
	for _, c := range cases {
		if fy != 0 && c.FiscalYear != fy {
			continue
		}
		if c.Test {
			continue
		}
		byStatus[c.Status]++
		caseVersion[c.ID] = c.VersionID
	}
	work, err := s.S.S.ListWork(ctx, store.WorkFilter{Status: []store.WorkStatus{store.WorkOpen, store.WorkClaimed}})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	steps := map[string]*stepCount{}
	byRole := map[string]int{}
	overdue := 0
	vno := map[string]int{}
	for _, w := range work {
		vid, ok := caseVersion[w.CaseID]
		if !ok {
			continue
		}
		if _, ok := vno[vid]; !ok {
			if v, err := s.S.S.GetVersion(ctx, vid); err == nil {
				vno[vid] = v.No
			}
		}
		k := vid + "/" + w.Node
		sc := steps[k]
		if sc == nil {
			sc = &stepCount{Version: vid, No: vno[vid], Node: w.Node, Name: w.Name}
			steps[k] = sc
		}
		sc.Open++
		if w.DueAt != nil && w.DueAt.Before(now) {
			sc.Overdue++
			overdue++
		}
		byRole[firstNonEmpty(w.Role, "-")]++
	}
	list := make([]*stepCount, 0, len(steps))
	for _, v := range steps {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Open > list[j].Open })
	return map[string]any{"casesByStatus": byStatus, "steps": list, "workByRole": byRole, "overdue": overdue, "openWork": len(work)}, nil
}

func decode(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 20<<20))
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return errors.Join(app.ErrInvalid, err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	switch {
	case errors.Is(err, store.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, app.ErrForbidden):
		code = http.StatusForbidden
	case errors.Is(err, app.ErrInvalid):
		code = http.StatusBadRequest
	case errors.Is(err, app.ErrConflict), errors.Is(err, engine.ErrNotWaiting):
		code = http.StatusConflict
	default:
		if strings.HasPrefix(err.Error(), "engine:") || strings.HasPrefix(err.Error(), "bpmn") {
			code = http.StatusUnprocessableEntity
		}
	}
	if code == 500 {
		slog.Error("request failed", "err", err)
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

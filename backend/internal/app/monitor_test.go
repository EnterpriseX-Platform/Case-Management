package app_test

import (
	"testing"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

func TestMonitorAndInspector(t *testing.T) {
	w := wmSetup(t, store.Binding{SLA: "P1D", Goal: "PT12H", Route: &store.Routing{Mode: "user", User: "u1"}})
	a, b := w.start(t), w.start(t)
	*w.now = w.now.Add(3 * time.Hour)
	u1 := app.Actor{ID: "u1", Roles: []string{"clerk"}}
	if _, err := w.s.Complete(w.ctx, u1, a.ID, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	*w.now = w.now.Add(30 * time.Hour) // b is now overdue

	if _, err := w.s.MonitorOverview(w.ctx, u1, "", 30); err == nil {
		t.Fatal("overview open to non-supervisors")
	}
	o, err := w.s.MonitorOverview(w.ctx, boss, "intake", 30)
	if err != nil {
		t.Fatal(err)
	}
	if o.Started != 2 || o.Completed != 1 || o.Running != 1 || o.SLA["overdue"] != 1 || o.CycleP50 < 2.9 || o.CycleP50 > 3.1 {
		t.Fatalf("overview: %+v", o)
	}
	if len(o.Workload) == 0 || o.Workload[0].User != "u1" || o.Workload[0].Open != 1 || o.Workload[0].Done != 1 {
		t.Fatalf("workload: %+v", o.Workload)
	}

	c, _ := w.s.S.GetCase(w.ctx, b.CaseID)
	ps, err := w.s.ProcessStatistics(w.ctx, boss, c.VersionID, 30)
	if err != nil {
		t.Fatal(err)
	}
	var triage *app.NodeStat
	for i := range ps.Nodes {
		if ps.Nodes[i].Node == "triage" {
			triage = &ps.Nodes[i]
		}
	}
	if triage == nil || triage.Active != 1 || triage.Overdue != 1 || triage.Passed != 1 || triage.AvgHours < 2.9 || triage.AvgHours > 3.1 {
		t.Fatalf("triage stats: %+v", triage)
	}

	rows, _ := w.s.Explore(w.ctx, boss, app.ExplorerFilter{Process: "intake", Node: "triage", SLA: "overdue"})
	if len(rows) != 1 || rows[0].ID != b.CaseID || rows[0].Assignee != "u1" {
		t.Fatalf("explorer: %+v", rows)
	}

	// inspector: fix data with a reason, then skip the step
	if _, err := w.s.SetVars(w.ctx, boss, b.CaseID, map[string]any{"priority": "high"}, ""); err == nil {
		t.Fatal("change without reason accepted")
	}
	if _, err := w.s.SetVars(w.ctx, boss, b.CaseID, map[string]any{"priority": "high"}, "customer called"); err != nil {
		t.Fatal(err)
	}
	in, err := w.s.Inspect(w.ctx, boss, b.CaseID)
	if err != nil || in.Case.Instance.Vars["priority"] != "high" || in.Steps["triage"] != "Triage" {
		t.Fatalf("inspect: %v %+v", err, in)
	}
	tok := in.Case.Instance.Tokens[0].ID
	if _, err := w.s.Skip(w.ctx, boss, b.CaseID, tok, "duplicate request"); err != nil {
		t.Fatal(err)
	}
	if c, _ := w.s.S.GetCase(w.ctx, b.CaseID); c.Status != "completed" {
		t.Fatalf("skip did not finish the case: %s", c.Status)
	}
}

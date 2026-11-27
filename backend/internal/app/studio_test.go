package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/importer"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

const blank = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/><endEvent id="e"/><sequenceFlow id="1" sourceRef="s" targetRef="e"/></process></definitions>`

func TestDecisionTable(t *testing.T) {
	dt := &store.DecisionTable{Key: "risk", Inputs: []store.DTColumn{{Key: "amount", Label: "Amount"}, {Key: "category", Label: "Category"}},
		Outputs: []store.DTColumn{{Key: "risk", Label: "Risk"}, {Key: "approver", Label: "Approver"}},
		Rows: []store.DTRow{
			{When: []string{">= 1000000", ""}, Then: []string{`"high"`, `"director"`}},
			{When: []string{"100000..999999", `in ["it","vehicle"]`}, Then: []string{`"medium"`, `"manager"`}},
			{When: []string{"", ""}, Then: []string{`"low"`, `"supervisor"`}},
		}}
	for _, c := range []struct {
		vars map[string]any
		risk string
	}{
		{map[string]any{"amount": 2e6, "category": "it"}, "high"},
		{map[string]any{"amount": 2e5, "category": "it"}, "medium"},
		{map[string]any{"amount": 2e5, "category": "office"}, "low"},
	} {
		out, _, err := app.EvalDecision(dt, c.vars)
		if err != nil || out["risk"] != c.risk {
			t.Fatalf("%v -> %v %v", c.vars, out, err)
		}
	}
}

// A business user builds a whole app without touching BPMN: lifecycle,
// decision table, REST connector; the generated flow runs end to end, and the
// app round-trips through export / import.
func TestLifecycleApp(t *testing.T) {
	var erpCalls int
	erp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		erpCalls++
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("Authorization") != "Bearer s3cret" || body["amount"] != 250000.0 {
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"order": map[string]any{"id": "ERP-77"}}})
	}))
	defer erp.Close()
	t.Setenv("CM_SECRET_ERP_TOKEN", "Bearer s3cret")

	ctx := context.Background()
	s := app.New(newStore(t), nil, []byte("x"))
	if _, err := s.CreateProcess(ctx, designer, store.ProcessDef{Key: "grant", Name: "Grant request"}); err != nil {
		t.Fatal(err)
	}
	v, _, _ := s.CreateDraft(ctx, designer, "grant", blank, "", 2027)
	m := &store.CaseModel{
		Data: []store.DataField{{Key: "amount", Label: "Amount", Type: "money", Summary: true}, {Key: "risk", Label: "Risk", Type: "text"}},
		Decisions: []store.DecisionTable{{Key: "risk", Name: "Risk", Inputs: []store.DTColumn{{Key: "amount", Label: "Amount"}},
			Outputs: []store.DTColumn{{Key: "risk", Label: "Risk"}},
			Rows:    []store.DTRow{{When: []string{">= 100000"}, Then: []string{`"high"`}}, {When: []string{""}, Then: []string{`"low"`}}}}},
		Connectors: []store.Connector{{Key: "erp", Name: "ERP", Method: "POST", URL: erp.URL + "/orders",
			Headers: map[string]string{"Authorization": "${secret:erp_token}"}, Body: map[string]string{"amount": "amount", "ref": "caseNumber"},
			Response: map[string]string{"erpId": "data.order.id"}}},
	}
	if _, err := s.PutModel(ctx, designer, v.ID, m); err != nil {
		t.Fatal(err)
	}
	// bad: credentials inline are refused
	bad := *m
	bad.Connectors = []store.Connector{{Key: "x", Name: "x", URL: "https://x", Headers: map[string]string{"Authorization": "Bearer abc"}}}
	if _, err := s.PutModel(ctx, designer, v.ID, &bad); err == nil {
		t.Fatal("inline credential accepted")
	}

	l := &store.Lifecycle{Stages: []store.LCStage{
		{Key: "intake", Name: "Intake", Kind: "primary", Steps: []store.LCStep{
			{Key: "submit", Name: "Submit request", Kind: "task", Role: "clerk"},
			{Key: "assess", Name: "Assess risk", Kind: "decision", Decision: "risk"}}},
		{Key: "review", Name: "Review", Kind: "primary", SLA: "P5D", Steps: []store.LCStep{
			{Key: "legal", Name: "Legal check", Kind: "task", Role: "clerk", When: `risk == "high"`},
			{Key: "approve", Name: "Approve", Kind: "approval", Role: "approver", OnReject: "intake"}}},
		{Key: "fulfil", Name: "Fulfil", Kind: "primary", Steps: []store.LCStep{
			{Key: "order", Name: "Create ERP order", Kind: "automation", Connector: "erp"},
			{Key: "paid", Name: "Wait for payment", Kind: "wait"}}},
		{Key: "cancelled", Name: "Cancelled", Kind: "alternate", Steps: []store.LCStep{{Key: "notify", Name: "Notify applicant", Kind: "task", Role: "clerk"}}},
		{Key: "done", Name: "Done", Kind: "resolution"},
	}}
	v2, warn, err := s.SaveLifecycle(ctx, designer, v.ID, l)
	if err != nil {
		t.Fatal(err, warn)
	}
	ps, err := bpmn.ParseString(v2.BPMN)
	if err != nil || len(ps[0].Unsupported()) != 0 || !strings.Contains(v2.BPMN, "bpmndi:BPMNShape") {
		t.Fatalf("generated BPMN: %v", err)
	}
	if len(v2.Model.Stages) != 5 || v2.Model.Data[0].Key != "amount" || len(v2.Model.Decisions) != 1 {
		t.Fatalf("model not merged: %+v", v2.Model.Stages)
	}
	approver := app.Actor{ID: "a1", Roles: []string{"approver"}}
	publish(t, s, ctx, v.ID)

	// --- run it
	c, err := s.StartCase(ctx, clerk, app.StartInput{ProcessKey: "grant", FiscalYear: 2027})
	if err != nil {
		t.Fatal(err)
	}
	do(t, s, ctx, clerk, c.ID, "submit", map[string]any{"amount": 250000.0})
	if got := stageOf(t, s, ctx, c.ID); got != "review" {
		t.Fatalf("stage %s", got)
	}
	c, _ = s.S.GetCase(ctx, c.ID)
	if c.Instance.Vars["risk"] != "high" {
		t.Fatalf("decision not applied: %v", c.Instance.Vars)
	}
	do(t, s, ctx, clerk, c.ID, "legal", nil) // only because risk is high
	do(t, s, ctx, approver, c.ID, "approve", map[string]any{"approve_approved": false})
	if got := stageOf(t, s, ctx, c.ID); got != "intake" {
		t.Fatalf("reject should return to intake, got %s", got)
	}
	do(t, s, ctx, clerk, c.ID, "submit", map[string]any{"amount": 250000.0})
	do(t, s, ctx, clerk, c.ID, "legal", nil)
	do(t, s, ctx, approver, c.ID, "approve", map[string]any{"approve_approved": true})
	c, _ = s.S.GetCase(ctx, c.ID)
	if c.Stage != "fulfil" || c.Instance.Vars["erpId"] != "ERP-77" || erpCalls != 1 {
		t.Fatalf("connector: stage=%s vars=%v calls=%d", c.Stage, c.Instance.Vars, erpCalls)
	}
	if n, err := s.Message(ctx, clerk, c.ID, "Wait for payment", "", nil); err != nil || n != 1 {
		t.Fatalf("wait step: %d %v", n, err)
	}
	if c, _ = s.S.GetCase(ctx, c.ID); c.Status != "completed" || c.Stage != "done" {
		t.Fatalf("status=%s stage=%s", c.Status, c.Stage)
	}

	// --- test runs are hidden from real work lists
	tc, err := s.StartCase(ctx, designer, app.StartInput{ProcessKey: "grant", FiscalYear: 2027, Test: true})
	if err != nil || !strings.HasPrefix(tc.Number, "TEST-") {
		t.Fatalf("test run: %v", err)
	}
	if len(inbox(t, s, ctx, clerk, tc.ID)) != 0 {
		t.Fatal("test work visible to real users")
	}

	// --- export and import into a fresh environment
	zipb, name, err := s.ExportApp(ctx, "grant", v.ID)
	if err != nil || !strings.HasSuffix(name, ".cmpack") {
		t.Fatal(err)
	}
	b, err := importer.Parse(name, zipb, importer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s2 := app.New(store.NewMemory(), nil, []byte("x"))
	if _, err := importer.Commit(ctx, s2, designer, b, name); err != nil {
		t.Fatal(err)
	}
	vs, _ := s2.S.ListVersions(ctx, "grant")
	if len(vs) != 1 || vs[0].Model == nil || len(vs[0].Model.Connectors) != 1 || vs[0].Model.Lifecycle == nil {
		t.Fatalf("round trip lost the model: %+v", vs)
	}
}

// An approval may reject to an alternate stage that has no steps of its own.
func TestLifecycleRejectToEmptyStage(t *testing.T) {
	l := &app.Lifecycle{Stages: []store.LCStage{
		{Key: "work", Name: "Work", Kind: "primary", Steps: []store.LCStep{
			{Key: "do", Name: "Do", Kind: "task", Role: "r"},
			{Key: "ok", Name: "OK?", Kind: "approval", Role: "a", OnReject: "rejected"},
		}},
		{Key: "rejected", Name: "Rejected", Kind: "alternate"},
		{Key: "done", Name: "Done", Kind: "resolution"},
	}}
	xml, m, _, err := app.GenerateBPMN("p", "P", l)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bpmn.ParseString(xml); err != nil {
		t.Fatal(err)
	}
	if st := m.Stage("rejected"); st == nil || !strings.Contains(strings.Join(st.Steps, ","), "rejected_end") {
		t.Fatal("ending in the rejected stage should leave the case there")
	}
	if !strings.Contains(xml, `sourceRef="ok_ok" targetRef="rejected_end"`) {
		t.Fatalf("reject flow does not reach the stage end")
	}
}

// A conditional approval that is skipped must not count as a rejection: the
// approve/reject check sits on the branch where the step runs.
func TestLifecycleSkippedApprovalIsNotRejection(t *testing.T) {
	l := &app.Lifecycle{Stages: []store.LCStage{
		{Key: "work", Name: "Work", Kind: "primary", Steps: []store.LCStep{
			{Key: "big", Name: "Big?", Kind: "approval", Role: "a", DecisionField: "ok", When: "amount > 10", OnReject: "end"},
		}},
		{Key: "pay", Name: "Pay", Kind: "primary", Steps: []store.LCStep{{Key: "pay", Name: "Pay", Kind: "task", Role: "f"}}},
	}}
	xml, _, _, err := app.GenerateBPMN("p", "P", l)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bpmn.ParseString(xml); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`sourceRef="big_when" targetRef="big_join"`, // skipped: straight on
		`sourceRef="big" targetRef="big_ok"`,        // done: check the decision
		`sourceRef="big_ok" targetRef="big_join"`,   // approved: straight on
		`sourceRef="big_join" targetRef="pay"`,
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("missing flow %s", want)
		}
	}
	if strings.Contains(xml, `sourceRef="big_join" targetRef="big_ok"`) {
		t.Fatal("skipped approval still goes through the decision")
	}
}

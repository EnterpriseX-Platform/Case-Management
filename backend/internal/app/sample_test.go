package app_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/importer"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

var (
	designer = app.Actor{ID: "designer1", Roles: []string{app.RoleDesigner}}
	approver = app.Actor{ID: "approver1", Roles: []string{app.RoleApprover}}
	admin    = app.Actor{ID: "admin1", Roles: []string{app.RoleAdmin}}
	reqr     = app.Actor{ID: "req1", Roles: []string{"requester"}}
	reviewer = app.Actor{ID: "rev1", Roles: []string{"reviewer"}}
	mgr      = app.Actor{ID: "mgr1", Roles: []string{"approver"}}
	buyer    = app.Actor{ID: "buy1", Roles: []string{"procurement"}}
	finance  = app.Actor{ID: "fin1", Roles: []string{"finance"}}
	boss     = app.Actor{ID: "sup1", Roles: []string{app.RoleSupervisor}}
)

func setup(t *testing.T) (*app.Service, context.Context, map[string]string) {
	t.Helper()
	ctx := context.Background()
	s := app.New(store.NewMemory(), nil, []byte("test"))
	data, err := os.ReadFile("../../../packs/purchase-request.cmpack")
	if err != nil {
		t.Fatal(err)
	}
	b, err := importer.Parse("purchase-request.cmpack", data, importer.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Processes) != 2 || len(b.Forms) < 7 || len(b.Screens) != 3 {
		t.Fatalf("bundle %+v", b.Summary())
	}
	if _, err := importer.Commit(ctx, s, designer, b, "purchase-request.cmpack"); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, key := range []string{"purchase-request", "purchase-request-urgent"} {
		vs, _ := s.S.ListVersions(ctx, key)
		v := vs[0]
		if _, _, err := s.UpdateDraft(ctx, designer, v.ID, "", 2027, ""); err != nil {
			t.Fatal(err)
		}
		publish(t, s, ctx, v.ID)
		ids[key] = v.ID
	}
	return s, ctx, ids
}

func publish(t *testing.T, s *app.Service, ctx context.Context, vid string) {
	t.Helper()
	if _, err := s.Transition(ctx, designer, vid, "submit"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Transition(ctx, designer, vid, "approve"); err == nil {
		t.Fatal("maker-checker: designer must not approve")
	}
	for _, a := range []string{"approve", "publish"} {
		if _, err := s.Transition(ctx, approver, vid, a); err != nil {
			t.Fatal(a, err)
		}
	}
}

func inbox(t *testing.T, s *app.Service, ctx context.Context, a app.Actor, caseID string) []*store.Work {
	t.Helper()
	ws, err := s.Inbox(ctx, a, false)
	if err != nil {
		t.Fatal(err)
	}
	var out []*store.Work
	for _, w := range ws {
		if w.CaseID == caseID {
			out = append(out, w)
		}
	}
	return out
}

func do(t *testing.T, s *app.Service, ctx context.Context, a app.Actor, caseID, node string, data map[string]any) *store.Case {
	t.Helper()
	for _, w := range inbox(t, s, ctx, a, caseID) {
		if w.Node == node {
			c, err := s.Complete(ctx, a, w.ID, data)
			if err != nil {
				t.Fatalf("%s: %v", node, err)
			}
			return c
		}
	}
	t.Fatalf("%s: no work item for %s", node, a.ID)
	return nil
}

func TestPurchaseEndToEnd(t *testing.T) {
	s, ctx, _ := setup(t)
	c, err := s.StartCase(ctx, reqr, app.StartInput{ProcessKey: "purchase-request", FiscalYear: 2027, Title: "จัดซื้อคอมพิวเตอร์"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.Number, "PURCHASE-REQUEST-2027-") {
		t.Fatal(c.Number)
	}
	if len(inbox(t, s, ctx, reviewer, c.ID)) != 0 {
		t.Fatal("reviewer must not see requester work")
	}
	w := inbox(t, s, ctx, reqr, c.ID)[0]
	if _, err := s.Complete(ctx, reqr, w.ID, map[string]any{}); err == nil {
		t.Fatal("expected form validation error")
	}
	do(t, s, ctx, reqr, c.ID, "request", map[string]any{"amount": 500000.0, "reviewers": []any{"IT", "Legal", "Finance"}})
	revs := inbox(t, s, ctx, reviewer, c.ID)
	if len(revs) != 3 || revs[0].Input["reviewer"] == nil || revs[0].DueAt == nil {
		t.Fatalf("expected one review per department, got %d", len(revs))
	}
	for _, r := range revs {
		if _, err := s.Complete(ctx, reviewer, r.ID, map[string]any{"opinion": "agree"}); err != nil {
			t.Fatal(err)
		}
	}
	ap := inbox(t, s, ctx, mgr, c.ID)
	if len(ap) != 1 || ap[0].ScreenKey != "task-approval" {
		t.Fatalf("approval work %+v", ap)
	}
	do(t, s, ctx, mgr, c.ID, "approve", map[string]any{"approvedAmount": 500000.0, "remark": "เกินอำนาจ", "withinLimit": false})
	do(t, s, ctx, reqr, c.ID, "revise", map[string]any{"approvedAmount": 300000.0, "remark": "ปรับลดแล้ว", "withinLimit": true})
	do(t, s, ctx, buyer, c.ID, "purchase", map[string]any{"poNumber": "PO-1", "vendor": "A", "poDate": "2026-12-01"})
	do(t, s, ctx, reqr, c.ID, "receive", map[string]any{"result": "accepted", "receiveDate": "2026-12-10"})
	c = do(t, s, ctx, finance, c.ID, "accounting", map[string]any{"voucher": "V-1", "postDate": "2026-12-11"})
	if c.Status != string(engine.Completed) {
		t.Fatalf("status %s", c.Status)
	}
}

func TestUrgentRejectedOverLimit(t *testing.T) {
	s, ctx, _ := setup(t)
	c, err := s.StartCase(ctx, reqr, app.StartInput{ProcessKey: "purchase-request-urgent", FiscalYear: 2027})
	if err != nil {
		t.Fatal(err)
	}
	c = do(t, s, ctx, reqr, c.ID, "justification", map[string]any{"amount": 1e9, "necessity": "เร่งด่วน", "withinLimit": false})
	if c.Status != string(engine.Completed) {
		t.Fatalf("status %s", c.Status)
	}
}

// Next year: clone, add a step, publish, migrate running cases.
func TestVersionPerFiscalYearAndMigration(t *testing.T) {
	s, ctx, ids := setup(t)
	v1 := ids["purchase-request"]
	c, _ := s.StartCase(ctx, reqr, app.StartInput{ProcessKey: "purchase-request", FiscalYear: 2027})
	do(t, s, ctx, reqr, c.ID, "request", map[string]any{"amount": 1.0, "reviewers": []any{"IT"}})
	do(t, s, ctx, reviewer, c.ID, "review", map[string]any{"opinion": "agree"})

	v2, err := s.Clone(ctx, designer, v1, 2028)
	if err != nil {
		t.Fatal(err)
	}
	// add a step "policy check" between request and review,
	// and rename approve -> approve_v2 (removed + added)
	x := v2.BPMN
	x = strings.Replace(x, `<sequenceFlow id="f2" sourceRef="request" targetRef="review"/>`,
		`<userTask id="policy" name="ตรวจตามระเบียบ" cm:role="finance"></userTask>`+
			`<sequenceFlow id="f2" sourceRef="request" targetRef="policy"/><sequenceFlow id="f2b" sourceRef="policy" targetRef="review"/>`, 1)
	x = strings.ReplaceAll(x, `"approve"`, `"approve_v2"`)
	if _, _, err := s.UpdateDraft(ctx, designer, v2.ID, x, 0, ""); err != nil {
		t.Fatal(err)
	}
	// the renamed step loses its copied screen binding; publish refuses dangling ones
	if err := s.PutBindings(ctx, designer, v2.ID, []store.Binding{{NodeID: "approve_v2", UI: engine.UIForm, FormKey: "approval-form", ScreenKey: "task-approval"}}); err != nil {
		t.Fatal(err)
	}
	publish(t, s, ctx, v2.ID)

	// both years have their own published version
	if v, _ := s.ResolveVersion(ctx, "purchase-request", 2027); v.ID != v1 {
		t.Fatal("2027 must stay on v1")
	}
	if v, _ := s.ResolveVersion(ctx, "purchase-request", 2028); v.ID != v2.ID {
		t.Fatal("2028 must use v2")
	}
	d, _ := s.Diff(ctx, v1, v2.ID)
	if len(d.Added) != 2 || len(d.Removed) != 1 {
		t.Fatalf("diff %+v", d)
	}

	// case waits at approve which v2 removed -> mapping required
	res, err := s.Migrate(ctx, admin, app.MigrationRequest{From: v1, To: v2.ID})
	if err != nil || !res.DryRun || len(res.Unmapped) != 1 {
		t.Fatalf("expected unmapped step: %+v %v", res, err)
	}
	res, err = s.Migrate(ctx, admin, app.MigrationRequest{From: v1, To: v2.ID, Mapping: map[string]string{"approve": "policy"}})
	if err != nil || res.OK != 1 || res.Failed != 0 {
		t.Fatalf("migrate %+v %v", res, err)
	}
	if len(inbox(t, s, ctx, mgr, c.ID)) != 0 || len(inbox(t, s, ctx, finance, c.ID)) != 1 {
		t.Fatal("work items not moved to the mapped step")
	}
	c, _ = s.S.GetCase(ctx, c.ID)
	if c.VersionID != v2.ID {
		t.Fatal("case not on v2")
	}
}

func TestScreenModesToggleAndExternalCallback(t *testing.T) {
	s, ctx, _ := setup(t)
	v, _ := s.S.ListVersions(ctx, "purchase-request")
	draft, err := s.Clone(ctx, designer, v[0].ID, 2029)
	if err != nil {
		t.Fatal(err)
	}
	off := false
	err = s.PutBindings(ctx, designer, draft.ID, []store.Binding{
		{NodeID: "request", UI: engine.UIExternal, URL: "https://legacy.example/request", Embed: true},
		{NodeID: "review", Enabled: &off}, // step toggled off this year
	})
	if err != nil {
		t.Fatal(err)
	}
	publish(t, s, ctx, draft.ID)
	c, _ := s.StartCase(ctx, reqr, app.StartInput{ProcessKey: "purchase-request", FiscalYear: 2029})
	w := inbox(t, s, ctx, reqr, c.ID)[0]
	if w.UI != engine.UIExternal || w.URL == "" || !w.Embed {
		t.Fatalf("external binding not applied: %+v", w)
	}
	if _, err := s.CompleteByCallback(ctx, w.ID, "bad", nil); err == nil {
		t.Fatal("bad token accepted")
	}
	if _, err := s.CompleteByCallback(ctx, w.ID, s.CallbackToken(w.ID), map[string]any{"amount": 1.0, "reviewers": []any{"IT"}}); err != nil {
		t.Fatal(err)
	}
	// review skipped -> straight to approval
	if len(inbox(t, s, ctx, reviewer, c.ID)) != 0 || len(inbox(t, s, ctx, mgr, c.ID)) != 1 {
		t.Fatal("toggle did not skip step")
	}
	// supervisor sends the case back to the request step
	c, _ = s.S.GetCase(ctx, c.ID)
	if _, err := s.Move(ctx, mgr, c.ID, c.Instance.Tokens[0].ID, "request", ""); err == nil {
		t.Fatal("non-supervisor moved a case")
	}
	if _, err := s.Move(ctx, boss, c.ID, c.Instance.Tokens[0].ID, "request", "แก้ไขคำขอ"); err != nil {
		t.Fatal(err)
	}
	if len(inbox(t, s, ctx, reqr, c.ID)) != 1 {
		t.Fatal("case not sent back")
	}
}

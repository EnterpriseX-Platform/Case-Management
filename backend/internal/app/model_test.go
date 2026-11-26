package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

func stageOf(t *testing.T, s *app.Service, ctx context.Context, id string) string {
	t.Helper()
	c, err := s.S.GetCase(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return c.Stage
}

func overview(t *testing.T, s *app.Service, ctx context.Context, a app.Actor, id string) ([]app.StageView, map[string]bool) {
	t.Helper()
	c, _ := s.S.GetCase(ctx, id)
	st, acts, _ := s.CaseOverview(ctx, a, c)
	allowed := map[string]bool{}
	for _, x := range acts {
		allowed[x.Key] = x.Allowed
	}
	return st, allowed
}

func TestStagesAndData(t *testing.T) {
	s, ctx, _ := setup(t)
	c, err := s.StartCase(ctx, reqr, app.StartInput{ProcessKey: "purchase-request", FiscalYear: 2027})
	if err != nil {
		t.Fatal(err)
	}
	if got := stageOf(t, s, ctx, c.ID); got != "request" {
		t.Fatalf("stage %q", got)
	}
	// typed data: wrong type is rejected by the data model
	w := inbox(t, s, ctx, reqr, c.ID)[0]
	if _, err := s.Complete(ctx, reqr, w.ID, map[string]any{"amount": "lots", "reviewers": []any{"IT"}}); err == nil {
		t.Fatal("non-numeric amount accepted")
	}
	do(t, s, ctx, reqr, c.ID, "request", map[string]any{"amount": 1000.0, "reviewers": []any{"IT", "Legal"}})
	c, _ = s.S.GetCase(ctx, c.ID)
	if c.Stage != "review" || c.Instance.Vars["vat"] != 70.0 || c.Instance.Vars["total"] != 1070.0 {
		t.Fatalf("stage=%s vat=%v total=%v", c.Stage, c.Instance.Vars["vat"], c.Instance.Vars["total"])
	}
	for _, r := range inbox(t, s, ctx, reviewer, c.ID) {
		s.Complete(ctx, reviewer, r.ID, map[string]any{"opinion": "agree"})
	}
	if got := stageOf(t, s, ctx, c.ID); got != "approval" {
		t.Fatalf("stage %q", got)
	}
	// cross-field rule
	ap := inbox(t, s, ctx, mgr, c.ID)[0]
	if _, err := s.Complete(ctx, mgr, ap.ID, map[string]any{"approvedAmount": 5000.0, "remark": "x", "withinLimit": true}); err == nil {
		t.Fatal("approved > requested accepted")
	}
	do(t, s, ctx, mgr, c.ID, "approve", map[string]any{"approvedAmount": 900.0, "remark": "ok", "withinLimit": true})
	if got := stageOf(t, s, ctx, c.ID); got != "fulfilment" {
		t.Fatalf("stage %q", got)
	}
	st, _ := overview(t, s, ctx, mgr, c.ID)
	want := map[string]string{"request": "done", "review": "done", "approval": "done", "fulfilment": "current"}
	for _, x := range st {
		if w, ok := want[x.Key]; ok && x.State != w {
			t.Fatalf("stage %s is %s, want %s", x.Key, x.State, w)
		}
		if x.Kind == "alternate" {
			t.Fatal("alternate stage shown before it was entered")
		}
	}
	do(t, s, ctx, buyer, c.ID, "purchase", map[string]any{"poNumber": "PO-9", "vendor": "V", "poDate": "2026-12-01"})
	do(t, s, ctx, reqr, c.ID, "receive", map[string]any{"result": "accepted", "receiveDate": "2026-12-05"})
	do(t, s, ctx, finance, c.ID, "accounting", map[string]any{"voucher": "V1", "postDate": "2026-12-06"})
	c, _ = s.S.GetCase(ctx, c.ID)
	if c.Status != "completed" || c.Stage != "closed" || len(c.StageHistory) != 5 {
		t.Fatalf("status=%s stage=%s history=%d", c.Status, c.Stage, len(c.StageHistory))
	}
}

func TestWithdrawActionEntersAlternateStage(t *testing.T) {
	s, ctx, _ := setup(t)
	c, _ := s.StartCase(ctx, reqr, app.StartInput{ProcessKey: "purchase-request", FiscalYear: 2027})
	do(t, s, ctx, reqr, c.ID, "request", map[string]any{"amount": 1000.0, "reviewers": []any{"IT"}})

	_, allowed := overview(t, s, ctx, reqr, c.ID)
	if !allowed["withdraw"] || !allowed["update-details"] || allowed["sub-request"] {
		t.Fatalf("actions in review stage: %v", allowed)
	}
	if _, err := s.RunAction(ctx, reviewer, c.ID, "withdraw", map[string]any{"withdrawReason": "x"}); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("reviewer withdrew: %v", err)
	}
	if _, err := s.RunAction(ctx, reqr, c.ID, "withdraw", map[string]any{}); err == nil {
		t.Fatal("action form validation skipped")
	}
	if _, err := s.RunAction(ctx, reqr, c.ID, "update-details", map[string]any{"priority": "very"}); err == nil {
		t.Fatal("invalid enum accepted")
	}
	if _, err := s.RunAction(ctx, reqr, c.ID, "update-details", map[string]any{"priority": "urgent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunAction(ctx, reqr, c.ID, "withdraw", map[string]any{"withdrawReason": "no longer needed"}); err != nil {
		t.Fatal(err)
	}
	c, _ = s.S.GetCase(ctx, c.ID)
	if c.Stage != "withdrawn" || c.Instance.Vars["priority"] != "urgent" {
		t.Fatalf("stage=%s vars=%v", c.Stage, c.Instance.Vars)
	}
	if len(inbox(t, s, ctx, reviewer, c.ID)) != 0 {
		t.Fatal("review work not cancelled")
	}
	do(t, s, ctx, reqr, c.ID, "withdrawn", map[string]any{"note": "recorded"})
	if c, _ = s.S.GetCase(ctx, c.ID); c.Status != "completed" || c.Stage != "closed" {
		t.Fatalf("status=%s stage=%s", c.Status, c.Stage)
	}
	if _, allowed = overview(t, s, ctx, reqr, c.ID); allowed["withdraw"] {
		t.Fatal("actions available on a closed case")
	}
}

const parentFlow = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:cm="http://enterprisex.io/schema/cm/1.0">
<process id="parent"><startEvent id="s"/><userTask id="prepare" cm:role="clerk"/>
<receiveTask id="waitkids" name="children.completed"/><userTask id="consolidate" cm:role="clerk"/><endEvent id="e"/>
<sequenceFlow id="1" sourceRef="s" targetRef="prepare"/><sequenceFlow id="2" sourceRef="prepare" targetRef="waitkids"/>
<sequenceFlow id="3" sourceRef="waitkids" targetRef="consolidate"/><sequenceFlow id="4" sourceRef="consolidate" targetRef="e"/>
</process></definitions>`

const childFlow = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:cm="http://enterprisex.io/schema/cm/1.0">
<process id="child"><startEvent id="s"/><userTask id="work" cm:role="clerk"/><endEvent id="e"/>
<sequenceFlow id="1" sourceRef="s" targetRef="work"/><sequenceFlow id="2" sourceRef="work" targetRef="e"/></process></definitions>`

// Child cases opened by an action; the parent waits until all are closed.
func TestChildCasesWaitForAll(t *testing.T) {
	ctx := context.Background()
	s := app.New(newStore(t), nil, []byte("x"))
	for _, d := range []struct{ key, xml string }{{"agency-request", childFlow}, {"consolidation", parentFlow}} {
		if _, err := s.CreateProcess(ctx, designer, store.ProcessDef{Key: d.key, Name: d.key}); err != nil {
			t.Fatal(err)
		}
		v, _, err := s.CreateDraft(ctx, designer, d.key, d.xml, "", 2027)
		if err != nil {
			t.Fatal(err)
		}
		if d.key == "consolidation" {
			m := &store.CaseModel{
				Stages: []store.Stage{{Key: "collect", Name: "Collect", Kind: "primary", Steps: []string{"prepare", "waitkids"}},
					{Key: "consolidate", Name: "Consolidate", Kind: "primary", Steps: []string{"consolidate"}}},
				Data: []store.DataField{{Key: "ceiling", Label: "Ceiling", Type: "money"}},
				Actions: []store.CaseAction{{Key: "add-agency", Label: "Add agency request", Kind: "child",
					Target: "agency-request", Stages: []string{"collect"}, Pass: []string{"ceiling"}}},
			}
			if _, err := s.PutModel(ctx, designer, v.ID, m); err != nil {
				t.Fatal(err)
			}
		}
		publish(t, s, ctx, v.ID)
	}
	p, _ := s.StartCase(ctx, clerk, app.StartInput{ProcessKey: "consolidation", FiscalYear: 2027, Vars: map[string]any{"ceiling": 5e6}})
	var kids []*store.Case
	for i := 0; i < 2; i++ {
		k, err := s.RunAction(ctx, clerk, p.ID, "add-agency", map[string]any{"title": "Agency"})
		if err != nil {
			t.Fatal(err)
		}
		if k.ParentID != p.ID || k.Instance.Vars["ceiling"] != 5e6 {
			t.Fatalf("child %+v", k.Instance.Vars)
		}
		kids = append(kids, k)
	}
	do(t, s, ctx, clerk, p.ID, "prepare", nil) // parent now waits for all children
	do(t, s, ctx, clerk, kids[0].ID, "work", nil)
	if tokenAt(t, s, ctx, p.ID, "waitkids") == nil {
		t.Fatal("parent continued before all children closed")
	}
	do(t, s, ctx, clerk, kids[1].ID, "work", nil)
	if tokenAt(t, s, ctx, p.ID, "consolidate") == nil || stageOf(t, s, ctx, p.ID) != "consolidate" {
		t.Fatal("parent did not continue after the last child closed")
	}
	// related cases are linked both ways
	if err := s.Relate(ctx, clerk, kids[0].ID, kids[1].ID, true); err != nil {
		t.Fatal(err)
	}
	k1, _ := s.S.GetCase(ctx, kids[1].ID)
	if len(k1.RelatedIDs) != 1 || k1.RelatedIDs[0] != kids[0].ID {
		t.Fatalf("related: %v", k1.RelatedIDs)
	}
}

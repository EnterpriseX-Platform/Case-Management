package client_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/api"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
	"github.com/EnterpriseX-Platform/Case-Management/backend/pkg/client"
)

const flow = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:cm="http://enterprisex.io/schema/cm/1.0">
<process id="p"><startEvent id="s"/><userTask id="fill" cm:role="clerk"/><receiveTask id="ack" name="ack"/><endEvent id="e"/>
<sequenceFlow id="1" sourceRef="s" targetRef="fill"/><sequenceFlow id="2" sourceRef="fill" targetRef="ack"/><sequenceFlow id="3" sourceRef="ack" targetRef="e"/></process></definitions>`

func TestSDK(t *testing.T) {
	ctx := context.Background()
	svc := app.New(store.NewMemory(), nil, []byte("x"))
	d := app.Actor{ID: "d", Roles: []string{app.RoleDesigner}}
	a := app.Actor{ID: "a", Roles: []string{app.RoleApprover}}
	svc.CreateProcess(ctx, d, store.ProcessDef{Key: "demo", Name: "Demo"})
	v, _, _ := svc.CreateDraft(ctx, d, "demo", flow, "", 2027)
	svc.Transition(ctx, d, v.ID, "submit")
	svc.Transition(ctx, a, v.ID, "approve")
	if _, err := svc.Transition(ctx, a, v.ID, "publish"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer((&api.Server{S: svc}).Handler())
	defer srv.Close()

	c := client.New(srv.URL, client.DevUser("u1", "clerk"))
	cs, err := c.StartCase(ctx, client.StartInput{ProcessKey: "demo", Vars: map[string]any{"amount": 5.0}})
	if err != nil || cs.Number == "" {
		t.Fatal(err)
	}
	ws, err := c.MyWork(ctx)
	if err != nil || len(ws) != 1 {
		t.Fatalf("work: %v %v", ws, err)
	}
	if _, err := c.CompleteWork(ctx, ws[0].ID, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if n, err := c.Message(ctx, "ack", cs.ID, "", nil); err != nil || n != 1 {
		t.Fatalf("message: %d %v", n, err)
	}
	got, err := c.GetCase(ctx, cs.ID)
	if err != nil || got.Status != "completed" || got.Instance.Vars["ok"] != true {
		t.Fatalf("case %+v %v", got, err)
	}
	if _, err := client.New(srv.URL).GetCase(ctx, cs.ID); err == nil {
		t.Fatal("unauthenticated call accepted")
	}
}

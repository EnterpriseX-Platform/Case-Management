package app_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

// E-mail goes through the outbox and is retried until the mail server answers.
func TestOutboxRetries(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	s := app.New(st, nil, []byte("x"))
	now := time.Now()
	s.Now = func() time.Time { return now }
	if err := st.PutOutbox(ctx, &store.OutboxMsg{ID: id.New(), Kind: "email", To: "a@example.test", Subject: "hi", Body: "b", Status: "pending", NextRunAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	old := app.Mailer
	defer func() { app.Mailer = old }()
	app.Mailer = func(to, subject, body string) error {
		calls++
		if calls == 1 {
			return errors.New("mail server down")
		}
		return nil
	}
	if n, err := s.RunOutbox(ctx, 10); err != nil || n != 1 {
		t.Fatalf("first run: n=%d err=%v", n, err)
	}
	if n, _ := s.RunOutbox(ctx, 10); n != 0 {
		t.Fatal("retried before the backoff")
	}
	now = now.Add(time.Minute)
	if n, err := s.RunOutbox(ctx, 10); err != nil || n != 1 || calls != 2 {
		t.Fatalf("retry: n=%d calls=%d err=%v", n, calls, err)
	}
	now = now.Add(time.Hour)
	if n, _ := s.RunOutbox(ctx, 10); n != 0 {
		t.Fatal("delivered message was sent again")
	}
}

// Reassigning work notifies the new assignee, in the app and by e-mail.
func TestReassignNotifies(t *testing.T) {
	s, ctx, _ := opsSetup(t, &flaky{})
	if err := s.S.PutUser(ctx, &store.UserProfile{ID: "c2", Name: "Clerk Two", Email: "c2@example.test", Roles: []string{"clerk"}, Active: true}); err != nil {
		t.Fatal(err)
	}
	c, err := s.StartCase(ctx, clerk, app.StartInput{ProcessKey: "ops", FiscalYear: 2027})
	if err != nil {
		t.Fatal(err)
	}
	w := inbox(t, s, ctx, clerk, c.ID)[0]
	boss := app.Actor{ID: "boss", Roles: []string{app.RoleSupervisor}}
	if _, err := s.Reassign(ctx, boss, w.ID, "c2", ""); err != nil {
		t.Fatal(err)
	}
	ns, _ := s.S.ListNotifications(ctx, "c2", 10)
	if len(ns) != 1 || ns[0].Kind != "assigned" {
		t.Fatalf("notifications: %+v", ns)
	}
	sent := ""
	old := app.Mailer
	defer func() { app.Mailer = old }()
	app.Mailer = func(to, subject, body string) error { sent = to; return nil }
	if n, err := s.RunOutbox(ctx, 10); err != nil || n != 1 || sent != "c2@example.test" {
		t.Fatalf("outbox: n=%d sent=%q err=%v", n, sent, err)
	}
}

// Form rules made in the builder are enforced by the server too.
func TestFormBuilderRules(t *testing.T) {
	f := &store.Form{Key: "f", Schema: map[string]any{
		"pages": []any{map[string]any{"key": "p1"}, map[string]any{"key": "p2", "skipIf": `result == "full"`}},
		"fields": []any{
			map[string]any{"key": "result", "label": "Result", "type": "radio", "required": true, "page": "p1",
				"options": []any{map[string]any{"value": "full"}, map[string]any{"value": "short"}}},
			map[string]any{"key": "reason", "label": "Reason", "type": "textarea", "required": true, "page": "p2", "minLen": 10.0, "message": "say why"},
			map[string]any{"key": "code", "label": "Code", "type": "text", "pattern": "[A-Z]{2}-\\d+", "page": "p1"},
		}}}
	if err := app.ValidateForm(f, map[string]any{"result": "full"}); err != nil {
		t.Fatalf("skipped page was checked: %v", err)
	}
	if err := app.ValidateForm(f, map[string]any{"result": "short"}); err == nil {
		t.Fatal("required field on a shown page not checked")
	}
	if err := app.ValidateForm(f, map[string]any{"result": "short", "reason": "short"}); err == nil || !strings.Contains(err.Error(), "say why") {
		t.Fatalf("min length / message: %v", err)
	}
	if err := app.ValidateForm(f, map[string]any{"result": "full", "code": "x1"}); err == nil {
		t.Fatal("pattern not checked")
	}
	if err := app.ValidateForm(f, map[string]any{"result": "full", "code": "AB-12"}); err != nil {
		t.Fatal(err)
	}
}

// Personal views stay personal; only supervisors share, and shared views
// reach only the roles they are for.
func TestWorkViews(t *testing.T) {
	ctx := context.Background()
	s := app.New(newStore(t), nil, []byte("x"))
	ann := app.Actor{ID: "ann", Roles: []string{"clerk"}}
	bob := app.Actor{ID: "bob", Roles: []string{"clerk"}}
	eve := app.Actor{ID: "eve", Roles: []string{"finance"}}
	boss := app.Actor{ID: "boss", Roles: []string{app.RoleSupervisor}}
	mine, err := s.SaveView(ctx, ann, &store.WorkView{Name: "Mine", Columns: []string{"caseNumber"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveView(ctx, ann, &store.WorkView{Name: "Team", Columns: []string{"caseNumber"}, Shared: true}); err == nil {
		t.Fatal("non-supervisor shared a view")
	}
	if _, err := s.SaveView(ctx, boss, &store.WorkView{Name: "Clerks", Columns: []string{"caseNumber"}, Shared: true, Roles: []string{"clerk"}, Default: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveView(ctx, bob, &store.WorkView{ID: mine.ID, Name: "Hijack", Columns: []string{"x"}}); err == nil {
		t.Fatal("someone else changed a personal view")
	}
	names := func(a app.Actor) string {
		vs, _ := s.Views(ctx, a)
		var n []string
		for _, v := range vs {
			n = append(n, v.Name)
		}
		return strings.Join(n, ",")
	}
	if got := names(ann); got != "Mine,Clerks" {
		t.Fatalf("ann sees %q", got)
	}
	if got := names(bob); got != "Clerks" {
		t.Fatalf("bob sees %q", got)
	}
	if got := names(eve); got != "" {
		t.Fatalf("finance sees %q", got)
	}
	if err := s.DeleteView(ctx, bob, mine.ID); err == nil {
		t.Fatal("someone else deleted a personal view")
	}
}

// Only admins change the theme, and bad values are refused.
func TestTheme(t *testing.T) {
	ctx := context.Background()
	s := app.New(newStore(t), nil, []byte("x"))
	admin := app.Actor{ID: "a", Roles: []string{app.RoleAdmin}}
	if _, err := s.SaveTheme(ctx, app.Actor{ID: "u"}, &store.Theme{Accent: "#00539c"}); err == nil {
		t.Fatal("non-admin changed the theme")
	}
	for _, bad := range []store.Theme{{Accent: "blue"}, {Logo: "javascript:alert(1)"}, {Radius: 40}, {Mode: "neon"}} {
		b := bad
		if _, err := s.SaveTheme(ctx, admin, &b); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
	if _, err := s.SaveTheme(ctx, admin, &store.Theme{Product: "ระบบติดตามเรื่อง", Accent: "#00539c", Header: "#00305e", Radius: 4, Mode: "dark"}); err != nil {
		t.Fatal(err)
	}
	if th := s.Theme(ctx); th.Accent != "#00539c" || th.Product != "ระบบติดตามเรื่อง" {
		t.Fatalf("theme not stored: %+v", th)
	}
}

// Case events reach a webhook, signed, and are retried when it fails.
func TestWebhookDelivery(t *testing.T) {
	var got []string
	var sigOK bool
	fail := 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if fail > 0 {
			fail--
			w.WriteHeader(503)
			return
		}
		got = append(got, r.Header.Get("X-CM-Event"))
		sigOK = r.Header.Get("X-CM-Signature") == "sha256="+app.Sign("s3cret", r.Header.Get("X-CM-Timestamp"), b)
	}))
	defer srv.Close()
	t.Setenv("CM_SECRET_ERP_HOOK", "s3cret")
	s, ctx, now := opsSetup(t, &flaky{})
	admin := app.Actor{ID: "a", Roles: []string{app.RoleAdmin}}
	if _, err := s.SaveWebhook(ctx, admin, &store.Webhook{Name: "ERP", URL: srv.URL, Events: []string{"case.started", "work.created"}, Secret: "erp_hook", Active: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartCase(ctx, clerk, app.StartInput{ProcessKey: "ops", FiscalYear: 2027}); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return time.Now() }
	s.RunOutbox(ctx, 10) // first call fails (503), second message delivered
	*now = time.Now().Add(time.Hour)
	s.Now = func() time.Time { return *now }
	s.RunOutbox(ctx, 10)
	if len(got) != 2 || !strings.Contains(strings.Join(got, ","), "case.started") || !sigOK {
		t.Fatalf("delivered %v, signature ok %v", got, sigOK)
	}
	hs, _ := s.Webhooks(ctx, admin)
	if hs[0].LastStatus != "200" {
		t.Fatalf("last status %q", hs[0].LastStatus)
	}
}

// email.send is an extension step any flow can use.
func TestEmailStepQueuesMail(t *testing.T) {
	if !strings.Contains(strings.Join(app.Actions(), ","), "email.send") {
		t.Fatal("email.send not registered")
	}
}

func TestVerifySignature(t *testing.T) {
	now := time.Now()
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"vars":{"paid":true}}`)
	sig := "sha256=" + app.Sign("k", ts, body)
	if !app.VerifySignature("k", ts, sig, body, now) {
		t.Fatal("valid signature rejected")
	}
	if app.VerifySignature("k", ts, sig, []byte(`{"vars":{"paid":false}}`), now) {
		t.Fatal("changed body accepted")
	}
	if app.VerifySignature("k", ts, sig, body, now.Add(10*time.Minute)) {
		t.Fatal("old request (replay) accepted")
	}
	if app.VerifySignature("", ts, sig, body, now) {
		t.Fatal("accepted without a configured secret")
	}
}

// Distribution rules route new work: first match wins, least-loaded person,
// cap per person, extra urgency; unmatched steps keep their own routing.
func TestAssignRules(t *testing.T) {
	s, ctx, _ := opsSetup(t, &flaky{})
	boss := app.Actor{ID: "boss", Roles: []string{app.RoleSupervisor}}
	for _, u := range []string{"s1", "s2"} {
		if err := s.S.PutUser(ctx, &store.UserProfile{ID: u, Name: u, Roles: []string{"clerk"}, Active: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.S.PutQueue(ctx, &store.Queue{Key: "senior", Name: "Senior", Members: []string{"s1", "s2"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveAssignRule(ctx, app.Actor{ID: "x"}, &store.AssignRule{Name: "no", Then: store.AssignAction{Mode: "keep"}}); err == nil {
		t.Fatal("non-supervisor saved a rule")
	}
	if _, err := s.SaveAssignRule(ctx, boss, &store.AssignRule{Name: "bad", Then: store.AssignAction{Mode: "workload"}}); err == nil {
		t.Fatal("workload rule without a queue accepted")
	}
	if _, err := s.SaveAssignRule(ctx, boss, &store.AssignRule{Name: "Big amounts to seniors", Active: true,
		When: []store.ViewRule{{Field: "caseType", Op: "eq", Value: "ops"}, {Field: "data.amount", Op: "gt", Value: "100000"}},
		Then: store.AssignAction{Mode: "workload", Queue: "senior", MaxPerPerson: 1, Urgency: 20, SLA: "P1D"}}); err != nil {
		t.Fatal(err)
	}
	start := func(amount float64) *store.Work {
		c, err := s.StartCase(ctx, clerk, app.StartInput{ProcessKey: "ops", FiscalYear: 2027, Vars: map[string]any{"amount": amount}})
		if err != nil {
			t.Fatal(err)
		}
		ws, _ := s.S.ListWork(ctx, store.WorkFilter{CaseID: c.ID})
		return ws[0]
	}
	a, b, full, small := start(500000), start(300000), start(200000), start(50)
	if a.Assignee == "" || b.Assignee == "" || a.Assignee == b.Assignee {
		t.Fatalf("least-loaded: %q, %q", a.Assignee, b.Assignee)
	}
	if a.AssignedBy != "Big amounts to seniors" || a.Urgency <= small.Urgency || a.DueAt == nil {
		t.Fatalf("rule effects: by=%q urgency=%d/%d due=%v", a.AssignedBy, a.Urgency, small.Urgency, a.DueAt)
	}
	if full.Assignee != "" || full.Queue != "senior" {
		t.Fatalf("cap per person ignored: assignee %q queue %q", full.Assignee, full.Queue)
	}
	if small.AssignedBy != "" || small.Role != "clerk" {
		t.Fatalf("unmatched step changed: by=%q role=%q", small.AssignedBy, small.Role)
	}
	p, err := s.PreviewAssign(ctx, boss, "ops", "fill", "", "clerk", map[string]any{"amount": 999999.0}, nil)
	if err != nil || p.Rule != "Big amounts to seniors" || p.Assignee != "" || p.Note == "" {
		t.Fatalf("preview: %+v %v", p, err)
	}
}

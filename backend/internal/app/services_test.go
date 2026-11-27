package app_test

import (
	"context"
	"errors"
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

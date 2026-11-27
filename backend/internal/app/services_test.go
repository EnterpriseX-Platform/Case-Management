package app_test

import (
	"context"
	"errors"
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

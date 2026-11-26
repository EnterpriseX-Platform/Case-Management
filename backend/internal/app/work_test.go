package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

const oneStep = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:cm="http://enterprisex.io/schema/cm/1.0">
<process id="p"><startEvent id="s"/><userTask id="triage" name="Triage" cm:role="clerk"/><endEvent id="e"/>
<sequenceFlow id="1" sourceRef="s" targetRef="triage"/><sequenceFlow id="2" sourceRef="triage" targetRef="e"/></process></definitions>`

type wm struct {
	s   *app.Service
	ctx context.Context
	now *time.Time
	key string
}

func wmSetup(t *testing.T, b store.Binding) *wm {
	ctx := context.Background()
	s := app.New(newStore(t), nil, []byte("x"))
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC) // Monday
	s.Now = func() time.Time { return now }
	s.E.Now = s.Now
	s.CreateProcess(ctx, designer, store.ProcessDef{Key: "intake", Name: "Intake"})
	v, _, _ := s.CreateDraft(ctx, designer, "intake", oneStep, "", 2027)
	b.NodeID = "triage"
	if err := s.PutBindings(ctx, designer, v.ID, []store.Binding{b}); err != nil {
		t.Fatal(err)
	}
	publish(t, s, ctx, v.ID)
	for _, u := range []*store.UserProfile{
		{ID: "u1", Roles: []string{"clerk"}, Active: true, Skills: []string{"legal"}},
		{ID: "u2", Roles: []string{"clerk"}, Active: true},
		{ID: "u3", Roles: []string{"clerk"}, Active: true},
		{ID: "boss", Roles: []string{app.RoleSupervisor}, Active: true},
	} {
		s.S.PutUser(ctx, u)
	}
	s.S.PutQueue(ctx, &store.Queue{Key: "triage", Name: "Triage desk", Members: []string{"u1", "u2"}, Supervisors: []string{"boss"}})
	return &wm{s, ctx, &now, "intake"}
}

func (w *wm) start(t *testing.T) *store.Work {
	t.Helper()
	c, err := w.s.StartCase(w.ctx, clerk, app.StartInput{ProcessKey: w.key, FiscalYear: 2027, Vars: map[string]any{"owner": "u3"}})
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := w.s.S.ListWork(w.ctx, store.WorkFilter{CaseID: c.ID})
	return ws[0]
}

func TestRoutingWorkloadRoundRobinSkill(t *testing.T) {
	w := wmSetup(t, store.Binding{Route: &store.Routing{Mode: "workload", Queue: "triage"}})
	a, b, c := w.start(t), w.start(t), w.start(t)
	if a.Assignee == "" || a.Assignee == b.Assignee || c.Assignee == "" {
		t.Fatalf("workload: %s %s %s", a.Assignee, b.Assignee, c.Assignee)
	}
	// capacity: u1 can hold only 1 more item than now
	w2 := wmSetup(t, store.Binding{Route: &store.Routing{Mode: "roundrobin", Queue: "triage"}})
	x, y, z := w2.start(t), w2.start(t), w2.start(t)
	if x.Assignee == y.Assignee || x.Assignee != z.Assignee {
		t.Fatalf("round robin: %s %s %s", x.Assignee, y.Assignee, z.Assignee)
	}
	w3 := wmSetup(t, store.Binding{Route: &store.Routing{Mode: "skill", Queue: "triage", Skill: "legal"}})
	if got := w3.start(t).Assignee; got != "u1" {
		t.Fatalf("skill routed to %q", got)
	}
	w4 := wmSetup(t, store.Binding{Route: &store.Routing{Mode: "user", User: "${owner}"}})
	if got := w4.start(t).Assignee; got != "u3" {
		t.Fatalf("expression user routed to %q", got)
	}
}

func TestQueueVisibilityAndNextWork(t *testing.T) {
	w := wmSetup(t, store.Binding{Route: &store.Routing{Mode: "queue", Queue: "triage"}, Urgency: 10})
	first, second := w.start(t), w.start(t)
	member := app.Actor{ID: "u2", Roles: []string{"clerk"}}
	outsider := app.Actor{ID: "u3", Roles: []string{"clerk"}}
	if in, _ := w.s.Inbox(w.ctx, outsider, false); len(in) != 0 {
		t.Fatal("queue work visible to a non-member")
	}
	if in, _ := w.s.Inbox(w.ctx, member, false); len(in) != 2 {
		t.Fatalf("member sees %d items", len(in))
	}
	// raise urgency of the second item: get-next-work must hand it out first
	if _, err := w.s.Bulk(w.ctx, member, []string{second.ID}, "urgency", "", 50); err == nil {
		t.Fatal("bulk allowed for non-supervisor")
	}
	if n, err := w.s.Bulk(w.ctx, boss, []string{second.ID}, "urgency", "", 50); err != nil || n != 1 {
		t.Fatal(err)
	}
	got, err := w.s.NextWork(w.ctx, member)
	if err != nil || got.ID != second.ID || got.Assignee != "u2" {
		t.Fatalf("next work: %+v %v", got, err)
	}
	if again, _ := w.s.NextWork(w.ctx, member); again.ID != second.ID {
		t.Fatal("next work should return the item already held")
	}
	if _, err := w.s.Complete(w.ctx, member, second.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.s.NextWork(w.ctx, member); got == nil || got.ID != first.ID {
		t.Fatal("next work did not move on")
	}
	// bulk transfer back to a role queue
	if n, _ := w.s.Bulk(w.ctx, boss, []string{first.ID}, "transfer", "clerk", 0); n != 1 {
		t.Fatal("transfer failed")
	}
	if in, _ := w.s.Inbox(w.ctx, outsider, false); len(in) != 1 {
		t.Fatal("transferred work not visible to the role")
	}
}

func TestDelegationWhenAway(t *testing.T) {
	w := wmSetup(t, store.Binding{Route: &store.Routing{Mode: "user", User: "u1"}})
	from, to := *w.now, w.now.Add(72*time.Hour)
	if _, err := w.s.SaveProfile(w.ctx, app.Actor{ID: "u1", Roles: []string{"clerk"}}, &store.UserProfile{ID: "u1", OOO: &store.OutOfOffice{From: &from, To: &to, Delegate: "u2"}}); err != nil {
		t.Fatal(err)
	}
	got := w.start(t)
	if got.Assignee != "u2" || got.DelegatedFrom != "u1" {
		t.Fatalf("delegation: %+v", got)
	}
	if ns, _ := w.s.S.ListNotifications(w.ctx, "u2", 10); len(ns) != 1 || ns[0].Kind != "assigned" {
		t.Fatalf("delegate not notified: %+v", ns)
	}
}

func TestSLAGoalDeadlineEscalation(t *testing.T) {
	w := wmSetup(t, store.Binding{SLA: "P2D", Goal: "P1D", Route: &store.Routing{Mode: "user", User: "u1"},
		Escalate: []store.Escalation{{At: "deadline", Do: "queue", Target: "triage"}, {At: "deadline", Do: "notify", Target: "supervisors"}}})
	item := w.start(t)
	if item.GoalAt == nil || item.DueAt == nil || item.Urgency != 10 {
		t.Fatalf("sla not set: %+v", item)
	}
	*w.now = w.now.Add(25 * time.Hour)
	if n, err := w.s.RunSLA(w.ctx); err != nil || n != 1 {
		t.Fatalf("goal run: %d %v", n, err)
	}
	got, _ := w.s.S.GetWork(w.ctx, item.ID)
	if got.Urgency != 30 || !got.GoalDone || got.Assignee != "u1" {
		t.Fatalf("after goal: %+v", got)
	}
	if n, _ := w.s.RunSLA(w.ctx); n != 0 {
		t.Fatal("goal escalation ran twice")
	}
	*w.now = w.now.Add(24 * time.Hour)
	w.s.RunSLA(w.ctx)
	got, _ = w.s.S.GetWork(w.ctx, item.ID)
	if got.Assignee != "" || got.Queue != "triage" || !got.DeadlineDone {
		t.Fatalf("after deadline: %+v", got)
	}
	if ns, _ := w.s.S.ListNotifications(w.ctx, "boss", 10); len(ns) == 0 {
		t.Fatal("supervisor not notified")
	}
}

func TestBusinessCalendar(t *testing.T) {
	w := wmSetup(t, store.Binding{SLA: "PT4H", Route: &store.Routing{Mode: "user", User: "u1"}})
	w.s.S.PutSettings(w.ctx, &store.Settings{Calendar: store.Calendar{Enabled: true, Days: []int{1, 2, 3, 4, 5},
		Start: "08:30", End: "16:30", TZ: "UTC", Holidays: []string{"2026-11-09"}}})
	*w.now = time.Date(2026, 11, 6, 15, 0, 0, 0, time.UTC) // Friday 15:00
	item := w.start(t)
	// 1.5 h on Friday, Monday 9 Nov is a holiday, 2.5 h on Tuesday from 08:30
	want := time.Date(2026, 11, 10, 11, 0, 0, 0, time.UTC)
	if item.DueAt == nil || !item.DueAt.Equal(want) {
		t.Fatalf("due %v, want %v", item.DueAt, want)
	}
}

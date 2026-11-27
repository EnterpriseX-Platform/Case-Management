package app

// Work management: routing, queues, delegation, SLA goal/deadline with
// escalation, urgency, business calendar, get-next-work, bulk actions and
// notifications.

import (
	"context"
	"errors"
	"fmt"
	"net/smtp"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

// Work items routed to a queue carry the role "queue:<key>"; members of the
// queue get it as an effective role.
const queuePrefix = "queue:"

const defaultUrgency = 10

var seen sync.Map // user id -> last profile refresh

// TouchUser registers / refreshes the caller's profile (at most every 5 min).
func (s *Service) TouchUser(ctx context.Context, a Actor) {
	if a.ID == "" || strings.HasPrefix(a.ID, "system") {
		return
	}
	if t, ok := seen.Load(a.ID); ok && time.Since(t.(time.Time)) < 5*time.Minute {
		return
	}
	seen.Store(a.ID, time.Now())
	u, err := s.S.GetUser(ctx, a.ID)
	if err != nil {
		u = &store.UserProfile{ID: a.ID, Active: true}
	}
	u.Name = firstNonEmpty(a.Name, u.Name, a.ID)
	u.Roles = a.Roles
	u.LastSeen = s.Now()
	_ = s.S.PutUser(ctx, u)
}

// effectiveRoles = IAM roles + queue memberships.
func (s *Service) effectiveRoles(ctx context.Context, a Actor) []string {
	roles := append([]string(nil), a.Roles...)
	qs, _ := s.S.ListQueues(ctx)
	for _, q := range qs {
		if s.inQueue(q, a.ID, a.Roles) {
			roles = append(roles, queuePrefix+q.Key)
		}
	}
	return roles
}

func (s *Service) inQueue(q *store.Queue, user string, roles []string) bool {
	if contains(q.Members, user) {
		return true
	}
	for _, r := range q.Roles {
		if contains(roles, r) {
			return true
		}
	}
	return false
}

// candidates returns active, present users of a queue (or a role).
func (s *Service) candidates(ctx context.Context, queue, role, skill string) []*store.UserProfile {
	users, _ := s.S.ListUsers(ctx)
	var q *store.Queue
	if queue != "" {
		q, _ = s.S.GetQueue(ctx, queue)
	}
	now := s.Now()
	var out []*store.UserProfile
	for _, u := range users {
		if !u.Active || u.Away(now) {
			continue
		}
		member := false
		switch {
		case q != nil:
			member = s.inQueue(q, u.ID, u.Roles)
		case role != "":
			member = contains(u.Roles, role)
		}
		if !member || (skill != "" && !contains(u.Skills, skill)) {
			continue
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// prepareWork applies routing, delegation, SLA goal/deadline (in business
// time when the calendar is on) and urgency to a new work item.
func (s *Service) prepareWork(ctx context.Context, w *store.Work, b *store.Binding, vars map[string]any, now time.Time) error {
	w.Urgency = defaultUrgency
	if b != nil && b.Urgency > 0 {
		w.Urgency = b.Urgency
	}
	cal := s.calendar(ctx)
	if w.SLA != "" {
		if d, err := engine.ParseDuration(w.SLA); err == nil {
			due := cal.Add(now, d)
			w.DueAt = &due
		}
	}
	if b != nil && b.Goal != "" {
		if d, err := engine.ParseDuration(b.Goal); err == nil {
			g := cal.Add(now, d)
			w.GoalAt = &g
		}
	}
	if b == nil || b.Route == nil {
		return s.delegate(ctx, w)
	}
	r := b.Route
	pick := func(list []*store.UserProfile) string {
		if len(list) == 0 {
			return ""
		}
		counts, _ := s.S.OpenCounts(ctx)
		best := ""
		bestN := 1 << 30
		for _, u := range list {
			n := counts[u.ID]
			if u.Capacity > 0 && n >= u.Capacity {
				continue
			}
			if n < bestN {
				best, bestN = u.ID, n
			}
		}
		return best
	}
	switch r.Mode {
	case "queue":
		w.Role, w.Queue = queuePrefix+r.Queue, r.Queue
	case "user":
		u := r.User
		if strings.HasPrefix(u, "${") {
			if v, err := engine.EvalAny(u, vars); err == nil {
				u = fmt.Sprint(v)
			}
		}
		w.Assignee = u
	case "expression":
		if v, err := engine.EvalAny(r.Expr, vars); err == nil && v != nil {
			w.Assignee = fmt.Sprint(v)
		}
	case "workload", "skill":
		w.Assignee = pick(s.candidates(ctx, r.Queue, w.Role, r.Skill))
		if r.Queue != "" {
			w.Queue = r.Queue
			if w.Assignee == "" {
				w.Role = queuePrefix + r.Queue
			}
		}
	case "roundrobin":
		list := s.candidates(ctx, r.Queue, w.Role, "")
		if len(list) > 0 {
			key := firstNonEmpty(r.Queue, "_rr-"+w.Role) // "_rr-" = internal cursor, not a real queue
			q, err := s.S.GetQueue(ctx, key)
			if err != nil {
				q = &store.Queue{Key: key, Name: key}
			}
			w.Assignee = list[q.Cursor%len(list)].ID
			q.Cursor++
			if err := s.S.PutQueue(ctx, q); err != nil {
				return err
			}
		}
		if r.Queue != "" {
			w.Queue = r.Queue
		}
	}
	if w.Assignee != "" {
		w.Status = store.WorkClaimed
	}
	return s.delegate(ctx, w)
}

// delegate forwards work of an absent assignee to their delegate.
func (s *Service) delegate(ctx context.Context, w *store.Work) error {
	for hop := 0; w.Assignee != "" && hop < 3; hop++ {
		u, err := s.S.GetUser(ctx, w.Assignee)
		if err != nil || !u.Away(s.Now()) || u.OOO.Delegate == "" {
			return nil
		}
		if w.DelegatedFrom == "" {
			w.DelegatedFrom = w.Assignee
		}
		w.Assignee = u.OOO.Delegate
	}
	return nil
}

// ---------------------------------------------------------------- calendar

type businessCal struct {
	c   store.Calendar
	loc *time.Location
}

func (s *Service) calendar(ctx context.Context) businessCal {
	st, err := s.S.GetSettings(ctx)
	bc := businessCal{loc: time.Local}
	if err != nil || st == nil {
		return bc
	}
	bc.c = st.Calendar
	if st.Calendar.TZ != "" {
		if l, err := time.LoadLocation(st.Calendar.TZ); err == nil {
			bc.loc = l
		}
	}
	return bc
}

func hm(s string, def int) int {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return def
	}
	return h*60 + m
}

// Add returns start + d counted in working time (calendar on) or wall time.
// Durations of whole days (P1D) mean working days of the calendar's length.
func (b businessCal) Add(start time.Time, d time.Duration) time.Time {
	if !b.c.Enabled || len(b.c.Days) == 0 {
		return start.Add(d)
	}
	open, close := hm(b.c.Start, 8*60+30), hm(b.c.End, 16*60+30)
	dayLen := time.Duration(close-open) * time.Minute
	if dayLen <= 0 {
		return start.Add(d)
	}
	// translate calendar days into working days
	days := d / (24 * time.Hour)
	rest := d % (24 * time.Hour)
	remaining := time.Duration(days)*dayLen + rest
	t := start.In(b.loc)
	holidays := map[string]bool{}
	for _, h := range b.c.Holidays {
		holidays[h] = true
	}
	working := func(t time.Time) bool {
		return contains(intsToStr(b.c.Days), fmt.Sprint(int(t.Weekday()))) && !holidays[t.Format("2006-01-02")]
	}
	for i := 0; i < 3660 && remaining > 0; i++ {
		dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, b.loc).Add(time.Duration(open) * time.Minute)
		dayEnd := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, b.loc).Add(time.Duration(close) * time.Minute)
		if !working(t) || !t.Before(dayEnd) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, b.loc).Add(time.Duration(open) * time.Minute)
			continue
		}
		if t.Before(dayStart) {
			t = dayStart
		}
		avail := dayEnd.Sub(t)
		if remaining <= avail {
			return t.Add(remaining)
		}
		remaining -= avail
		t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, b.loc).Add(time.Duration(open) * time.Minute)
	}
	return t
}

func intsToStr(v []int) []string {
	out := make([]string, len(v))
	for i, x := range v {
		out[i] = fmt.Sprint(x)
	}
	return out
}

// ---------------------------------------------------------------- SLA

// RunSLA fires goal / deadline escalations of overdue work.
func (s *Service) RunSLA(ctx context.Context) (int, error) {
	due, err := s.S.SLADue(ctx, s.Now(), 100)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, w := range due {
		changed, err := s.escalate(ctx, w)
		if err != nil {
			return n, err
		}
		if changed {
			n++
		}
	}
	return n, nil
}

func (s *Service) escalate(ctx context.Context, w *store.Work) (bool, error) {
	now := s.Now()
	c, err := s.S.GetCase(ctx, w.CaseID)
	if err != nil {
		return false, err
	}
	binds, _ := s.S.GetBindings(ctx, c.VersionID)
	var rules []store.Escalation
	for _, b := range binds {
		if b.NodeID == w.Node {
			rules = b.Escalate
		}
	}
	run := func(at string) {
		ran := false
		for _, e := range rules {
			if e.At != at {
				continue
			}
			ran = true
			s.applyEscalation(ctx, w, e)
		}
		if !ran { // defaults: goal -> remind assignee; deadline -> remind assignee and queue supervisors, raise urgency
			if at == "goal" {
				s.applyEscalation(ctx, w, store.Escalation{At: at, Do: "urgency", Amount: 20})
				s.applyEscalation(ctx, w, store.Escalation{At: at, Do: "notify", Target: "assignee"})
			} else {
				s.applyEscalation(ctx, w, store.Escalation{At: at, Do: "urgency", Amount: 30})
				s.applyEscalation(ctx, w, store.Escalation{At: at, Do: "notify", Target: "assignee"})
				s.applyEscalation(ctx, w, store.Escalation{At: at, Do: "notify", Target: "supervisors"})
			}
		}
		_ = s.S.AppendEvents(ctx, []store.EventRec{{CaseID: w.CaseID, At: now, Type: "sla." + at, Node: w.Node, Data: map[string]any{"work": w.ID}}})
	}
	changed := false
	if w.GoalAt != nil && !w.GoalDone && !w.GoalAt.After(now) {
		run("goal")
		w.GoalDone, changed = true, true
	}
	if w.DueAt != nil && !w.DeadlineDone && !w.DueAt.After(now) {
		run("deadline")
		w.DeadlineDone, changed = true, true
	}
	if !changed {
		return false, nil
	}
	return true, s.S.PutWork(ctx, w)
}

func (s *Service) applyEscalation(ctx context.Context, w *store.Work, e store.Escalation) {
	title := fmt.Sprintf("%s · %s", w.CaseNumber, w.Name)
	kind := map[string]string{"goal": "goal passed", "deadline": "overdue"}[e.At]
	switch e.Do {
	case "urgency":
		w.Urgency += e.Amount
		if w.Urgency > 100 {
			w.Urgency = 100
		}
	case "notify":
		for _, u := range s.escalationTargets(ctx, w, e.Target) {
			s.notify(ctx, u, "escalated", title+" — "+kind, "/work/"+w.ID)
		}
	case "reassign":
		targets := s.escalationTargets(ctx, w, e.Target)
		if len(targets) > 0 {
			w.Assignee, w.Status = targets[0], store.WorkClaimed
			s.notify(ctx, targets[0], "assigned", title+" — "+kind, "/work/"+w.ID)
		}
	case "queue":
		w.Assignee, w.Status = "", store.WorkOpen
		w.Role, w.Queue = queuePrefix+e.Target, e.Target
	}
}

func (s *Service) escalationTargets(ctx context.Context, w *store.Work, target string) []string {
	switch target {
	case "", "assignee":
		if w.Assignee != "" {
			return []string{w.Assignee}
		}
		var out []string
		for _, u := range s.candidates(ctx, w.Queue, strings.TrimPrefix(w.Role, queuePrefix), "") {
			out = append(out, u.ID)
		}
		return out
	case "supervisors":
		if w.Queue != "" {
			if q, err := s.S.GetQueue(ctx, w.Queue); err == nil && len(q.Supervisors) > 0 {
				return q.Supervisors
			}
		}
		var out []string
		for _, u := range s.candidates(ctx, "", RoleSupervisor, "") {
			out = append(out, u.ID)
		}
		return out
	default:
		return []string{target}
	}
}

// ---------------------------------------------------------------- next / bulk

func sortWork(ws []*store.Work) {
	sort.SliceStable(ws, func(i, j int) bool {
		if ws[i].Urgency != ws[j].Urgency {
			return ws[i].Urgency > ws[j].Urgency
		}
		di, dj := ws[i].DueAt, ws[j].DueAt
		switch {
		case di != nil && dj != nil:
			return di.Before(*dj)
		case di != nil:
			return true
		case dj != nil:
			return false
		}
		return ws[i].CreatedAt.Before(ws[j].CreatedAt)
	})
}

// NextWork claims the most urgent unassigned item from the caller's queues.
func (s *Service) NextWork(ctx context.Context, by Actor) (*store.Work, error) {
	ws, err := s.Inbox(ctx, by, false)
	if err != nil {
		return nil, err
	}
	for _, w := range ws {
		if w.Assignee == by.ID {
			return w, nil // finish what you hold first
		}
	}
	for _, w := range ws {
		if w.Assignee != "" {
			continue
		}
		if got, err := s.Claim(ctx, by, w.ID); err == nil {
			return got, nil
		}
	}
	return nil, store.ErrNotFound
}

// Bulk applies one operation to many work items (supervisor):
// reassign (to user), transfer (to queue or role), release (back to queue), urgency (+/-).
func (s *Service) Bulk(ctx context.Context, by Actor, ids []string, op, target string, amount int) (int, error) {
	if !by.Has(RoleSupervisor) {
		return 0, ErrForbidden
	}
	n := 0
	for _, wid := range ids {
		err := s.S.Tx(ctx, func(ctx context.Context) error {
			w, err := s.S.GetWork(ctx, wid)
			if err != nil || (w.Status != store.WorkOpen && w.Status != store.WorkClaimed) {
				return err
			}
			switch op {
			case "reassign":
				w.Assignee, w.Status = target, store.WorkClaimed
				_ = s.delegate(ctx, w)
				s.notify(ctx, w.Assignee, "assigned", w.CaseNumber+" · "+w.Name, "/work/"+w.ID)
			case "transfer":
				w.Assignee, w.Status = "", store.WorkOpen
				if q, err := s.S.GetQueue(ctx, target); err == nil {
					w.Role, w.Queue = queuePrefix+q.Key, q.Key
				} else {
					w.Role, w.Queue = target, ""
				}
			case "release":
				w.Assignee, w.Status = "", store.WorkOpen
			case "urgency":
				w.Urgency += amount
				if w.Urgency < 0 {
					w.Urgency = 0
				}
				if w.Urgency > 100 {
					w.Urgency = 100
				}
			default:
				return invalid("unknown bulk operation %q", op)
			}
			s.audit(ctx, by, "work.bulk."+op, "case", w.CaseID, map[string]any{"work": w.ID, "target": target})
			n++
			return s.S.PutWork(ctx, w)
		})
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// ---------------------------------------------------------------- notifications

func (s *Service) notify(ctx context.Context, user, kind, title, link string) {
	if user == "" {
		return
	}
	n := &store.Notification{ID: id.New(), User: user, At: s.Now(), Kind: kind, Title: title, Link: link}
	_ = s.S.AddNotification(ctx, n)
	// e-mail goes through the outbox; the notify service delivers it with retries
	if u, err := s.S.GetUser(ctx, user); err == nil && u.Email != "" {
		now := s.Now()
		_ = s.S.PutOutbox(ctx, &store.OutboxMsg{ID: id.New(), Kind: "email", To: u.Email, Subject: title,
			Body: title + "\r\n" + os.Getenv("PUBLIC_WEB_URL") + link + "\r\n", Status: "pending", NextRunAt: now, CreatedAt: now})
	}
}

// Mailer sends one e-mail. The default uses SMTP_* settings; tests replace it.
var Mailer = func(to, subject, body string) error {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return errNoMailer
	}
	from := firstNonEmpty(os.Getenv("SMTP_FROM"), "case-management@localhost")
	port := firstNonEmpty(os.Getenv("SMTP_PORT"), "587")
	var auth smtp.Auth
	if u := os.Getenv("SMTP_USER"); u != "" {
		auth = smtp.PlainAuth("", u, os.Getenv("SMTP_PASSWORD"), host)
	}
	msg := "From: " + from + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + body
	return smtp.SendMail(host+":"+port, auth, from, []string{to}, []byte(msg))
}

var errNoMailer = errors.New("SMTP_HOST not set")

const maxMailAttempts = 8

// RunOutbox delivers pending e-mail. Failures retry with backoff; after
// maxMailAttempts the message stays as failed for an operator to look at.
// Without SMTP configured, messages are dropped (in-app notifications remain).
func (s *Service) RunOutbox(ctx context.Context, limit int) (int, error) {
	msgs, err := s.S.LeaseOutbox(ctx, s.Now(), 2*time.Minute, limit)
	if err != nil {
		return 0, err
	}
	for _, m := range msgs {
		err := Mailer(m.To, m.Subject, m.Body)
		switch {
		case err == nil, errors.Is(err, errNoMailer):
			Metrics.MailSent.Add(1)
			if err := s.S.DeleteOutbox(ctx, m.ID); err != nil {
				return 0, err
			}
		default:
			m.Attempts++
			m.LastError, m.LeaseUntil = err.Error(), nil
			if m.Attempts >= maxMailAttempts {
				m.Status = "failed"
				Metrics.MailFailed.Add(1)
			} else {
				m.Status, m.NextRunAt = "pending", s.Now().Add(backoff(m.Attempts))
			}
			if err := s.S.PutOutbox(ctx, m); err != nil {
				return 0, err
			}
		}
	}
	return len(msgs), nil
}

// Profile / settings helpers used by the API.

func (s *Service) SaveProfile(ctx context.Context, by Actor, u *store.UserProfile) (*store.UserProfile, error) {
	if u.ID != by.ID && !by.Has(RoleAdmin) && !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	cur, err := s.S.GetUser(ctx, u.ID)
	if err != nil {
		cur = &store.UserProfile{ID: u.ID, Active: true}
	}
	if u.ID == by.ID && !by.Has(RoleAdmin) && !by.Has(RoleSupervisor) {
		cur.OOO, cur.Email = u.OOO, u.Email // users manage their own absence and e-mail
	} else {
		cur.Name, cur.Email, cur.Skills, cur.Capacity, cur.Active, cur.OOO = firstNonEmpty(u.Name, cur.Name), u.Email, u.Skills, u.Capacity, u.Active, u.OOO
	}
	if cur.OOO != nil && cur.OOO.Delegate == cur.ID {
		return nil, invalid("cannot delegate to yourself")
	}
	s.audit(ctx, by, "user.update", "user", u.ID, nil)
	return cur, s.S.PutUser(ctx, cur)
}

func (s *Service) SaveQueue(ctx context.Context, by Actor, q *store.Queue) error {
	if !by.Has(RoleAdmin) && !by.Has(RoleSupervisor) {
		return ErrForbidden
	}
	if !keyPattern.MatchString(q.Key) || q.Name == "" {
		return invalid("queue key (lowercase) and name are required")
	}
	if old, err := s.S.GetQueue(ctx, q.Key); err == nil {
		q.Cursor = old.Cursor
	}
	s.audit(ctx, by, "queue.save", "queue", q.Key, nil)
	return s.S.PutQueue(ctx, q)
}

func (s *Service) SaveSettings(ctx context.Context, by Actor, st *store.Settings) error {
	if !by.Has(RoleAdmin) {
		return ErrForbidden
	}
	s.audit(ctx, by, "settings.update", "settings", "main", nil)
	return s.S.PutSettings(ctx, st)
}

// WorkableBy lists the ids of open work items the caller may do now
// (assigned to them, or unassigned in one of their roles or queues).
func (s *Service) WorkableBy(ctx context.Context, by Actor, ws []*store.Work) []string {
	roles := s.effectiveRoles(ctx, by)
	out := []string{}
	for _, w := range ws {
		if w.Status != store.WorkOpen && w.Status != store.WorkClaimed {
			continue
		}
		if w.Assignee == by.ID || (w.Assignee == "" && (w.Role == "" || contains(roles, w.Role))) {
			out = append(out, w.ID)
		}
	}
	return out
}

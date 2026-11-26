package store

import (
	"context"
	"time"
)

// UserProfile is what work management knows about a person. Users are
// registered automatically when they first call the API (identity and roles
// come from the IAM); skills, capacity and out-of-office are kept here.
type UserProfile struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Email    string       `json:"email,omitempty"`
	Roles    []string     `json:"roles"`
	Skills   []string     `json:"skills,omitempty"`
	Capacity int          `json:"capacity,omitempty"` // max open items (0 = unlimited)
	Active   bool         `json:"active"`
	OOO      *OutOfOffice `json:"ooo,omitempty"`
	LastSeen time.Time    `json:"lastSeen"`
}

type OutOfOffice struct {
	From     *time.Time `json:"from,omitempty"`
	To       *time.Time `json:"to,omitempty"`
	Delegate string     `json:"delegate,omitempty"` // receives new work meanwhile
}

func (u *UserProfile) Away(now time.Time) bool {
	if u.OOO == nil {
		return false
	}
	if u.OOO.From != nil && now.Before(*u.OOO.From) {
		return false
	}
	if u.OOO.To != nil && now.After(*u.OOO.To) {
		return false
	}
	return true
}

// Queue is a shared work basket: members are users with one of its roles
// and/or listed explicitly.
type Queue struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles,omitempty"`
	Members     []string `json:"members,omitempty"`
	Supervisors []string `json:"supervisors,omitempty"`
	Cursor      int      `json:"cursor,omitempty"` // round-robin position
}

// Calendar defines business time for SLAs.
type Calendar struct {
	Enabled  bool     `json:"enabled"`
	Days     []int    `json:"days"`               // working weekdays, 0 = Sunday
	Start    string   `json:"start"`              // "08:30"
	End      string   `json:"end"`                // "16:30"
	Holidays []string `json:"holidays,omitempty"` // "2026-12-05"
	TZ       string   `json:"tz,omitempty"`       // IANA name, default server local
}

type Settings struct {
	Calendar Calendar `json:"calendar"`
}

type Notification struct {
	ID    string    `json:"id"`
	User  string    `json:"user"`
	At    time.Time `json:"at"`
	Kind  string    `json:"kind"` // assigned | escalated | mention | info
	Title string    `json:"title"`
	Body  string    `json:"body,omitempty"`
	Link  string    `json:"link,omitempty"`
	Read  bool      `json:"read"`
}

// Routing decides who gets a step's work item.
type Routing struct {
	Mode  string `json:"mode,omitempty"`  // role (default) | queue | user | workload | roundrobin | skill | expression
	Queue string `json:"queue,omitempty"` // queue key for queue / workload / roundrobin / skill
	User  string `json:"user,omitempty"`  // fixed user or ${expression}
	Skill string `json:"skill,omitempty"`
	Expr  string `json:"expr,omitempty"` // expression returning a user id
}

// Escalation runs when a work item passes its goal or deadline.
type Escalation struct {
	At     string `json:"at"`               // goal | deadline
	Do     string `json:"do"`               // notify | urgency | reassign | queue
	Target string `json:"target,omitempty"` // user / queue / "supervisors" / "assignee"
	Amount int    `json:"amount,omitempty"` // urgency points
}

type WorkOps interface {
	PutUser(ctx context.Context, u *UserProfile) error
	GetUser(ctx context.Context, id string) (*UserProfile, error)
	ListUsers(ctx context.Context) ([]*UserProfile, error)

	PutQueue(ctx context.Context, q *Queue) error
	GetQueue(ctx context.Context, key string) (*Queue, error)
	ListQueues(ctx context.Context) ([]*Queue, error)
	DeleteQueue(ctx context.Context, key string) error

	GetSettings(ctx context.Context) (*Settings, error)
	PutSettings(ctx context.Context, s *Settings) error

	AddNotification(ctx context.Context, n *Notification) error
	ListNotifications(ctx context.Context, user string, limit int) ([]*Notification, error)
	MarkNotificationsRead(ctx context.Context, user string, ids []string) error

	// SLADue returns open work whose goal or deadline passed and was not yet handled.
	SLADue(ctx context.Context, now time.Time, limit int) ([]*Work, error)
	// OpenCounts returns open/claimed item counts per assignee.
	OpenCounts(ctx context.Context) (map[string]int, error)
}

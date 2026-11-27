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
	Theme    *Theme   `json:"theme,omitempty"`
}

// Theme is how the system looks for one organisation (tenant): name, logo,
// colours, type, shape and density. Applied to every screen and the portal.
type Theme struct {
	Product  string `json:"product,omitempty"` // system name in the header, e.g. "ระบบติดตามเรื่อง"
	Org      string `json:"org,omitempty"`     // organisation name
	Logo     string `json:"logo,omitempty"`    // data: URL (png, jpeg or svg), small
	Accent   string `json:"accent,omitempty"`  // #rrggbb buttons, links, selection
	Header   string `json:"header,omitempty"`  // #rrggbb header bar
	Font     string `json:"font,omitempty"`    // Google font family name
	BaseSize int    `json:"baseSize,omitempty"`
	Radius   int    `json:"radius,omitempty"`  // corner radius in px (0 = Carbon square)
	Density  string `json:"density,omitempty"` // compact | normal | comfortable
	Mode     string `json:"mode,omitempty"`    // light | dark | system
	Footer   string `json:"footer,omitempty"`
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

// OutboxMsg is an outgoing message (e-mail) delivered by the notify service
// with retries, so a slow or unavailable mail server never delays case work.
type OutboxMsg struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`           // email | webhook | event
	Hook       string     `json:"hook,omitempty"` // webhook id (kind webhook)
	To         string     `json:"to"`
	Subject    string     `json:"subject"`
	Body       string     `json:"body"`
	Status     string     `json:"status"` // pending | sending | failed
	Attempts   int        `json:"attempts"`
	NextRunAt  time.Time  `json:"nextRunAt"`
	LeaseUntil *time.Time `json:"leaseUntil,omitempty"`
	LastError  string     `json:"lastError,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// WorkView is a saved way of looking at the work list: columns, filters,
// grouping, sorting and highlight rules. Personal, or shared with roles.
type WorkView struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Owner     string          `json:"owner"`
	Shared    bool            `json:"shared,omitempty"`
	Roles     []string        `json:"roles,omitempty"` // shared with (empty = everyone)
	Default   bool            `json:"default,omitempty"`
	Scope     string          `json:"scope,omitempty"` // mine | queue | all (empty = all I can see)
	Columns   []string        `json:"columns"`         // built-in column keys or data.<field>
	Filters   []ViewRule      `json:"filters,omitempty"`
	GroupBy   string          `json:"groupBy,omitempty"`
	SortBy    string          `json:"sortBy,omitempty"`
	SortDesc  bool            `json:"sortDesc,omitempty"`
	Density   string          `json:"density,omitempty"` // normal | compact
	Highlight []ViewHighlight `json:"highlight,omitempty"`
	OpenIn    string          `json:"openIn,omitempty"` // page (default) | panel
	UpdatedAt time.Time       `json:"updatedAt"`
}

type ViewRule struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value,omitempty"`
}

type ViewHighlight struct {
	When  []ViewRule `json:"when"`
	Color string     `json:"color"` // red | amber | blue | green
}

// Webhook sends case events to another system. The signing secret is never
// stored: Secret names an environment variable CM_SECRET_<NAME>.
type Webhook struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	URL         string     `json:"url"`
	Events      []string   `json:"events"`                // e.g. case.completed; "*" = all
	ProcessKeys []string   `json:"processKeys,omitempty"` // only these case types (empty = all)
	Secret      string     `json:"secret,omitempty"`      // secret name for the HMAC signature
	IncludeData bool       `json:"includeData,omitempty"` // add case data to the payload
	Active      bool       `json:"active"`
	LastAt      *time.Time `json:"lastAt,omitempty"`
	LastStatus  string     `json:"lastStatus,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// AssignRule is an organisation-wide work distribution rule. Rules are
// checked in order and the first that matches decides: who gets the work,
// a cap per person, extra urgency and SLA. Built in the admin screen.
type AssignRule struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Order     int          `json:"order"`
	Active    bool         `json:"active"`
	When      []ViewRule   `json:"when"`          // fields: caseType, step, stage, role, urgency, data.<field>
	Any       bool         `json:"any,omitempty"` // OR instead of AND
	Then      AssignAction `json:"then"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

type AssignAction struct {
	Mode         string `json:"mode"` // keep | role | queue | user | workload | roundrobin | skill
	Queue        string `json:"queue,omitempty"`
	Role         string `json:"role,omitempty"`
	User         string `json:"user,omitempty"`
	Skill        string `json:"skill,omitempty"`
	MaxPerPerson int    `json:"maxPerPerson,omitempty"` // skip people with this many open items
	Urgency      int    `json:"urgency,omitempty"`      // added to the item's urgency
	SLA          string `json:"sla,omitempty"`          // replaces the step's deadline, ISO-8601 duration
	Goal         string `json:"goal,omitempty"`
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

	PutAssignRule(ctx context.Context, r *AssignRule) error
	ListAssignRules(ctx context.Context) ([]*AssignRule, error)
	DeleteAssignRule(ctx context.Context, id string) error

	PutWebhook(ctx context.Context, h *Webhook) error
	ListWebhooks(ctx context.Context) ([]*Webhook, error)
	DeleteWebhook(ctx context.Context, id string) error
	ListOutbox(ctx context.Context, kind, status string, limit int) ([]*OutboxMsg, error)

	PutView(ctx context.Context, v *WorkView) error
	ListViews(ctx context.Context) ([]*WorkView, error)
	DeleteView(ctx context.Context, id string) error

	PutOutbox(ctx context.Context, m *OutboxMsg) error
	// LeaseOutbox claims due messages (or ones whose sender died) until now+lease.
	LeaseOutbox(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]*OutboxMsg, error)
	DeleteOutbox(ctx context.Context, id string) error

	// SLADue returns open work whose goal or deadline passed and was not yet handled.
	SLADue(ctx context.Context, now time.Time, limit int) ([]*Work, error)
	// OpenCounts returns open/claimed item counts per assignee.
	OpenCounts(ctx context.Context) (map[string]int, error)
}

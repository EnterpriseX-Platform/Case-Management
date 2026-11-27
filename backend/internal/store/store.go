// Package store persists definitions, versions, low-code artefacts, cases,
// work items and timers. Two implementations: Memory (dev/tests) and Postgres.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrStale is returned by PutCase when the case changed since it was read.
	ErrStale = errors.New("case was changed by someone else")
)

type VersionStatus string

const (
	Draft      VersionStatus = "draft"
	InReview   VersionStatus = "in_review"
	Approved   VersionStatus = "approved"
	Published  VersionStatus = "published"
	Deprecated VersionStatus = "deprecated"
	Archived   VersionStatus = "archived"
)

// ProcessDef is a case type, e.g. "purchase-request" or "leave-request".
type ProcessDef struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Category    string    `json:"category,omitempty"`
	ParentKey   string    `json:"parentKey,omitempty"` // case type inheritance
	CreatedAt   time.Time `json:"createdAt"`
}

type Version struct {
	ID            string        `json:"id"`
	ProcessKey    string        `json:"processKey"`
	No            int           `json:"no"`
	Status        VersionStatus `json:"status"`
	FiscalYear    int           `json:"fiscalYear,omitempty"`
	EffectiveFrom *time.Time    `json:"effectiveFrom,omitempty"`
	BPMN          string        `json:"bpmn"`
	Checksum      string        `json:"checksum"`
	Note          string        `json:"note,omitempty"`
	ClonedFrom    string        `json:"clonedFrom,omitempty"`
	Model         *CaseModel    `json:"model,omitempty"`
	CreatedBy     string        `json:"createdBy"`
	ApprovedBy    string        `json:"approvedBy,omitempty"`
	PublishedAt   *time.Time    `json:"publishedAt,omitempty"`
	CreatedAt     time.Time     `json:"createdAt"`
}

// Binding maps a flow step (BPMN node) to how it is presented and who does it.
// It overrides cm:* attributes in the BPMN, so screens can be re-mapped
// without redrawing the flow.
type Binding struct {
	VersionID string        `json:"versionId"`
	NodeID    string        `json:"nodeId"`
	UI        engine.UIMode `json:"ui"`                  // form | external | none
	FormKey   string        `json:"formKey,omitempty"`   // low-code form
	ScreenKey string        `json:"screenKey,omitempty"` // low-code page (layout around the form)
	URL       string        `json:"url,omitempty"`       // external screen
	Embed     bool          `json:"embed,omitempty"`     // iframe vs redirect
	Role      string        `json:"role,omitempty"`
	SLA       string        `json:"sla,omitempty"`
	Enabled   *bool         `json:"enabled,omitempty"` // step toggle
	Condition string        `json:"condition,omitempty"`
	Route     *Routing      `json:"route,omitempty"`
	Goal      string        `json:"goal,omitempty"`    // goal duration (warning before the SLA deadline)
	Urgency   int           `json:"urgency,omitempty"` // base urgency 0-100
	Escalate  []Escalation  `json:"escalate,omitempty"`
}

// Form is a low-code form definition (JSON schema + layout).
type Form struct {
	ID        string         `json:"id"`
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	Version   int            `json:"version"`
	Schema    map[string]any `json:"schema"` // {fields:[{key,label,type,required,options,...}], layout:...}
	UpdatedAt time.Time      `json:"updatedAt"`
}

// Screen is a low-code page: a set of widgets (form, case header, history,
// attachments, related cases, iframe ...) arranged for a role.
type Screen struct {
	ID        string         `json:"id"`
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"` // task | case | portal | dashboard
	Roles     []string       `json:"roles,omitempty"`
	Layout    map[string]any `json:"layout"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

type Case struct {
	ID           string           `json:"id"`
	Number       string           `json:"number"`
	ProcessKey   string           `json:"processKey"`
	VersionID    string           `json:"versionId"`
	Title        string           `json:"title"`
	FiscalYear   int              `json:"fiscalYear,omitempty"`
	ParentID     string           `json:"parentId,omitempty"`
	RelatedIDs   []string         `json:"relatedIds,omitempty"`
	Status       string           `json:"status"`
	Instance     *engine.Instance `json:"instance"`
	Revision     int              `json:"revision"`       // optimistic lock, bumped by every PutCase
	Test         bool             `json:"test,omitempty"` // studio test run, hidden from work lists and reports
	Stage        string           `json:"stage,omitempty"`
	StageHistory []StageEntry     `json:"stageHistory,omitempty"`
	Incidents    int              `json:"incidents,omitempty"`  // open incidents (cached count)
	Subscribed   bool             `json:"subscribed,omitempty"` // has rows in the subscription index
	Closed       bool             `json:"closed,omitempty"`     // completion/cancellation was announced
	CreatedBy    string           `json:"createdBy"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

type WorkStatus string

const (
	WorkOpen      WorkStatus = "open"
	WorkClaimed   WorkStatus = "claimed"
	WorkDone      WorkStatus = "done"
	WorkCancelled WorkStatus = "cancelled"
)

type Work struct {
	engine.WorkItem
	Queue         string     `json:"queue,omitempty"`
	Urgency       int        `json:"urgency"`
	GoalAt        *time.Time `json:"goalAt,omitempty"`
	GoalDone      bool       `json:"goalDone,omitempty"`     // goal escalations ran
	DeadlineDone  bool       `json:"deadlineDone,omitempty"` // deadline escalations ran
	DelegatedFrom string     `json:"delegatedFrom,omitempty"`
	CaseID        string     `json:"caseId"`
	CaseNumber    string     `json:"caseNumber"`
	CaseTitle     string     `json:"caseTitle"`
	ScreenKey     string     `json:"screenKey,omitempty"`
	Test          bool       `json:"test,omitempty"`
	Embed         bool       `json:"embed,omitempty"`
	Status        WorkStatus `json:"status"`
	CreatedAt     time.Time  `json:"createdAt"`
	CompletedBy   string     `json:"completedBy,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

type WorkFilter struct {
	Roles    []string
	Assignee string
	CaseID   string
	Status   []WorkStatus
	Limit    int // 0 = no limit
}

type Audit struct {
	At     time.Time      `json:"at"`
	Actor  string         `json:"actor"`
	Action string         `json:"action"`
	Entity string         `json:"entity"`
	Ref    string         `json:"ref"`
	Data   map[string]any `json:"data,omitempty"`
}

type ImportJob struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	FileName  string         `json:"fileName"`
	Status    string         `json:"status"` // preview | committed | failed
	Summary   map[string]any `json:"summary"`
	Issues    []string       `json:"issues,omitempty"`
	CreatedBy string         `json:"createdBy"`
	CreatedAt time.Time      `json:"createdAt"`
}

type Store interface {
	PutProcess(ctx context.Context, p *ProcessDef) error
	GetProcess(ctx context.Context, key string) (*ProcessDef, error)
	ListProcesses(ctx context.Context) ([]*ProcessDef, error)

	PutVersion(ctx context.Context, v *Version) error
	GetVersion(ctx context.Context, id string) (*Version, error)
	ListVersions(ctx context.Context, processKey string) ([]*Version, error)

	PutBindings(ctx context.Context, versionID string, b []Binding) error
	GetBindings(ctx context.Context, versionID string) ([]Binding, error)

	PutForm(ctx context.Context, f *Form) error
	GetForm(ctx context.Context, key string) (*Form, error)
	ListForms(ctx context.Context) ([]*Form, error)

	PutScreen(ctx context.Context, s *Screen) error
	GetScreen(ctx context.Context, key string) (*Screen, error)
	ListScreens(ctx context.Context) ([]*Screen, error)

	PutCase(ctx context.Context, c *Case) error
	GetCase(ctx context.Context, id string) (*Case, error)
	ListCases(ctx context.Context, processKey string, versionID string, limit int) ([]*Case, error)
	NextCaseNumber(ctx context.Context, prefix string) (int, error)

	PutWork(ctx context.Context, w *Work) error
	GetWork(ctx context.Context, id string) (*Work, error)
	ListWork(ctx context.Context, f WorkFilter) ([]*Work, error)

	PutTimer(ctx context.Context, t *engine.Timer) error
	DeleteTimersByToken(ctx context.Context, instanceID, tokenID string) error
	DueTimers(ctx context.Context, now time.Time, limit int) ([]*engine.Timer, error)
	ListTimers(ctx context.Context, instanceID string) ([]*engine.Timer, error)
	DeleteTimer(ctx context.Context, id string) error

	AddAudit(ctx context.Context, a *Audit) error
	ListAudit(ctx context.Context, ref string, limit int) ([]*Audit, error)

	PutImport(ctx context.Context, j *ImportJob) error
	ListImports(ctx context.Context) ([]*ImportJob, error)

	OpsStore
	WorkOps

	// Tx runs fn atomically (Memory: global lock; Postgres: transaction).
	Tx(ctx context.Context, fn func(ctx context.Context) error) error
}

package store

import (
	"context"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
)

type JobStatus string

const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobDead    JobStatus = "dead" // gave up; an incident was raised
)

// Job queues let each kind of background work scale on its own:
// engine jobs are fast in-process evaluations (decision tables), integration
// jobs call other systems and may be slow.
const (
	QueueEngine      = "engine"
	QueueIntegration = "integration"
)

// JobRec is a unit of external work (connector / rule call) with retries.
type JobRec struct {
	ID    string `json:"id"`
	Queue string `json:"queue,omitempty"` // engine | integration (empty = integration)
	engine.Job
	CaseID      string     `json:"caseId"`
	Status      JobStatus  `json:"status"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"maxAttempts"`
	NextRunAt   time.Time  `json:"nextRunAt"`
	LeaseUntil  *time.Time `json:"leaseUntil,omitempty"`
	LeasedBy    string     `json:"leasedBy,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type IncidentStatus string

const (
	IncidentOpen     IncidentStatus = "open"
	IncidentResolved IncidentStatus = "resolved"
)

type IncidentRec struct {
	ID         string         `json:"id"`
	CaseID     string         `json:"caseId"`
	CaseNumber string         `json:"caseNumber"`
	ProcessKey string         `json:"processKey"`
	TokenID    string         `json:"tokenId"`
	Node       string         `json:"node"`
	NodeName   string         `json:"nodeName"`
	Message    string         `json:"message"`
	Status     IncidentStatus `json:"status"`
	CreatedAt  time.Time      `json:"createdAt"`
	ResolvedAt *time.Time     `json:"resolvedAt,omitempty"`
	ResolvedBy string         `json:"resolvedBy,omitempty"`
	Resolution string         `json:"resolution,omitempty"` // retried | moved | cancelled
}

// EventRec is one entry of a case's append-only event log.
type EventRec struct {
	Seq    int64          `json:"seq"`
	CaseID string         `json:"caseId"`
	At     time.Time      `json:"at"`
	Type   string         `json:"type"` // enter | leave | cancel | error | migrate | incident ...
	Node   string         `json:"node,omitempty"`
	Token  string         `json:"token,omitempty"`
	By     string         `json:"by,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
}

// Subscription indexes what a case waits for, so messages and signals are
// routed without scanning every case.
type Subscription struct {
	CaseID  string `json:"caseId"`
	TokenID string `json:"tokenId"`
	Kind    string `json:"kind"` // message | signal
	Name    string `json:"name"`
}

type OpsStore interface {
	PutJob(ctx context.Context, j *JobRec) error
	GetJob(ctx context.Context, id string) (*JobRec, error)
	// LeaseJobs atomically claims due jobs for a worker until now+lease.
	// queues limits the claim to those queues (none = every queue).
	LeaseJobs(ctx context.Context, worker string, queues []string, now time.Time, lease time.Duration, limit int) ([]*JobRec, error)
	DeleteJob(ctx context.Context, id string) error
	DeleteJobsByToken(ctx context.Context, instanceID, tokenID string) error
	ListJobs(ctx context.Context, caseID string) ([]*JobRec, error)

	PutIncident(ctx context.Context, i *IncidentRec) error
	GetIncident(ctx context.Context, id string) (*IncidentRec, error)
	ListIncidents(ctx context.Context, status IncidentStatus, caseID string) ([]*IncidentRec, error)

	AppendEvents(ctx context.Context, ev []EventRec) error
	ListEvents(ctx context.Context, caseID string, limit int) ([]*EventRec, error)

	ReplaceSubscriptions(ctx context.Context, caseID string, subs []Subscription) error
	FindSubscriptions(ctx context.Context, kind, name string) ([]Subscription, error)
}

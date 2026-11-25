// Package engine executes BPMN processes with token semantics.
//
// The engine is deterministic and storage-agnostic: every call takes an
// Instance, mutates it, and returns Effects (work items to create, timers to
// schedule, child cases to start ...). The caller persists the instance and
// applies the effects inside one transaction.
package engine

import "time"

type InstanceStatus string

const (
	Running   InstanceStatus = "running"
	Completed InstanceStatus = "completed"
	Failed    InstanceStatus = "failed"
	Cancelled InstanceStatus = "cancelled"
)

type WaitKind string

const (
	WaitNone      WaitKind = ""
	WaitUser      WaitKind = "user"      // userTask / manualTask -> work item
	WaitTimer     WaitKind = "timer"     // intermediate timer catch
	WaitMessage   WaitKind = "message"   // receiveTask / message catch
	WaitSignal    WaitKind = "signal"    // signal catch
	WaitCondition WaitKind = "condition" // conditional catch
	WaitJoin      WaitKind = "join"      // parallel / inclusive join
	WaitScope     WaitKind = "scope"     // embedded sub-process container
	WaitMulti     WaitKind = "multi"     // multi-instance container
	WaitCall      WaitKind = "call"      // call activity -> child case
	WaitEventGW   WaitKind = "eventgw"   // event-based gateway
)

type Token struct {
	ID     string         `json:"id"`
	Node   string         `json:"node"`
	Scope  string         `json:"scope,omitempty"` // container token id ("" = root)
	Wait   WaitKind       `json:"wait,omitempty"`
	Locals map[string]any `json:"locals,omitempty"` // multi-instance element vars
	// multi-instance bookkeeping (container tokens only)
	MIItems []any `json:"miItems,omitempty"`
	MINext  int   `json:"miNext,omitempty"`
	MIOpen  int   `json:"miOpen,omitempty"`
	MIInner bool  `json:"miInner,omitempty"`
	// wait details
	Message string `json:"message,omitempty"`
	Signal  string `json:"signal,omitempty"`
	Cond    string `json:"cond,omitempty"`
}

// HistoryEntry records the path a case actually walked (for audit & replay).
type HistoryEntry struct {
	At    time.Time `json:"at"`
	Node  string    `json:"node"`
	Event string    `json:"event"` // enter | leave | cancel | error | migrate
	Token string    `json:"token"`
	By    string    `json:"by,omitempty"`
}

type Instance struct {
	ID          string         `json:"id"`
	ProcessID   string         `json:"processId"`
	VersionID   string         `json:"versionId"`
	Status      InstanceStatus `json:"status"`
	Vars        map[string]any `json:"vars"`
	Tokens      []*Token       `json:"tokens"`
	History     []HistoryEntry `json:"history"`
	ParentID    string         `json:"parentId,omitempty"`
	ParentToken string         `json:"parentToken,omitempty"`
	Error       string         `json:"error,omitempty"`
	Seq         int            `json:"seq"`
}

// UIMode tells the front-end how a step is presented.
//
//	form     -> low-code form rendered by the platform (cm:form = form key)
//	external -> external screen (cm:url), embedded or redirected, completes via callback API
//	none     -> headless; completed through the API only
type UIMode string

const (
	UIForm     UIMode = "form"
	UIExternal UIMode = "external"
	UINone     UIMode = "none"
)

type WorkItem struct {
	ID         string         `json:"id"`
	InstanceID string         `json:"instanceId"`
	TokenID    string         `json:"tokenId"`
	Node       string         `json:"node"`
	Name       string         `json:"name"`
	Role       string         `json:"role,omitempty"`
	Assignee   string         `json:"assignee,omitempty"`
	UI         UIMode         `json:"ui"`
	FormKey    string         `json:"formKey,omitempty"`
	URL        string         `json:"url,omitempty"`
	Input      map[string]any `json:"input,omitempty"`
	SLA        string         `json:"sla,omitempty"` // ISO-8601 duration
	DueAt      *time.Time     `json:"dueAt,omitempty"`
}

type Timer struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instanceId"`
	TokenID    string    `json:"tokenId"`
	Node       string    `json:"node"` // catch event / boundary / gateway target node
	DueAt      time.Time `json:"dueAt"`
}

type StartChild struct {
	ParentID    string         `json:"parentId"`
	ParentToken string         `json:"parentToken"`
	ProcessKey  string         `json:"processKey"`
	Vars        map[string]any `json:"vars"`
}

type Outbound struct {
	Kind string         `json:"kind"` // message | signal
	Name string         `json:"name"`
	Vars map[string]any `json:"vars"`
}

// Effects are side effects the caller must apply after a step.
type Effects struct {
	NewWork       []WorkItem   `json:"newWork,omitempty"`
	CancelWork    []string     `json:"cancelWork,omitempty"` // token ids
	NewTimers     []Timer      `json:"newTimers,omitempty"`
	CancelTimers  []string     `json:"cancelTimers,omitempty"` // token ids
	StartChildren []StartChild `json:"startChildren,omitempty"`
	CancelChild   []string     `json:"cancelChild,omitempty"` // parent token ids
	Outbound      []Outbound   `json:"outbound,omitempty"`
	Completed     bool         `json:"completed,omitempty"`
}

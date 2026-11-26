package store

import "time"

// CaseModel is the case-management layer of a version: stages, typed data
// and user actions (see app/model.go).
type CaseModel struct {
	Stages  []Stage      `json:"stages"`
	Data    []DataField  `json:"data"`
	Rules   []DataRule   `json:"rules,omitempty"`
	Actions []CaseAction `json:"actions"`

	Lifecycle  *Lifecycle      `json:"lifecycle,omitempty"`
	Decisions  []DecisionTable `json:"decisions,omitempty"`
	Connectors []Connector     `json:"connectors,omitempty"`
}

type Stage struct {
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`            // primary | alternate | resolution
	Steps []string `json:"steps"`           // flow node ids in this stage
	Entry string   `json:"entry,omitempty"` // node to start at when the stage is entered by an action
	SLA   string   `json:"sla,omitempty"`   // ISO-8601 duration for the whole stage
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type DataField struct {
	Key        string   `json:"key"`
	Label      string   `json:"label"`
	Type       string   `json:"type"` // text longtext number money date boolean enum list user org reference
	Required   bool     `json:"required,omitempty"`
	RequiredIn []string `json:"requiredIn,omitempty"` // stages in which it must be filled
	Options    []Option `json:"options,omitempty"`
	Calc       string   `json:"calc,omitempty"` // expression for a calculated field
	Searchable bool     `json:"searchable,omitempty"`
	Summary    bool     `json:"summary,omitempty"` // show in case header
	Help       string   `json:"help,omitempty"`
}

// DataRule is a validation across fields, e.g. approved <= requested.
type DataRule struct {
	When    string `json:"when,omitempty"`
	Check   string `json:"check"`
	Message string `json:"message"`
}

type CaseAction struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"`             // update | stage | child
	Target  string   `json:"target,omitempty"` // stage key (stage) or case type (child)
	FormKey string   `json:"formKey,omitempty"`
	Stages  []string `json:"stages,omitempty"` // where available (empty = all)
	Roles   []string `json:"roles,omitempty"`  // who may run it (empty = anyone who can see the case)
	Pass    []string `json:"pass,omitempty"`   // fields copied to a child case
	Confirm bool     `json:"confirm,omitempty"`
}

func (m *CaseModel) Stage(key string) *Stage {
	if m == nil {
		return nil
	}
	for i := range m.Stages {
		if m.Stages[i].Key == key {
			return &m.Stages[i]
		}
	}
	return nil
}

type StageEntry struct {
	Stage string     `json:"stage"`
	At    time.Time  `json:"at"`
	Left  *time.Time `json:"left,omitempty"`
}

// Lifecycle is the business-mode description of a case type from which the
// BPMN flow is generated (see app/lifecycle.go).
type Lifecycle struct {
	Stages   []LCStage `json:"stages"`
	Detached bool      `json:"detached,omitempty"` // BPMN edited by hand; no longer generated
}

type LCStage struct {
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Kind  string   `json:"kind"` // primary | alternate | resolution
	SLA   string   `json:"sla,omitempty"`
	Steps []LCStep `json:"steps"`
}

type LCStep struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Kind          string `json:"kind"` // task | approval | automation | decision | wait | child | timer
	Role          string `json:"role,omitempty"`
	Form          string `json:"form,omitempty"`
	Screen        string `json:"screen,omitempty"`
	URL           string `json:"url,omitempty"`           // external screen
	SLA           string `json:"sla,omitempty"`           // task SLA, or wait time for a timer step
	When          string `json:"when,omitempty"`          // only run when this condition holds
	Multi         string `json:"multi,omitempty"`         // one task per item of this list field
	OnReject      string `json:"onReject,omitempty"`      // approval: "end", a stage key or a step key
	DecisionField string `json:"decisionField,omitempty"` // approval: boolean field holding the decision
	Connector     string `json:"connector,omitempty"`
	Decision      string `json:"decision,omitempty"`
	Message       string `json:"message,omitempty"`
	CaseType      string `json:"caseType,omitempty"`
}

// DecisionTable maps input conditions to outputs (hit policy: first match,
// or collect all matches into lists).
type DecisionTable struct {
	Key     string     `json:"key"`
	Name    string     `json:"name"`
	Inputs  []DTColumn `json:"inputs"`  // expressions over case data, e.g. "amount"
	Outputs []DTColumn `json:"outputs"` // case fields written
	Rows    []DTRow    `json:"rows"`
	Hit     string     `json:"hit,omitempty"` // first (default) | collect
}

type DTColumn struct {
	Key   string `json:"key"`   // input: expression; output: field key
	Label string `json:"label"` // shown as column header
}

type DTRow struct {
	When []string `json:"when"` // one cell per input: "", "> 1000000", "in [\"a\",\"b\"]", "== \"x\"", "1000..5000"
	Then []string `json:"then"` // one cell per output: expression or literal
	Note string   `json:"note,omitempty"`
}

// Connector is a REST call designed in the studio.
type Connector struct {
	Key      string            `json:"key"`
	Name     string            `json:"name"`
	Method   string            `json:"method"`
	URL      string            `json:"url"`                // may contain ${field}
	Headers  map[string]string `json:"headers,omitempty"`  // values may be ${secret:NAME}
	Body     map[string]string `json:"body,omitempty"`     // request JSON key -> expression
	Response map[string]string `json:"response,omitempty"` // case field -> JSON path in the response (a.b.0.c)
	Timeout  int               `json:"timeout,omitempty"`  // seconds
	Retries  int               `json:"retries,omitempty"`
}

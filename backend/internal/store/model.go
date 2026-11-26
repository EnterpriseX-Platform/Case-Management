package store

import "time"

// CaseModel is the case-management layer of a version: stages, typed data
// and user actions (see app/model.go).
type CaseModel struct {
	Stages  []Stage      `json:"stages"`
	Data    []DataField  `json:"data"`
	Rules   []DataRule   `json:"rules,omitempty"`
	Actions []CaseAction `json:"actions"`
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

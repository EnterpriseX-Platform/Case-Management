// Package bpmn parses BPMN 2.0 XML into an executable graph model.
//
// Every BPMN 2.0 element is recognised and preserved (round-trip). Elements
// whose executor is not yet implemented are reported by Supported() so the
// engine can refuse to publish a version that relies on them.
package bpmn

// ElementType is the BPMN 2.0 local element name, e.g. "userTask".
type ElementType string

const (
	StartEvent             ElementType = "startEvent"
	EndEvent               ElementType = "endEvent"
	IntermediateCatchEvent ElementType = "intermediateCatchEvent"
	IntermediateThrowEvent ElementType = "intermediateThrowEvent"
	BoundaryEvent          ElementType = "boundaryEvent"

	Task             ElementType = "task"
	UserTask         ElementType = "userTask"
	ManualTask       ElementType = "manualTask"
	ServiceTask      ElementType = "serviceTask"
	ScriptTask       ElementType = "scriptTask"
	BusinessRuleTask ElementType = "businessRuleTask"
	SendTask         ElementType = "sendTask"
	ReceiveTask      ElementType = "receiveTask"
	SubProcess       ElementType = "subProcess"
	AdHocSubProcess  ElementType = "adHocSubProcess"
	Transaction      ElementType = "transaction"
	CallActivity     ElementType = "callActivity"

	ExclusiveGateway  ElementType = "exclusiveGateway"
	ParallelGateway   ElementType = "parallelGateway"
	InclusiveGateway  ElementType = "inclusiveGateway"
	EventBasedGateway ElementType = "eventBasedGateway"
	ComplexGateway    ElementType = "complexGateway"
)

// EventDefinition kinds (BPMN *EventDefinition elements).
type EventKind string

const (
	EvNone         EventKind = ""
	EvMessage      EventKind = "message"
	EvTimer        EventKind = "timer"
	EvSignal       EventKind = "signal"
	EvConditional  EventKind = "conditional"
	EvError        EventKind = "error"
	EvEscalation   EventKind = "escalation"
	EvCompensation EventKind = "compensate"
	EvCancel       EventKind = "cancel"
	EvTerminate    EventKind = "terminate"
	EvLink         EventKind = "link"
)

// Node is one flow node of a process (or of a nested sub-process).
type Node struct {
	ID       string      `json:"id"`
	Type     ElementType `json:"type"`
	Name     string      `json:"name,omitempty"`
	Incoming []string    `json:"incoming,omitempty"` // sequence flow ids
	Outgoing []string    `json:"outgoing,omitempty"`
	Parent   string      `json:"parent,omitempty"` // enclosing sub-process id, "" = process root

	Event        EventKind `json:"event,omitempty"`
	TimerExpr    string    `json:"timerExpr,omitempty"` // ISO-8601 duration (timeDuration) / date
	MessageRef   string    `json:"messageRef,omitempty"`
	SignalRef    string    `json:"signalRef,omitempty"`
	ErrorCode    string    `json:"errorCode,omitempty"`
	ConditionExp string    `json:"condition,omitempty"`

	AttachedTo     string `json:"attachedTo,omitempty"` // boundary events
	CancelActivity bool   `json:"cancelActivity,omitempty"`

	DefaultFlow string `json:"default,omitempty"` // gateways / activities
	CalledElem  string `json:"calledElement,omitempty"`
	Script      string `json:"script,omitempty"`

	// Multi-instance
	MultiInstance *MultiInstance `json:"multiInstance,omitempty"`

	// Extension attributes in the cm: namespace (form, role, sla, rule, url ...)
	Ext map[string]string `json:"ext,omitempty"`
}

type MultiInstance struct {
	Sequential  bool   `json:"sequential"`
	Collection  string `json:"collection"` // variable holding a list
	ElementVar  string `json:"elementVariable"`
	Cardinality string `json:"cardinality,omitempty"`
}

type Flow struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Name      string `json:"name,omitempty"`
	Condition string `json:"condition,omitempty"`
}

// Lane maps to a role / org unit for automatic assignment.
type Lane struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Nodes []string `json:"nodes"`
	Role  string   `json:"role,omitempty"`
}

type Process struct {
	ID    string           `json:"id"`
	Name  string           `json:"name"`
	Nodes map[string]*Node `json:"nodes"`
	Flows map[string]*Flow `json:"flows"`
	Lanes []Lane           `json:"lanes,omitempty"`
}

func (p *Process) StartNodes(parent string) []*Node {
	var out []*Node
	for _, n := range p.Nodes {
		if n.Type == StartEvent && n.Parent == parent {
			out = append(out, n)
		}
	}
	return out
}

func (p *Process) Boundaries(activity string) []*Node {
	var out []*Node
	for _, n := range p.Nodes {
		if n.Type == BoundaryEvent && n.AttachedTo == activity {
			out = append(out, n)
		}
	}
	return out
}

// LaneOf returns the lane containing node id, if any.
func (p *Process) LaneOf(id string) *Lane {
	for i := range p.Lanes {
		for _, n := range p.Lanes[i].Nodes {
			if n == id {
				return &p.Lanes[i]
			}
		}
	}
	return nil
}

// executable lists element/event combinations the engine can run today.
var executable = map[ElementType]map[EventKind]bool{
	StartEvent:             {EvNone: true, EvMessage: true, EvTimer: true, EvSignal: true},
	EndEvent:               {EvNone: true, EvTerminate: true, EvError: true, EvMessage: true, EvSignal: true, EvEscalation: true},
	IntermediateCatchEvent: {EvTimer: true, EvMessage: true, EvSignal: true, EvLink: true, EvConditional: true},
	IntermediateThrowEvent: {EvNone: true, EvMessage: true, EvSignal: true, EvLink: true, EvEscalation: true},
	BoundaryEvent:          {EvTimer: true, EvError: true, EvMessage: true, EvSignal: true, EvEscalation: true},
	Task:                   {EvNone: true}, UserTask: {EvNone: true}, ManualTask: {EvNone: true},
	ServiceTask: {EvNone: true}, ScriptTask: {EvNone: true}, BusinessRuleTask: {EvNone: true},
	SendTask: {EvNone: true}, ReceiveTask: {EvNone: true},
	SubProcess: {EvNone: true}, CallActivity: {EvNone: true},
	ExclusiveGateway: {EvNone: true}, ParallelGateway: {EvNone: true},
	InclusiveGateway: {EvNone: true}, EventBasedGateway: {EvNone: true},
}

// Unsupported returns nodes that can be stored/exported but not yet executed.
func (p *Process) Unsupported() []*Node {
	var out []*Node
	for _, n := range p.Nodes {
		if !executable[n.Type][n.Event] {
			out = append(out, n)
		}
	}
	return out
}

func IsActivity(t ElementType) bool {
	switch t {
	case Task, UserTask, ManualTask, ServiceTask, ScriptTask, BusinessRuleTask,
		SendTask, ReceiveTask, SubProcess, AdHocSubProcess, Transaction, CallActivity:
		return true
	}
	return false
}

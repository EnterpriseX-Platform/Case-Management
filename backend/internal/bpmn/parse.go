package bpmn

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// ExtNS is the namespace for Case-Management extension attributes.
const ExtNS = "http://enterprisex.io/schema/cm/1.0"

type xnode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []xnode    `xml:",any"`
	Text     string     `xml:",chardata"`
}

func (x *xnode) attr(local string) string {
	for _, a := range x.Attrs {
		if a.Name.Local == local && (a.Name.Space == "" || a.Name.Space == x.XMLName.Space) {
			return a.Value
		}
	}
	return ""
}

func (x *xnode) child(local string) *xnode {
	for i := range x.Children {
		if x.Children[i].XMLName.Local == local {
			return &x.Children[i]
		}
	}
	return nil
}

// Parse reads BPMN 2.0 XML and returns all processes found in definitions.
func Parse(r io.Reader) ([]*Process, error) {
	var root xnode
	if err := xml.NewDecoder(r).Decode(&root); err != nil {
		return nil, fmt.Errorf("bpmn: %w", err)
	}
	if root.XMLName.Local != "definitions" {
		return nil, fmt.Errorf("bpmn: root element is %q, want definitions", root.XMLName.Local)
	}
	var out []*Process
	for i := range root.Children {
		c := &root.Children[i]
		if c.XMLName.Local != "process" {
			continue
		}
		p := &Process{ID: c.attr("id"), Name: c.attr("name"),
			Nodes: map[string]*Node{}, Flows: map[string]*Flow{}}
		walk(p, c, "")
		for _, f := range p.Flows { // fill incoming/outgoing from flows (authoritative)
			if s := p.Nodes[f.Source]; s != nil && !contains(s.Outgoing, f.ID) {
				s.Outgoing = append(s.Outgoing, f.ID)
			}
			if t := p.Nodes[f.Target]; t != nil && !contains(t.Incoming, f.ID) {
				t.Incoming = append(t.Incoming, f.ID)
			}
		}
		if err := validate(p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("bpmn: no process element")
	}
	return out, nil
}

func ParseString(s string) ([]*Process, error) { return Parse(strings.NewReader(s)) }

func walk(p *Process, parent *xnode, scope string) {
	for i := range parent.Children {
		c := &parent.Children[i]
		name := c.XMLName.Local
		switch {
		case name == "sequenceFlow":
			f := &Flow{ID: c.attr("id"), Source: c.attr("sourceRef"), Target: c.attr("targetRef"), Name: c.attr("name")}
			if ce := c.child("conditionExpression"); ce != nil {
				f.Condition = strings.TrimSpace(ce.Text)
			}
			p.Flows[f.ID] = f
		case name == "laneSet":
			for _, l := range c.Children {
				if l.XMLName.Local != "lane" {
					continue
				}
				lane := Lane{ID: l.attr("id"), Name: l.attr("name"), Role: extAttr(&l, "role")}
				for _, r := range l.Children {
					if r.XMLName.Local == "flowNodeRef" {
						lane.Nodes = append(lane.Nodes, strings.TrimSpace(r.Text))
					}
				}
				p.Lanes = append(p.Lanes, lane)
			}
		case isFlowNode(name):
			n := &Node{ID: c.attr("id"), Type: ElementType(name), Name: c.attr("name"), Parent: scope,
				AttachedTo: c.attr("attachedToRef"), DefaultFlow: c.attr("default"),
				CalledElem: c.attr("calledElement"), Ext: map[string]string{}}
			if name == "boundaryEvent" {
				n.CancelActivity = c.attr("cancelActivity") != "false"
			}
			if name == "subProcess" && c.attr("triggeredByEvent") == "true" {
				n.Ext["eventSubProcess"] = "true"
			}
			for _, a := range c.Attrs {
				if a.Name.Space == ExtNS || a.Name.Space == "cm" {
					n.Ext[a.Name.Local] = a.Value
				}
			}
			for j := range c.Children {
				cc := &c.Children[j]
				switch cc.XMLName.Local {
				case "incoming":
					n.Incoming = append(n.Incoming, strings.TrimSpace(cc.Text))
				case "outgoing":
					n.Outgoing = append(n.Outgoing, strings.TrimSpace(cc.Text))
				case "script":
					n.Script = strings.TrimSpace(cc.Text)
				case "multiInstanceLoopCharacteristics":
					mi := &MultiInstance{Sequential: cc.attr("isSequential") == "true",
						Collection: firstNonEmpty(extAttr(cc, "collection"), cc.attr("collection")),
						ElementVar: firstNonEmpty(extAttr(cc, "elementVariable"), cc.attr("elementVariable"))}
					if lc := cc.child("loopCardinality"); lc != nil {
						mi.Cardinality = strings.TrimSpace(lc.Text)
					}
					n.MultiInstance = mi
				case "extensionElements":
					for _, e := range cc.Children {
						if e.XMLName.Space == ExtNS {
							for _, a := range e.Attrs {
								n.Ext[e.XMLName.Local+"."+a.Name.Local] = a.Value
							}
							if t := strings.TrimSpace(e.Text); t != "" {
								n.Ext[e.XMLName.Local] = t
							}
						}
					}
				default:
					if strings.HasSuffix(cc.XMLName.Local, "EventDefinition") {
						n.Event = EventKind(strings.TrimSuffix(cc.XMLName.Local, "EventDefinition"))
						n.MessageRef = cc.attr("messageRef")
						n.SignalRef = cc.attr("signalRef")
						n.ErrorCode = firstNonEmpty(cc.attr("errorRef"), cc.attr("escalationRef"))
						for _, t := range cc.Children {
							switch t.XMLName.Local {
							case "timeDuration", "timeDate", "timeCycle":
								n.TimerExpr = t.XMLName.Local + ":" + strings.TrimSpace(t.Text)
							case "condition":
								n.ConditionExp = strings.TrimSpace(t.Text)
							}
						}
						if n.Event == EvLink {
							n.MessageRef = cc.attr("name")
						}
					}
				}
			}
			p.Nodes[n.ID] = n
			if name == "subProcess" || name == "adHocSubProcess" || name == "transaction" {
				walk(p, c, n.ID)
			}
		}
	}
}

func extAttr(x *xnode, local string) string {
	for _, a := range x.Attrs {
		if a.Name.Local == local && (a.Name.Space == ExtNS || a.Name.Space == "cm") {
			return a.Value
		}
	}
	return ""
}

func isFlowNode(local string) bool {
	switch ElementType(local) {
	case StartEvent, EndEvent, IntermediateCatchEvent, IntermediateThrowEvent, BoundaryEvent,
		Task, UserTask, ManualTask, ServiceTask, ScriptTask, BusinessRuleTask, SendTask, ReceiveTask,
		SubProcess, AdHocSubProcess, Transaction, CallActivity,
		ExclusiveGateway, ParallelGateway, InclusiveGateway, EventBasedGateway, ComplexGateway:
		return true
	}
	return false
}

func validate(p *Process) error {
	if len(p.StartNodes("")) == 0 {
		return fmt.Errorf("bpmn: process %s has no start event", p.ID)
	}
	for _, f := range p.Flows {
		if p.Nodes[f.Source] == nil || p.Nodes[f.Target] == nil {
			return fmt.Errorf("bpmn: flow %s references unknown node", f.ID)
		}
	}
	for _, n := range p.Nodes {
		if n.Type != EndEvent && n.Type != BoundaryEvent && len(n.Outgoing) == 0 &&
			!(n.Type == SubProcess && n.Ext["eventSubProcess"] == "true") && n.Event != EvLink {
			return fmt.Errorf("bpmn: node %s (%s) has no outgoing flow", n.ID, n.Type)
		}
	}
	return nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

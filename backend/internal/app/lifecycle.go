package app

// Lifecycle designer (business mode). A business user describes the case
// as stages holding simple steps; the platform generates the BPMN flow, the
// stage mapping and the step screens from it. Developers can keep editing
// the generated BPMN in developer mode (the lifecycle is then "detached" so
// it never overwrites hand-made changes).

import (
	"context"
	"fmt"
	"strings"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

type Lifecycle = store.Lifecycle

var stepKinds = map[string]bool{"task": true, "approval": true, "automation": true, "decision": true, "wait": true, "child": true, "timer": true}

func validateLifecycle(l *Lifecycle) error {
	if l == nil || len(l.Stages) == 0 {
		return invalid("the lifecycle needs at least one stage")
	}
	keys := map[string]bool{}
	steps := map[string]bool{}
	primary := 0
	for _, st := range l.Stages {
		if !keyPattern.MatchString(st.Key) || keys[st.Key] {
			return invalid("stage key %q is invalid or duplicated", st.Key)
		}
		keys[st.Key] = true
		if st.Kind == "primary" {
			primary++
		}
		for _, s := range st.Steps {
			if !keyPattern.MatchString(s.Key) || steps[s.Key] {
				return invalid("step key %q is invalid or duplicated", s.Key)
			}
			steps[s.Key] = true
			if !stepKinds[s.Kind] {
				return invalid("step %s: unknown kind %q", s.Key, s.Kind)
			}
		}
	}
	if primary == 0 {
		return invalid("at least one primary stage is required")
	}
	for _, st := range l.Stages {
		for _, s := range st.Steps {
			if s.Kind == "approval" && s.OnReject != "" && s.OnReject != "end" && !keys[s.OnReject] && !steps[s.OnReject] {
				return invalid("step %s: reject target %q does not exist", s.Key, s.OnReject)
			}
		}
	}
	return nil
}

// bpmn id helpers keep generated ids stable across regenerations so that
// version migration can map running cases by id.
func sid(s string) string { return strings.ReplaceAll(s, "-", "_") }

// GenerateBPMN turns a lifecycle into BPMN 2.0 XML (with diagram layout),
// the stage mapping and step screen bindings.
func GenerateBPMN(processKey, name string, l *Lifecycle) (string, *CaseModel, []store.Binding, error) {
	if err := validateLifecycle(l); err != nil {
		return "", nil, nil, err
	}
	type node struct {
		id, tag, name, attrs, inner string
		x, y, w, h                  int
	}
	type flow struct{ id, src, dst, cond string }
	var nodes []node
	var flows []flow
	var binds []store.Binding
	model := &CaseModel{}
	fid := 0
	addFlow := func(src, dst, cond string) {
		fid++
		flows = append(flows, flow{fmt.Sprintf("f%d", fid), src, dst, cond})
	}
	esc := func(s string) string {
		r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
		return r.Replace(s)
	}
	const D, Y = 170, 120
	x := 0
	nodes = append(nodes, node{id: "start", tag: "startEvent", name: "Start", x: x + 60, y: Y + 22, w: 36, h: 36})
	prev := "start"
	stageEntry := map[string]string{}
	stepID := map[string]string{}
	var pendingRejects []struct{ gw, target string }
	altY := Y + 220

	// link connects cur -> dst; cur may be "" (no incoming flow: alternate
	// stages are entered by an action) or "<gateway>#yes" (approve branch).
	var link func(cur, dst string)
	link = func(cur, dst string) {
		switch {
		case cur == "":
		case strings.HasSuffix(cur, "#yes"):
			gw := strings.TrimSuffix(cur, "#yes")
			for i := range flows {
				if flows[i].id == gw+"_yes" {
					flows[i].dst = dst
				}
			}
		default:
			addFlow(cur, dst, "")
		}
	}

	// emit lays out a stage's steps from startX on the given line, linked from cur.
	emit := func(st store.LCStage, line, startX int, cur string) (string, int, []string) {
		cx := startX
		var ids []string
		for _, s := range st.Steps {
			id := sid(s.Key)
			stepID[s.Key] = id
			attrs := ""
			if s.Role != "" {
				attrs += fmt.Sprintf(` cm:role="%s"`, esc(s.Role))
			}
			if s.SLA != "" && s.Kind != "timer" {
				attrs += fmt.Sprintf(` cm:sla="%s"`, esc(s.SLA))
			}
			if s.Form != "" {
				attrs += fmt.Sprintf(` cm:form="%s"`, esc(s.Form))
			}
			tag, inner, w, h := "userTask", "", 120, 80
			switch s.Kind {
			case "automation":
				tag = "serviceTask"
				if s.Connector != "" {
					attrs += fmt.Sprintf(` cm:connector="%s"`, esc(s.Connector))
				}
			case "decision":
				tag = "businessRuleTask"
				if s.Decision != "" {
					attrs += fmt.Sprintf(` cm:decision="%s"`, esc(s.Decision))
				}
			case "wait":
				tag = "receiveTask"
				if s.Message != "" {
					attrs += fmt.Sprintf(` cm:message="%s"`, esc(s.Message))
				}
			case "child":
				tag = "callActivity"
				attrs += fmt.Sprintf(` calledElement="%s"`, esc(s.CaseType))
			case "timer":
				tag, w, h = "intermediateCatchEvent", 36, 36
				inner = fmt.Sprintf("<timerEventDefinition><timeDuration>%s</timeDuration></timerEventDefinition>", esc(firstNonEmpty(s.SLA, "P1D")))
			}
			if s.Multi != "" && (tag == "userTask" || tag == "callActivity") {
				inner += fmt.Sprintf(`<multiInstanceLoopCharacteristics cm:collection="%s" cm:elementVariable="item"/>`, esc(s.Multi))
			}
			if s.When != "" { // optional step: skipped when the condition is false
				gw, join := id+"_when", id+"_join"
				nodes = append(nodes, node{id: gw, tag: "exclusiveGateway", attrs: fmt.Sprintf(` default="%s_skip"`, gw), x: cx, y: line + 15, w: 50, h: 50})
				link(cur, gw)
				cx += 90
				nodes = append(nodes, node{id: id, tag: tag, name: s.Name, attrs: attrs, inner: inner, x: cx, y: line + (80-h)/2, w: w, h: h})
				flows = append(flows, flow{gw + "_do", gw, id, "${" + s.When + "}"})
				cx += 160
				nodes = append(nodes, node{id: join, tag: "exclusiveGateway", x: cx, y: line + 15, w: 50, h: 50})
				addFlow(id, join, "")
				flows = append(flows, flow{gw + "_skip", gw, join, ""})
				ids = append(ids, gw, id, join)
				cur = join
				cx += 90
			} else {
				nodes = append(nodes, node{id: id, tag: tag, name: s.Name, attrs: attrs, inner: inner, x: cx, y: line + (80-h)/2, w: w, h: h})
				ids = append(ids, id)
				link(cur, id)
				cur = id
				cx += D
			}
			if s.Kind == "approval" {
				gw := id + "_ok"
				nodes = append(nodes, node{id: gw, tag: "exclusiveGateway", name: s.Name + "?", attrs: fmt.Sprintf(` default="%s_no"`, gw), x: cx - 25, y: line + 15, w: 50, h: 50})
				ids = append(ids, gw)
				addFlow(cur, gw, "")
				field := firstNonEmpty(s.DecisionField, id+"_approved")
				flows = append(flows, flow{gw + "_yes", gw, "", "${" + field + " == true}"})
				pendingRejects = append(pendingRejects, struct{ gw, target string }{gw, firstNonEmpty(s.OnReject, "end")})
				cur = gw + "#yes"
				cx += 100
			}
		}
		return cur, cx, ids
	}

	var resolutions []store.LCStage
	x += D
	for _, st := range l.Stages {
		switch st.Kind {
		case "primary":
			if len(st.Steps) > 0 {
				stageEntry[st.Key] = sid(st.Steps[0].Key)
				if st.Steps[0].When != "" {
					stageEntry[st.Key] += "_when"
				}
			}
			cur, nx, ids := emit(st, Y, x, prev)
			prev, x = cur, nx
			model.Stages = append(model.Stages, store.Stage{Key: st.Key, Name: st.Name, Kind: "primary", Steps: ids, SLA: st.SLA, Entry: stageEntry[st.Key]})
		case "alternate":
			if len(st.Steps) > 0 {
				stageEntry[st.Key] = sid(st.Steps[0].Key)
			}
			cur, ex, ids := emit(st, altY, D*2, "")
			endID := sid(st.Key) + "_end"
			nodes = append(nodes, node{id: endID, tag: "endEvent", name: st.Name, x: ex, y: altY + 22, w: 36, h: 36})
			link(cur, endID)
			model.Stages = append(model.Stages, store.Stage{Key: st.Key, Name: st.Name, Kind: "alternate", Steps: ids, SLA: st.SLA, Entry: stageEntry[st.Key]})
			altY += 200
		case "resolution":
			resolutions = append(resolutions, st)
		}
	}
	// resolution: the end of the main line (+ reject end)
	endIDs := []string{"end"}
	endName := "Resolved"
	if len(resolutions) > 0 {
		endName = resolutions[0].Name
	}
	nodes = append(nodes, node{id: "end", tag: "endEvent", name: endName, x: x, y: Y + 22, w: 36, h: 36})
	link(prev, "end")
	for _, rj := range pendingRejects {
		dst := rj.target
		switch {
		case dst == "end":
			dst = "rejected_end"
			if !contains(endIDs, dst) {
				nodes = append(nodes, node{id: dst, tag: "endEvent", name: "Rejected", x: x, y: Y + 140, w: 36, h: 36})
				endIDs = append(endIDs, dst)
			}
		case stageEntry[dst] != "":
			dst = stageEntry[dst]
		case stepID[dst] != "":
			dst = stepID[dst]
		}
		flows = append(flows, flow{rj.gw + "_no", rj.gw, dst, ""})
	}
	if len(resolutions) > 0 {
		st := resolutions[0]
		res := store.Stage{Key: st.Key, Name: st.Name, Kind: "resolution", Steps: endIDs}
		for _, a := range model.Stages {
			if a.Kind == "alternate" {
				res.Steps = append(res.Steps, sid(a.Key)+"_end")
			}
		}
		model.Stages = append(model.Stages, res)
	}
	// screens
	for _, st := range l.Stages {
		for _, s := range st.Steps {
			if (s.Kind == "task" || s.Kind == "approval") && (s.Form != "" || s.Screen != "" || s.URL != "") {
				b := store.Binding{NodeID: sid(s.Key), UI: "form", FormKey: s.Form, ScreenKey: s.Screen, Role: s.Role, SLA: s.SLA}
				if s.URL != "" {
					b.UI, b.URL = "external", s.URL
				}
				binds = append(binds, b)
			}
		}
	}

	// XML
	var b strings.Builder
	pid := sid(processKey)
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI" xmlns:dc="http://www.omg.org/spec/DD/20100524/DC" xmlns:di="http://www.omg.org/spec/DD/20100524/DI" xmlns:cm="http://enterprisex.io/schema/cm/1.0" id="defs_%s" targetNamespace="http://enterprisex.io/cm/generated">
  <process id="%s" name="%s" isExecutable="true">
`, pid, pid, esc(name))
	for _, n := range nodes {
		fmt.Fprintf(&b, "    <%s id=\"%s\" name=\"%s\"%s>%s</%s>\n", n.tag, n.id, esc(n.name), n.attrs, n.inner, n.tag)
	}
	for _, f := range flows {
		if f.src == "" || f.dst == "" {
			continue
		}
		if f.cond != "" {
			fmt.Fprintf(&b, "    <sequenceFlow id=\"%s\" sourceRef=\"%s\" targetRef=\"%s\"><conditionExpression>%s</conditionExpression></sequenceFlow>\n", f.id, f.src, f.dst, esc(f.cond))
		} else {
			fmt.Fprintf(&b, "    <sequenceFlow id=\"%s\" sourceRef=\"%s\" targetRef=\"%s\"/>\n", f.id, f.src, f.dst)
		}
	}
	fmt.Fprintf(&b, "  </process>\n  <bpmndi:BPMNDiagram id=\"dia\"><bpmndi:BPMNPlane id=\"plane\" bpmnElement=\"%s\">\n", pid)
	pos := map[string]node{}
	for _, n := range nodes {
		pos[n.id] = n
		marker := ""
		if n.tag == "exclusiveGateway" {
			marker = ` isMarkerVisible="true"`
		}
		fmt.Fprintf(&b, "    <bpmndi:BPMNShape id=\"%s_di\" bpmnElement=\"%s\"%s><dc:Bounds x=\"%d\" y=\"%d\" width=\"%d\" height=\"%d\"/></bpmndi:BPMNShape>\n", n.id, n.id, marker, n.x, n.y, n.w, n.h)
	}
	for _, f := range flows {
		s, d := pos[f.src], pos[f.dst]
		if f.src == "" || f.dst == "" {
			continue
		}
		var pts [][2]int
		if d.x > s.x+s.w-1 {
			a, z := [2]int{s.x + s.w, s.y + s.h/2}, [2]int{d.x, d.y + d.h/2}
			if a[1] == z[1] {
				pts = [][2]int{a, z}
			} else {
				m := (a[0] + z[0]) / 2
				pts = [][2]int{a, {m, a[1]}, {m, z[1]}, z}
			}
		} else {
			a, z := [2]int{s.x + s.w/2, s.y + s.h}, [2]int{d.x + d.w/2, d.y + d.h}
			low := max(s.y+s.h, d.y+d.h) + 40
			pts = [][2]int{a, {a[0], low}, {z[0], low}, z}
		}
		fmt.Fprintf(&b, "    <bpmndi:BPMNEdge id=\"%s_di\" bpmnElement=\"%s\">", f.id, f.id)
		for _, p := range pts {
			fmt.Fprintf(&b, "<di:waypoint x=\"%d\" y=\"%d\"/>", p[0], p[1])
		}
		b.WriteString("</bpmndi:BPMNEdge>\n")
	}
	b.WriteString("  </bpmndi:BPMNPlane></bpmndi:BPMNDiagram>\n</definitions>\n")
	return b.String(), model, binds, nil
}

// SaveLifecycle stores the lifecycle of a draft and regenerates its BPMN,
// stage mapping and step screens. Data, rules, actions, decisions and
// connectors already in the model are kept.
func (s *Service) SaveLifecycle(ctx context.Context, by Actor, vid string, l *Lifecycle) (*store.Version, []string, error) {
	if !by.Has(RoleDesigner) {
		return nil, nil, ErrForbidden
	}
	v, err := s.S.GetVersion(ctx, vid)
	if err != nil {
		return nil, nil, err
	}
	if v.Status != store.Draft {
		return nil, nil, conflict("version %d is %s; clone it to change the lifecycle", v.No, v.Status)
	}
	if v.Model != nil && v.Model.Lifecycle != nil && v.Model.Lifecycle.Detached {
		return nil, nil, conflict("the flow was edited in developer mode; the lifecycle no longer generates it")
	}
	pd, err := s.S.GetProcess(ctx, v.ProcessKey)
	if err != nil {
		return nil, nil, err
	}
	xml, gen, binds, err := GenerateBPMN(v.ProcessKey, pd.Name, l)
	if err != nil {
		return nil, nil, err
	}
	_, warn, err := ValidateBPMN(xml)
	if err != nil {
		return nil, nil, fmt.Errorf("generated flow is invalid: %w", err)
	}
	m := &CaseModel{}
	if v.Model != nil {
		cp := *v.Model
		m = &cp
	}
	m.Stages = gen.Stages
	m.Lifecycle = l
	v.BPMN, v.Checksum, v.Model = xml, checksum(xml), m
	if err := s.S.PutVersion(ctx, v); err != nil {
		return nil, nil, err
	}
	p, _, err := s.Process(ctx, vid)
	if err != nil {
		return nil, nil, err
	}
	if err := ValidateModel(m, p); err != nil {
		return nil, nil, err
	}
	if err := s.S.PutBindings(ctx, vid, binds); err != nil {
		return nil, nil, err
	}
	s.audit(ctx, by, "lifecycle.save", "version", vid, map[string]any{"stages": len(l.Stages)})
	return v, warn, nil
}

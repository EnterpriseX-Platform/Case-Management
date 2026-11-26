package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

func (s *Service) CreateProcess(ctx context.Context, by Actor, p store.ProcessDef) (*store.ProcessDef, error) {
	if !by.Has(RoleDesigner) {
		return nil, ErrForbidden
	}
	p.Key = strings.ToLower(strings.TrimSpace(p.Key))
	if p.Key == "" || p.Name == "" {
		return nil, invalid("key and name are required")
	}
	if _, err := s.S.GetProcess(ctx, p.Key); err == nil {
		return nil, conflict("case type %q exists", p.Key)
	}
	p.ID, p.CreatedAt = id.New(), s.Now()
	s.audit(ctx, by, "process.create", "process", p.Key, nil)
	return &p, s.S.PutProcess(ctx, &p)
}

// ValidateBPMN parses xml and returns the process plus non-blocking warnings.
func ValidateBPMN(xml string) (*bpmn.Process, []string, error) {
	ps, err := bpmn.ParseString(xml)
	if err != nil {
		return nil, nil, invalid("%v", err)
	}
	var warn []string
	if len(ps) > 1 {
		warn = append(warn, fmt.Sprintf("%d processes found; only %q is executed", len(ps), ps[0].ID))
	}
	for _, n := range ps[0].Unsupported() {
		warn = append(warn, fmt.Sprintf("%s %s (%s %s) is stored but not executable yet", n.ID, n.Name, n.Type, n.Event))
	}
	sort.Strings(warn)
	return ps[0], warn, nil
}

func (s *Service) CreateDraft(ctx context.Context, by Actor, key, xml, note string, fy int) (*store.Version, []string, error) {
	if !by.Has(RoleDesigner) {
		return nil, nil, ErrForbidden
	}
	if _, err := s.S.GetProcess(ctx, key); err != nil {
		return nil, nil, err
	}
	_, warn, err := ValidateBPMN(xml)
	if err != nil {
		return nil, nil, err
	}
	vs, err := s.S.ListVersions(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	no := 1
	if len(vs) > 0 {
		no = vs[0].No + 1
	}
	v := &store.Version{ID: id.New(), ProcessKey: key, No: no, Status: store.Draft, FiscalYear: fy,
		BPMN: xml, Checksum: checksum(xml), Note: note, CreatedBy: by.ID, CreatedAt: s.Now()}
	s.audit(ctx, by, "version.create", "version", v.ID, map[string]any{"no": no})
	return v, warn, s.S.PutVersion(ctx, v)
}

func (s *Service) UpdateDraft(ctx context.Context, by Actor, vid, xml string, fy int, note string) (*store.Version, []string, error) {
	if !by.Has(RoleDesigner) {
		return nil, nil, ErrForbidden
	}
	v, err := s.S.GetVersion(ctx, vid)
	if err != nil {
		return nil, nil, err
	}
	if v.Status != store.Draft {
		return nil, nil, conflict("version %d is %s; clone it to make changes", v.No, v.Status)
	}
	var warn []string
	if xml != "" {
		if _, warn, err = ValidateBPMN(xml); err != nil {
			return nil, nil, err
		}
		v.BPMN, v.Checksum = xml, checksum(xml)
	}
	if fy != 0 {
		v.FiscalYear = fy
	}
	if note != "" {
		v.Note = note
	}
	s.audit(ctx, by, "version.update", "version", v.ID, nil)
	return v, warn, s.S.PutVersion(ctx, v)
}

// Clone copies a version (and its screen bindings) into a new draft, e.g.
// to prepare next fiscal year's flow from this year's.
func (s *Service) Clone(ctx context.Context, by Actor, vid string, fy int) (*store.Version, error) {
	src, err := s.S.GetVersion(ctx, vid)
	if err != nil {
		return nil, err
	}
	v, _, err := s.CreateDraft(ctx, by, src.ProcessKey, src.BPMN, fmt.Sprintf("cloned from v%d", src.No), fy)
	if err != nil {
		return nil, err
	}
	v.ClonedFrom = src.ID
	v.Model = src.Model
	if err := s.S.PutVersion(ctx, v); err != nil {
		return nil, err
	}
	b, err := s.S.GetBindings(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	for i := range b {
		b[i].VersionID = v.ID
	}
	return v, s.S.PutBindings(ctx, v.ID, b)
}

// Transition moves a version through its life cycle:
// draft -submit-> in_review -approve-> approved -publish-> published -deprecate-> deprecated -archive-> archived
// in_review -reject-> draft. Approval is maker-checker: the approver must
// not be the author.
func (s *Service) Transition(ctx context.Context, by Actor, vid, action string) (*store.Version, error) {
	var out *store.Version
	err := s.S.Tx(ctx, func(ctx context.Context) error {
		v, err := s.S.GetVersion(ctx, vid)
		if err != nil {
			return err
		}
		need := func(st store.VersionStatus, role string) error {
			if v.Status != st {
				return conflict("cannot %s a %s version", action, v.Status)
			}
			if !by.Has(role) {
				return ErrForbidden
			}
			return nil
		}
		switch action {
		case "submit":
			if err := need(store.Draft, RoleDesigner); err != nil {
				return err
			}
			v.Status = store.InReview
		case "reject":
			if err := need(store.InReview, RoleApprover); err != nil {
				return err
			}
			v.Status = store.Draft
		case "approve":
			if err := need(store.InReview, RoleApprover); err != nil {
				return err
			}
			if v.CreatedBy == by.ID && !by.Has(RoleAdmin) {
				return fmt.Errorf("%w: maker-checker: author cannot approve own version", ErrForbidden)
			}
			v.Status, v.ApprovedBy = store.Approved, by.ID
		case "publish":
			if err := need(store.Approved, RoleApprover); err != nil {
				return err
			}
			if v.FiscalYear == 0 {
				return invalid("set the fiscal year before publishing")
			}
			p, _, err := ValidateBPMN(v.BPMN)
			if err != nil {
				return err
			}
			if u := p.Unsupported(); len(u) > 0 {
				return invalid("cannot publish: %d element(s) not executable yet (first: %s %s)", len(u), u[0].ID, u[0].Type)
			}
			if err := s.checkBindings(ctx, v, p); err != nil {
				return err
			}
			if err := ValidateModel(v.Model, p); err != nil {
				return err
			}
			vs, err := s.S.ListVersions(ctx, v.ProcessKey)
			if err != nil {
				return err
			}
			for _, o := range vs {
				if o.ID != v.ID && o.Status == store.Published && o.FiscalYear == v.FiscalYear {
					o.Status = store.Deprecated // running cases keep using it
					if err := s.S.PutVersion(ctx, o); err != nil {
						return err
					}
				}
			}
			now := s.Now()
			v.Status, v.PublishedAt = store.Published, &now
		case "deprecate":
			if err := need(store.Published, RoleApprover); err != nil {
				return err
			}
			v.Status = store.Deprecated
		case "archive":
			if err := need(store.Deprecated, RoleAdmin); err != nil {
				return err
			}
			cs, err := s.S.ListCases(ctx, "", v.ID, 0)
			if err != nil {
				return err
			}
			for _, c := range cs {
				if c.Instance != nil && c.Instance.Status == engine.Running {
					return conflict("version still has running cases; migrate them first")
				}
			}
			v.Status = store.Archived
		default:
			return invalid("unknown action %q", action)
		}
		s.audit(ctx, by, "version."+action, "version", v.ID, map[string]any{"no": v.No, "status": v.Status})
		out = v
		return s.S.PutVersion(ctx, v)
	})
	return out, err
}

func (s *Service) checkBindings(ctx context.Context, v *store.Version, p *bpmn.Process) error {
	b, err := s.S.GetBindings(ctx, v.ID)
	if err != nil {
		return err
	}
	for _, x := range b {
		if p.Nodes[x.NodeID] == nil {
			return invalid("screen binding refers to missing step %s", x.NodeID)
		}
		if x.UI == engine.UIExternal && x.URL == "" {
			return invalid("step %s uses an external screen but has no URL", x.NodeID)
		}
		if x.FormKey != "" {
			if _, err := s.S.GetForm(ctx, x.FormKey); errors.Is(err, store.ErrNotFound) {
				return invalid("step %s uses unknown form %q", x.NodeID, x.FormKey)
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- bindings

// StepInfo is one row of the screen-mapping table for a version.
type StepInfo struct {
	NodeID  string        `json:"nodeId"`
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	Lane    string        `json:"lane,omitempty"`
	Human   bool          `json:"human"`
	Binding store.Binding `json:"binding"`
}

func (s *Service) Steps(ctx context.Context, vid string) ([]StepInfo, error) {
	p, _, err := s.Process(ctx, vid)
	if err != nil {
		return nil, err
	}
	b, err := s.S.GetBindings(ctx, vid)
	if err != nil {
		return nil, err
	}
	bm := map[string]store.Binding{}
	for _, x := range b {
		bm[x.NodeID] = x
	}
	var out []StepInfo
	for _, n := range p.Nodes {
		if !bpmn.IsActivity(n.Type) {
			continue
		}
		human := n.Type == bpmn.UserTask || n.Type == bpmn.ManualTask || n.Type == bpmn.Task
		si := StepInfo{NodeID: n.ID, Name: n.Name, Type: string(n.Type), Human: human, Binding: bm[n.ID]}
		if l := p.LaneOf(n.ID); l != nil {
			si.Lane = l.Name
		}
		if si.Binding.NodeID == "" {
			si.Binding = store.Binding{VersionID: vid, NodeID: n.ID, UI: engine.UIMode(firstNonEmpty(n.Ext["ui"], "form")),
				FormKey: n.Ext["form"], URL: n.Ext["url"], Role: n.Ext["role"], SLA: n.Ext["sla"]}
			if !human {
				si.Binding.UI = engine.UINone
			}
		}
		out = append(out, si)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out, nil
}

// PutBindings saves screen mappings. Allowed on drafts; on published
// versions only role / SLA / toggle (hot changes) may change.
func (s *Service) PutBindings(ctx context.Context, by Actor, vid string, in []store.Binding) error {
	if !by.Has(RoleDesigner) {
		return ErrForbidden
	}
	v, err := s.S.GetVersion(ctx, vid)
	if err != nil {
		return err
	}
	p, _, err := s.Process(ctx, vid)
	if err != nil {
		return err
	}
	if v.Status != store.Draft {
		old, _ := s.S.GetBindings(ctx, vid)
		om := map[string]store.Binding{}
		for _, o := range old {
			om[o.NodeID] = o
		}
		for _, b := range in {
			o := om[b.NodeID]
			if o.NodeID != "" && (o.UI != b.UI || o.FormKey != b.FormKey || o.ScreenKey != b.ScreenKey || o.URL != b.URL) {
				return conflict("version %d is %s: only role, SLA and step toggle can change; clone to remap screens", v.No, v.Status)
			}
		}
	}
	for i := range in {
		in[i].VersionID = vid
		if p.Nodes[in[i].NodeID] == nil {
			return invalid("unknown step %s", in[i].NodeID)
		}
		if in[i].SLA != "" {
			if _, err := engine.ParseDuration(in[i].SLA); err != nil {
				return invalid("step %s: %v", in[i].NodeID, err)
			}
		}
	}
	s.audit(ctx, by, "bindings.update", "version", vid, map[string]any{"count": len(in)})
	return s.S.PutBindings(ctx, vid, in)
}

// ---------------------------------------------------------------- diff

type NodeChange struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Fields []string `json:"fields,omitempty"`
}

type Diff struct {
	From    string       `json:"from"`
	To      string       `json:"to"`
	Added   []NodeChange `json:"added"`
	Removed []NodeChange `json:"removed"`
	Changed []NodeChange `json:"changed"`
}

func (s *Service) Diff(ctx context.Context, fromID, toID string) (*Diff, error) {
	a, _, err := s.Process(ctx, fromID)
	if err != nil {
		return nil, err
	}
	b, _, err := s.Process(ctx, toID)
	if err != nil {
		return nil, err
	}
	return DiffProcesses(fromID, toID, a, b), nil
}

func DiffProcesses(fromID, toID string, a, b *bpmn.Process) *Diff {
	d := &Diff{From: fromID, To: toID, Added: []NodeChange{}, Removed: []NodeChange{}, Changed: []NodeChange{}}
	targets := func(p *bpmn.Process, n *bpmn.Node) string {
		var t []string
		for _, f := range n.Outgoing {
			t = append(t, p.Flows[f].Target)
		}
		sort.Strings(t)
		return strings.Join(t, ",")
	}
	for id, n := range a.Nodes {
		m, ok := b.Nodes[id]
		if !ok {
			d.Removed = append(d.Removed, NodeChange{ID: id, Name: n.Name, Type: string(n.Type)})
			continue
		}
		var f []string
		if n.Name != m.Name {
			f = append(f, "name")
		}
		if n.Type != m.Type || n.Event != m.Event {
			f = append(f, "type")
		}
		if targets(a, n) != targets(b, m) {
			f = append(f, "flow")
		}
		if fmt.Sprint(n.Ext) != fmt.Sprint(m.Ext) {
			f = append(f, "settings")
		}
		if len(f) > 0 {
			d.Changed = append(d.Changed, NodeChange{ID: id, Name: m.Name, Type: string(m.Type), Fields: f})
		}
	}
	for id, n := range b.Nodes {
		if _, ok := a.Nodes[id]; !ok {
			d.Added = append(d.Added, NodeChange{ID: id, Name: n.Name, Type: string(n.Type)})
		}
	}
	for _, l := range [][]NodeChange{d.Added, d.Removed, d.Changed} {
		sort.Slice(l, func(i, j int) bool { return l[i].ID < l[j].ID })
	}
	return d
}

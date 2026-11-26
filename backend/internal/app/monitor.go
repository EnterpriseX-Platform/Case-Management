package app

// Monitoring: operations overview, per-step process statistics (for the
// flow monitor), case explorer filters and the process inspector.
//
// Figures are computed from cases, work items and the event log on request.
// This suits up to tens of thousands of cases per query; larger installations
// add the metric rollup tables planned for Phase 6.

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
)

type DayCount struct {
	Day       string `json:"day"` // YYYY-MM-DD
	Started   int    `json:"started"`
	Completed int    `json:"completed"`
}

type Overview struct {
	From       time.Time      `json:"from"`
	Days       []DayCount     `json:"days"`
	Running    int            `json:"running"`
	Completed  int            `json:"completed"`
	Started    int            `json:"started"`
	CycleP50   float64        `json:"cycleP50Hours"`
	CycleP90   float64        `json:"cycleP90Hours"`
	SLA        map[string]int `json:"sla"` // ontime | atrisk | overdue | none
	ByStage    []NameCount    `json:"byStage"`
	Workload   []PersonLoad   `json:"workload"`
	Unassigned int            `json:"unassigned"`
	Incidents  int            `json:"incidents"`
	ByProcess  map[string]int `json:"byProcess"`
}

type NameCount struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type PersonLoad struct {
	User    string `json:"user"`
	Name    string `json:"name"`
	Open    int    `json:"open"`
	Overdue int    `json:"overdue"`
	Away    bool   `json:"away"`
	Done    int    `json:"done"` // completed in the period
}

func (s *Service) caseScope(ctx context.Context, processKey, versionID string) ([]*store.Case, error) {
	cs, err := s.S.ListCases(ctx, processKey, versionID, 5000)
	if err != nil {
		return nil, err
	}
	out := cs[:0]
	for _, c := range cs {
		if !c.Test {
			out = append(out, c)
		}
	}
	return out, nil
}

func pct(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	i := int(float64(len(v)-1) * p)
	return v[i]
}

// MonitorOverview returns the operations dashboard figures for the period.
func (s *Service) MonitorOverview(ctx context.Context, by Actor, processKey string, days int) (*Overview, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	if days <= 0 || days > 366 {
		days = 30
	}
	now := s.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -days+1)
	cs, err := s.caseScope(ctx, processKey, "")
	if err != nil {
		return nil, err
	}
	o := &Overview{From: from, SLA: map[string]int{"ontime": 0, "atrisk": 0, "overdue": 0, "none": 0}, ByProcess: map[string]int{}}
	idx := map[string]*DayCount{}
	for d := 0; d < days; d++ {
		k := from.AddDate(0, 0, d).Format("2006-01-02")
		o.Days = append(o.Days, DayCount{Day: k})
	}
	for i := range o.Days {
		idx[o.Days[i].Day] = &o.Days[i]
	}
	var cycles []float64
	stage := map[string]*NameCount{}
	inScope := map[string]bool{}
	for _, c := range cs {
		inScope[c.ID] = true
		if c.Status == string(engine.Running) {
			o.Running++
			o.ByProcess[c.ProcessKey]++
			k := firstNonEmpty(c.Stage, "-")
			if stage[k] == nil {
				stage[k] = &NameCount{Key: k, Name: k}
			}
			stage[k].Count++
		}
		if !c.CreatedAt.Before(from) {
			o.Started++
			if d := idx[c.CreatedAt.In(now.Location()).Format("2006-01-02")]; d != nil {
				d.Started++
			}
		}
		if c.Status == string(engine.Completed) && !c.UpdatedAt.Before(from) {
			o.Completed++
			cycles = append(cycles, c.UpdatedAt.Sub(c.CreatedAt).Hours())
			if d := idx[c.UpdatedAt.In(now.Location()).Format("2006-01-02")]; d != nil {
				d.Completed++
			}
		}
	}
	o.CycleP50, o.CycleP90 = pct(cycles, .5), pct(cycles, .9)
	// stage names from the latest version models
	names := map[string]string{}
	if vs, err := s.S.ListVersions(ctx, processKey); err == nil {
		for _, v := range vs {
			if v.Model != nil {
				for _, st := range v.Model.Stages {
					if _, ok := names[st.Key]; !ok {
						names[st.Key] = st.Name
					}
				}
			}
		}
	}
	for _, v := range stage {
		v.Name = firstNonEmpty(names[v.Key], v.Key)
		o.ByStage = append(o.ByStage, *v)
	}
	sort.Slice(o.ByStage, func(i, j int) bool { return o.ByStage[i].Count > o.ByStage[j].Count })

	work, err := s.S.ListWork(ctx, store.WorkFilter{})
	if err != nil {
		return nil, err
	}
	people := map[string]*PersonLoad{}
	person := func(id string) *PersonLoad {
		if people[id] == nil {
			people[id] = &PersonLoad{User: id, Name: id}
		}
		return people[id]
	}
	for _, w := range work {
		if !inScope[w.CaseID] || w.Test {
			continue
		}
		open := w.Status == store.WorkOpen || w.Status == store.WorkClaimed
		if open {
			switch {
			case w.DueAt != nil && w.DueAt.Before(now):
				o.SLA["overdue"]++
			case w.GoalAt != nil && w.GoalAt.Before(now):
				o.SLA["atrisk"]++
			case w.DueAt != nil:
				o.SLA["ontime"]++
			default:
				o.SLA["none"]++
			}
			if w.Assignee == "" {
				o.Unassigned++
			} else {
				p := person(w.Assignee)
				p.Open++
				if w.DueAt != nil && w.DueAt.Before(now) {
					p.Overdue++
				}
			}
		} else if w.Status == store.WorkDone && w.CompletedBy != "" && w.CompletedAt != nil && !w.CompletedAt.Before(from) && !strings.HasPrefix(w.CompletedBy, "system") {
			person(w.CompletedBy).Done++
		}
	}
	users, _ := s.S.ListUsers(ctx)
	for _, u := range users {
		if p := people[u.ID]; p != nil {
			p.Name, p.Away = firstNonEmpty(u.Name, u.ID), u.Away(now)
		} else if u.Away(now) {
			p := person(u.ID)
			p.Name, p.Away = firstNonEmpty(u.Name, u.ID), true
		}
	}
	for _, p := range people {
		o.Workload = append(o.Workload, *p)
	}
	sort.Slice(o.Workload, func(i, j int) bool { return o.Workload[i].Open > o.Workload[j].Open })
	inc, _ := s.S.ListIncidents(ctx, store.IncidentOpen, "")
	for _, i := range inc {
		if inScope[i.CaseID] {
			o.Incidents++
		}
	}
	return o, nil
}

// NodeStat is the flow monitor figure for one step.
type NodeStat struct {
	Node      string  `json:"node"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Active    int     `json:"active"`    // cases waiting here now
	Overdue   int     `json:"overdue"`   // open work past its deadline
	Incidents int     `json:"incidents"` // tokens parked by an error
	Passed    int     `json:"passed"`    // times the step was completed in the period
	AvgHours  float64 `json:"avgHours"`  // average time spent in the step
	P90Hours  float64 `json:"p90Hours"`
}

type ProcessStats struct {
	VersionID string     `json:"versionId"`
	BPMN      string     `json:"bpmn"`
	Cases     int        `json:"cases"`
	Nodes     []NodeStat `json:"nodes"`
}

// ProcessStatistics overlays live counts and durations on a version's flow.
func (s *Service) ProcessStatistics(ctx context.Context, by Actor, versionID string, days int) (*ProcessStats, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	if days <= 0 {
		days = 30
	}
	p, v, err := s.Process(ctx, versionID)
	if err != nil {
		return nil, err
	}
	since := s.Now().AddDate(0, 0, -days)
	cs, err := s.caseScope(ctx, v.ProcessKey, versionID)
	if err != nil {
		return nil, err
	}
	stats := map[string]*NodeStat{}
	for id, n := range p.Nodes {
		if n.Type == bpmn.BoundaryEvent || n.Type == bpmn.StartEvent {
			continue
		}
		stats[id] = &NodeStat{Node: id, Name: firstNonEmpty(n.Name, id), Type: string(n.Type)}
	}
	durations := map[string][]float64{}
	now := s.Now()
	open, _ := s.S.ListWork(ctx, store.WorkFilter{Status: []store.WorkStatus{store.WorkOpen, store.WorkClaimed}})
	overdue := map[string]int{}
	inScope := map[string]bool{}
	for _, c := range cs {
		inScope[c.ID] = true
	}
	for _, w := range open {
		if inScope[w.CaseID] && w.DueAt != nil && w.DueAt.Before(now) {
			overdue[w.Node]++
		}
	}
	n := 0
	for _, c := range cs {
		if c.Status == string(engine.Running) {
			for _, t := range c.Instance.Tokens {
				if st := stats[t.Node]; st != nil && t.Wait != engine.WaitScope && t.Wait != engine.WaitMulti {
					st.Active++
					if t.Wait == engine.WaitIncident {
						st.Incidents++
					}
				}
			}
		}
		if c.UpdatedAt.Before(since) {
			continue
		}
		n++
		if n > 2000 {
			continue // durations from the most recent 2,000 cases
		}
		ev, err := s.S.ListEvents(ctx, c.ID, 0)
		if err != nil {
			return nil, err
		}
		entered := map[string]time.Time{}
		for _, e := range ev {
			switch e.Type {
			case "enter":
				entered[e.Token+"|"+e.Node] = e.At
			case "leave":
				if t0, ok := entered[e.Token+"|"+e.Node]; ok && !e.At.Before(since) {
					durations[e.Node] = append(durations[e.Node], e.At.Sub(t0).Hours())
				}
			}
		}
	}
	out := &ProcessStats{VersionID: versionID, BPMN: v.BPMN, Cases: len(cs)}
	for id, st := range stats {
		d := durations[id]
		st.Passed = len(d)
		if len(d) > 0 {
			sum := 0.0
			for _, x := range d {
				sum += x
			}
			st.AvgHours = sum / float64(len(d))
			st.P90Hours = pct(append([]float64(nil), d...), .9)
		}
		st.Overdue = overdue[id]
		out.Nodes = append(out.Nodes, *st)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].Active > out.Nodes[j].Active })
	return out, nil
}

// ExplorerFilter selects cases for the case explorer.
type ExplorerFilter struct {
	Process, Version, Node, Stage, Status, Assignee, SLA, Q string
	Limit                                                   int
}

type ExplorerRow struct {
	*store.Case
	Steps    []string   `json:"steps"`    // where the case waits now (step names)
	Assignee string     `json:"assignee"` // current assignee(s)
	DueAt    *time.Time `json:"dueAt,omitempty"`
	Overdue  bool       `json:"overdue"`
	Incident bool       `json:"incident"`
}

func (s *Service) Explore(ctx context.Context, by Actor, f ExplorerFilter) ([]ExplorerRow, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	cs, err := s.caseScope(ctx, f.Process, f.Version)
	if err != nil {
		return nil, err
	}
	work, _ := s.S.ListWork(ctx, store.WorkFilter{Status: []store.WorkStatus{store.WorkOpen, store.WorkClaimed}})
	byCase := map[string][]*store.Work{}
	for _, w := range work {
		byCase[w.CaseID] = append(byCase[w.CaseID], w)
	}
	now := s.Now()
	q := strings.ToLower(f.Q)
	var out []ExplorerRow
	for _, c := range cs {
		if f.Status != "" && c.Status != f.Status || f.Stage != "" && c.Stage != f.Stage {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(c.Number+" "+c.Title), q) {
			continue
		}
		row := ExplorerRow{Case: c}
		atNode := false
		for _, t := range c.Instance.Tokens {
			if t.Node == f.Node {
				atNode = true
			}
			if t.Wait == engine.WaitIncident {
				row.Incident = true
			}
		}
		if f.Node != "" && !atNode {
			continue
		}
		var who []string
		for _, w := range byCase[c.ID] {
			row.Steps = append(row.Steps, w.Name)
			if w.Assignee != "" {
				who = append(who, w.Assignee)
			}
			if w.DueAt != nil && (row.DueAt == nil || w.DueAt.Before(*row.DueAt)) {
				d := *w.DueAt
				row.DueAt = &d
			}
		}
		row.Assignee = strings.Join(who, ", ")
		row.Overdue = row.DueAt != nil && row.DueAt.Before(now)
		if f.Assignee != "" && !contains(who, f.Assignee) {
			continue
		}
		if f.SLA == "overdue" && !row.Overdue || f.SLA == "incident" && !row.Incident {
			continue
		}
		out = append(out, row)
		if f.Limit > 0 && len(out) >= f.Limit {
			break
		}
	}
	return out, nil
}

// Inspection is everything the process inspector shows about one case.
type Inspection struct {
	Case      *store.Case          `json:"case"`
	Version   *store.Version       `json:"version"`
	Work      []*store.Work        `json:"work"`
	Jobs      []*store.JobRec      `json:"jobs"`
	Timers    []*engine.Timer      `json:"timers"`
	Incidents []*store.IncidentRec `json:"incidents"`
	Events    []*store.EventRec    `json:"events"`
	Steps     map[string]string    `json:"steps"` // node id -> name
}

func (s *Service) Inspect(ctx context.Context, by Actor, caseID string) (*Inspection, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	c, err := s.S.GetCase(ctx, caseID)
	if err != nil {
		return nil, err
	}
	in := &Inspection{Case: c, Steps: map[string]string{}}
	in.Version, _ = s.S.GetVersion(ctx, c.VersionID)
	if p, _, err := s.Process(ctx, c.VersionID); err == nil {
		for id, n := range p.Nodes {
			in.Steps[id] = firstNonEmpty(n.Name, id)
		}
	}
	in.Work, _ = s.S.ListWork(ctx, store.WorkFilter{CaseID: c.ID})
	in.Jobs, _ = s.S.ListJobs(ctx, c.ID)
	in.Timers, _ = s.S.ListTimers(ctx, c.ID)
	in.Incidents, _ = s.S.ListIncidents(ctx, "", c.ID)
	in.Events, _ = s.S.ListEvents(ctx, c.ID, 500)
	return in, nil
}

// SetVars lets a supervisor correct case data (with a reason, audited).
func (s *Service) SetVars(ctx context.Context, by Actor, caseID string, vars map[string]any, reason string) (*store.Case, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(reason) == "" {
		return nil, invalid("a reason is required to change case data")
	}
	return s.withCase(ctx, caseID, func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error) {
		before := map[string]any{}
		for k, v := range vars {
			before[k] = c.Instance.Vars[k]
			if v == nil {
				delete(c.Instance.Vars, k)
			} else {
				c.Instance.Vars[k] = v
			}
		}
		s.audit(ctx, by, "case.vars", "case", c.ID, map[string]any{"before": before, "after": vars, "reason": reason})
		fx, _, err := s.E.Signal(ctx, p, c.Instance, "data.changed", nil) // re-check conditional events
		return fx, err
	}, by)
}

// Skip completes the step a token waits on without doing it (supervisor).
func (s *Service) Skip(ctx context.Context, by Actor, caseID, tokenID, reason string) (*store.Case, error) {
	if !by.Has(RoleSupervisor) {
		return nil, ErrForbidden
	}
	return s.withCase(ctx, caseID, func(ctx context.Context, c *store.Case, p *bpmn.Process) (*engine.Effects, error) {
		var t *engine.Token
		for _, x := range c.Instance.Tokens {
			if x.ID == tokenID {
				t = x
			}
		}
		if t == nil {
			return nil, engine.ErrNotWaiting
		}
		s.audit(ctx, by, "case.skip", "case", c.ID, map[string]any{"token": tokenID, "node": t.Node, "reason": reason})
		switch t.Wait {
		case engine.WaitUser:
			ws, _ := s.S.ListWork(ctx, store.WorkFilter{CaseID: c.ID, Status: []store.WorkStatus{store.WorkOpen, store.WorkClaimed}})
			for _, w := range ws {
				if w.TokenID == tokenID {
					now := s.Now()
					w.Status, w.CompletedBy, w.CompletedAt = store.WorkDone, by.ID+" (skipped)", &now
					_ = s.S.PutWork(ctx, w)
				}
			}
			return s.E.CompleteTask(ctx, p, c.Instance, tokenID, by.ID, nil)
		case engine.WaitJob:
			return s.E.CompleteJob(ctx, p, c.Instance, tokenID, nil)
		}
		// timers, messages, incidents: continue after the step
		next := ""
		if n := p.Nodes[t.Node]; n != nil && len(n.Outgoing) > 0 {
			next = p.Flows[n.Outgoing[0]].Target
		}
		if next == "" {
			return nil, invalid("this step has nothing after it")
		}
		return s.E.MoveToken(ctx, p, c.Instance, tokenID, next, by.ID)
	}, by)
}

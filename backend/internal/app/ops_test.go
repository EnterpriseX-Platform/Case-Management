package app_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/engine"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/id"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
	"github.com/EnterpriseX-Platform/Case-Management/backend/migrations"
)

// newStore returns a Postgres store (isolated tenant) when
// CM_TEST_DATABASE_URL is set, otherwise the in-memory store.
func newStore(t testing.TB) store.Store {
	url := os.Getenv("CM_TEST_DATABASE_URL")
	if url == "" {
		return store.NewMemory()
	}
	ctx := context.Background()
	pg, err := store.NewPostgres(ctx, url, "t-"+id.New())
	if err != nil {
		t.Fatal(err)
	}
	if err := pg.Migrate(ctx, migrations.All()...); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	return pg
}

// flaky is a connector that fails the first `fail` calls.
type flaky struct {
	fail  int32
	calls atomic.Int32
	err   error
}

func (f *flaky) Service(ctx context.Context, n *bpmn.Node, v map[string]any) (map[string]any, error) {
	if f.calls.Add(1) <= f.fail {
		return nil, f.err
	}
	return map[string]any{"remoteId": "R-1", "key": app.IdempotencyKey(ctx)}, nil
}
func (f *flaky) Rule(ctx context.Context, n *bpmn.Node, v map[string]any) (map[string]any, error) {
	return f.Service(ctx, n, v)
}

const opsFlow = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:cm="http://enterprisex.io/schema/cm/1.0">
<process id="ops"><startEvent id="s"/><userTask id="fill" cm:role="clerk"/>
<serviceTask id="sync" name="Send to ERP" cm:url="http://erp.example/api" cm:retries="3"/>
<boundaryEvent id="rejected" attachedToRef="sync"><errorEventDefinition errorRef="REJECTED"/></boundaryEvent>
<receiveTask id="wait" name="paid"/><userTask id="fix" cm:role="clerk"/><endEvent id="e"/>
<sequenceFlow id="1" sourceRef="s" targetRef="fill"/><sequenceFlow id="2" sourceRef="fill" targetRef="sync"/>
<sequenceFlow id="3" sourceRef="sync" targetRef="wait"/><sequenceFlow id="4" sourceRef="wait" targetRef="e"/>
<sequenceFlow id="5" sourceRef="rejected" targetRef="fix"/><sequenceFlow id="6" sourceRef="fix" targetRef="e"/>
</process></definitions>`

var clerk = app.Actor{ID: "c1", Roles: []string{"clerk"}}

func opsSetup(t *testing.T, h *flaky) (*app.Service, context.Context, *time.Time) {
	ctx := context.Background()
	s := app.New(newStore(t), h, []byte("x"))
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	s.E.Now = s.Now
	if _, err := s.CreateProcess(ctx, designer, store.ProcessDef{Key: "ops", Name: "Ops"}); err != nil {
		t.Fatal(err)
	}
	v, _, err := s.CreateDraft(ctx, designer, "ops", opsFlow, "", 2027)
	if err != nil {
		t.Fatal(err)
	}
	publish(t, s, ctx, v.ID)
	return s, ctx, &now
}

func startAndFill(t *testing.T, s *app.Service, ctx context.Context) *store.Case {
	c, err := s.StartCase(ctx, clerk, app.StartInput{ProcessKey: "ops", FiscalYear: 2027})
	if err != nil {
		t.Fatal(err)
	}
	w := inbox(t, s, ctx, clerk, c.ID)[0]
	if _, err := s.Complete(ctx, clerk, w.ID, map[string]any{"amount": 10.0}); err != nil {
		t.Fatal(err)
	}
	return c
}

func tokenAt(t *testing.T, s *app.Service, ctx context.Context, caseID, node string) *engine.Token {
	c, err := s.S.GetCase(ctx, caseID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range c.Instance.Tokens {
		if tk.Node == node {
			return tk
		}
	}
	return nil
}

// Connector fails twice, succeeds on the 3rd attempt with backoff; the
// idempotency key is the same on every attempt.
func TestJobRetryWithBackoff(t *testing.T) {
	h := &flaky{fail: 2, err: errors.New("503")}
	s, ctx, now := opsSetup(t, h)
	c := startAndFill(t, s, ctx)
	for i := 0; i < 3; i++ {
		if _, err := s.RunJobs(ctx, "w1", 10); err != nil {
			t.Fatal(err)
		}
		*now = now.Add(10 * time.Second) // past the backoff
	}
	tk := tokenAt(t, s, ctx, c.ID, "wait")
	c, _ = s.S.GetCase(ctx, c.ID)
	if tk == nil || h.calls.Load() != 3 || c.Instance.Vars["remoteId"] != "R-1" || c.Instance.Vars["key"] == "" {
		t.Fatalf("calls=%d vars=%v token=%v", h.calls.Load(), c.Instance.Vars, tk)
	}
	// message arrives through the subscription index
	n, err := s.Message(ctx, clerk, "", "paid", "", nil)
	if err != nil || n != 1 {
		t.Fatalf("message: %d %v", n, err)
	}
	if c, _ = s.S.GetCase(ctx, c.ID); c.Status != "completed" {
		t.Fatal(c.Status)
	}
	if ev, _ := s.S.ListEvents(ctx, c.ID, 0); len(ev) < 8 {
		t.Fatalf("event log has %d entries", len(ev))
	}
}

// Connector keeps failing: after max attempts an incident is raised, the
// case stays running, and an operator retry completes it.
func TestIncidentAndRetry(t *testing.T) {
	h := &flaky{fail: 3, err: errors.New("connection refused")}
	s, ctx, now := opsSetup(t, h)
	c := startAndFill(t, s, ctx)
	for i := 0; i < 4; i++ {
		s.RunJobs(ctx, "w1", 10)
		*now = now.Add(time.Minute)
	}
	inc, _ := s.S.ListIncidents(ctx, store.IncidentOpen, c.ID)
	c, _ = s.S.GetCase(ctx, c.ID)
	if len(inc) != 1 || c.Status != "running" || inc[0].NodeName != "Send to ERP" {
		t.Fatalf("incident: %+v status=%s", inc, c.Status)
	}
	if _, err := s.ResolveIncident(ctx, clerk, inc[0].ID, "retry", "", nil, ""); err == nil {
		t.Fatal("clerk must not resolve incidents")
	}
	if _, err := s.ResolveIncident(ctx, boss, inc[0].ID, "retry", "", nil, "ERP is back"); err != nil {
		t.Fatal(err)
	}
	s.RunJobs(ctx, "w1", 10)
	if tokenAt(t, s, ctx, c.ID, "wait") == nil {
		t.Fatal("retry did not continue the case")
	}
	if open, _ := s.S.ListIncidents(ctx, store.IncidentOpen, c.ID); len(open) != 0 {
		t.Fatal("incident still open")
	}
}

// A business rejection from the connector follows the modelled error path.
func TestConnectorBusinessError(t *testing.T) {
	h := &flaky{fail: 1, err: &engine.BPMNError{Code: "REJECTED"}}
	s, ctx, _ := opsSetup(t, h)
	c := startAndFill(t, s, ctx)
	s.RunJobs(ctx, "w1", 10)
	if tokenAt(t, s, ctx, c.ID, "fix") == nil || h.calls.Load() != 1 {
		t.Fatal("BPMN error not routed to boundary")
	}
}

// A worker leases a job and dies; after the lease expires another worker
// takes it over and the case continues exactly once.
func TestWorkerCrashLeaseTakeover(t *testing.T) {
	h := &flaky{}
	s, ctx, now := opsSetup(t, h)
	c := startAndFill(t, s, ctx)
	leased, err := s.S.LeaseJobs(ctx, "dead-worker", nil, *now, 2*time.Minute, 10)
	if err != nil || len(leased) != 1 {
		t.Fatalf("lease: %v %d", err, len(leased))
	}
	if n, _ := s.RunJobs(ctx, "w2", 10); n != 0 {
		t.Fatal("job taken while lease is valid")
	}
	*now = now.Add(3 * time.Minute)
	if n, _ := s.RunJobs(ctx, "w2", 10); n != 1 {
		t.Fatal("expired lease not taken over")
	}
	if tokenAt(t, s, ctx, c.ID, "wait") == nil || h.calls.Load() != 1 {
		t.Fatalf("calls=%d", h.calls.Load())
	}
}

// Many users submit the same work item at once: exactly one wins.
func TestConcurrentCompleteSameWork(t *testing.T) {
	s, ctx, _ := opsSetup(t, &flaky{})
	c, _ := s.StartCase(ctx, clerk, app.StartInput{ProcessKey: "ops", FiscalYear: 2027})
	w := inbox(t, s, ctx, clerk, c.ID)[0]
	var ok, conflicts atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Complete(ctx, clerk, w.ID, map[string]any{"n": 1.0}); err == nil {
				ok.Add(1)
			} else if errors.Is(err, app.ErrConflict) {
				conflicts.Add(1)
			} else {
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || conflicts.Load() != 49 {
		t.Fatalf("ok=%d conflicts=%d", ok.Load(), conflicts.Load())
	}
	jobs, _ := s.S.ListJobs(ctx, c.ID)
	if len(jobs) != 1 {
		t.Fatalf("expected exactly one job, got %d", len(jobs))
	}
}

// Many different cases progressing in parallel never lose updates.
func TestParallelCasesThroughput(t *testing.T) {
	s, ctx, _ := opsSetup(t, &flaky{})
	const N = 200
	var wg sync.WaitGroup
	errs := make(chan error, N)
	start := time.Now()
	steps0 := app.Metrics.Steps.Load()
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := s.StartCase(ctx, clerk, app.StartInput{ProcessKey: "ops", FiscalYear: 2027, Title: fmt.Sprint(i)})
			if err != nil {
				errs <- err
				return
			}
			ws, _ := s.S.ListWork(ctx, store.WorkFilter{CaseID: c.ID})
			if _, err := s.Complete(ctx, clerk, ws[0].ID, nil); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	for {
		n, err := s.RunJobs(ctx, "w", 100)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	cs, _ := s.S.ListCases(ctx, "ops", "", 0)
	waiting := 0
	for _, c := range cs {
		for _, tk := range c.Instance.Tokens {
			if tk.Node == "wait" {
				waiting++
			}
		}
	}
	if waiting != N {
		t.Fatalf("%d of %d cases reached the wait step", waiting, N)
	}
	el := time.Since(start)
	steps := app.Metrics.Steps.Load() - steps0
	t.Logf("%d cases × (start + complete + job) in %v · %d steps · %.0f steps/s", N, el, steps, float64(steps)/el.Seconds())
}

package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/bpmn"
)

const hdr = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:cm="http://enterprisex.io/schema/cm/1.0"><process id="p">`
const ftr = `</process></definitions>`

func proc(t *testing.T, body string) *bpmn.Process {
	t.Helper()
	ps, err := bpmn.ParseString(hdr + body + ftr)
	if err != nil {
		t.Fatal(err)
	}
	return ps[0]
}

func eng() *Engine {
	n := 0
	e := New(nil, func() string { n++; return fmt.Sprintf("w%d", n) })
	e.Now = func() time.Time { return time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) }
	return e
}

func flow(id, s, t string) string {
	return fmt.Sprintf(`<sequenceFlow id="%s" sourceRef="%s" targetRef="%s"/>`, id, s, t)
}
func cflow(id, s, t, c string) string {
	return fmt.Sprintf(`<sequenceFlow id="%s" sourceRef="%s" targetRef="%s"><conditionExpression>%s</conditionExpression></sequenceFlow>`, id, s, t, c)
}

func waiting(in *Instance, node string) *Token {
	for _, t := range in.Tokens {
		if t.Node == node {
			return t
		}
	}
	return nil
}

func TestExclusiveAndUserTask(t *testing.T) {
	p := proc(t, `<startEvent id="s"/><userTask id="review" name="พิจารณา" cm:role="analyst" cm:form="review-form" cm:sla="P10D"/>
	<exclusiveGateway id="g" default="f4"/><endEvent id="ok"/><userTask id="fix" name="แก้ไข"/>`+
		flow("f1", "s", "review")+flow("f2", "review", "g")+cflow("f3", "g", "ok", "${approved == true}")+
		flow("f4", "g", "fix")+flow("f5", "fix", "review"))
	e := eng()
	ctx := context.Background()
	in, fx, err := e.Start(ctx, p, "i1", "v1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fx.NewWork) != 1 || fx.NewWork[0].Role != "analyst" || fx.NewWork[0].FormKey != "review-form" || fx.NewWork[0].DueAt == nil {
		t.Fatalf("work item: %+v", fx.NewWork)
	}
	tok := waiting(in, "review")
	if _, err := e.CompleteTask(ctx, p, in, tok.ID, "u1", map[string]any{"approved": false}); err != nil {
		t.Fatal(err)
	}
	if waiting(in, "fix") == nil {
		t.Fatal("expected fix step")
	}
	e.CompleteTask(ctx, p, in, waiting(in, "fix").ID, "u1", nil)
	e.CompleteTask(ctx, p, in, waiting(in, "review").ID, "u1", map[string]any{"approved": true})
	if in.Status != Completed {
		t.Fatalf("status %s", in.Status)
	}
}

func TestParallelJoin(t *testing.T) {
	p := proc(t, `<startEvent id="s"/><parallelGateway id="fork"/><userTask id="a"/><userTask id="b"/><parallelGateway id="join"/><endEvent id="e"/>`+
		flow("1", "s", "fork")+flow("2", "fork", "a")+flow("3", "fork", "b")+flow("4", "a", "join")+flow("5", "b", "join")+flow("6", "join", "e"))
	e, ctx := eng(), context.Background()
	in, _, _ := e.Start(ctx, p, "i", "v", "", nil)
	e.CompleteTask(ctx, p, in, waiting(in, "a").ID, "u", nil)
	if in.Status == Completed {
		t.Fatal("joined too early")
	}
	e.CompleteTask(ctx, p, in, waiting(in, "b").ID, "u", nil)
	if in.Status != Completed {
		t.Fatalf("status %s tokens %+v", in.Status, in.Tokens)
	}
}

func TestInclusive(t *testing.T) {
	p := proc(t, `<startEvent id="s"/><inclusiveGateway id="split"/><userTask id="a"/><userTask id="b"/><inclusiveGateway id="join"/><endEvent id="e"/>`+
		flow("1", "s", "split")+cflow("2", "split", "a", "x > 1")+cflow("3", "split", "b", "x > 5")+flow("4", "a", "join")+flow("5", "b", "join")+flow("6", "join", "e"))
	e, ctx := eng(), context.Background()
	in, _, _ := e.Start(ctx, p, "i", "v", "", map[string]any{"x": 3})
	if waiting(in, "b") != nil {
		t.Fatal("b should not be active")
	}
	e.CompleteTask(ctx, p, in, waiting(in, "a").ID, "u", nil)
	if in.Status != Completed {
		t.Fatalf("status %s", in.Status)
	}
}

func TestMultiInstancePerDepartment(t *testing.T) {
	p := proc(t, `<startEvent id="s"/><userTask id="req" name="ยื่นคำขอ"><multiInstanceLoopCharacteristics cm:collection="departments" cm:elementVariable="dept"/></userTask><endEvent id="e"/>`+
		flow("1", "s", "req")+flow("2", "req", "e"))
	e, ctx := eng(), context.Background()
	in, fx, err := e.Start(ctx, p, "i", "v", "", map[string]any{"departments": []any{"A", "B", "C"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(fx.NewWork) != 3 || fx.NewWork[1].Input["dept"] != "B" {
		t.Fatalf("work %+v", fx.NewWork)
	}
	for _, w := range fx.NewWork {
		e.CompleteTask(ctx, p, in, w.TokenID, "u", nil)
	}
	if in.Status != Completed {
		t.Fatalf("status %s", in.Status)
	}
}

func TestSubProcessBoundaryTimerAndError(t *testing.T) {
	p := proc(t, `<startEvent id="s"/>
	<subProcess id="sp"><startEvent id="ss"/><userTask id="in"/><endEvent id="err"><errorEventDefinition errorRef="OVER"/></endEvent>`+
		flow("a", "ss", "in")+flow("b", "in", "err")+`</subProcess>
	<boundaryEvent id="late" attachedToRef="sp"><timerEventDefinition><timeDuration>P5D</timeDuration></timerEventDefinition></boundaryEvent>
	<boundaryEvent id="caught" attachedToRef="sp"><errorEventDefinition errorRef="OVER"/></boundaryEvent>
	<endEvent id="done"/><userTask id="escalate"/><userTask id="handle"/>`+
		flow("1", "s", "sp")+flow("2", "sp", "done")+flow("3", "late", "escalate")+flow("4", "escalate", "done")+
		flow("5", "caught", "handle")+flow("6", "handle", "done"))
	e, ctx := eng(), context.Background()

	in, fx, _ := e.Start(ctx, p, "i", "v", "", nil)
	if len(fx.NewTimers) != 1 {
		t.Fatalf("timers %+v", fx.NewTimers)
	}
	fx2, err := e.FireTimer(ctx, p, in, fx.NewTimers[0])
	if err != nil || waiting(in, "escalate") == nil || waiting(in, "in") != nil {
		t.Fatalf("timer boundary: %v %+v %+v", err, in.Tokens, fx2)
	}

	in2, _, _ := e.Start(ctx, p, "i2", "v", "", nil)
	e.CompleteTask(ctx, p, in2, waiting(in2, "in").ID, "u", nil)
	if waiting(in2, "handle") == nil || in2.Status != Running {
		t.Fatalf("error boundary: %s %+v", in2.Status, in2.Tokens)
	}
}

func TestEventGatewayMessageAndCallActivity(t *testing.T) {
	p := proc(t, `<startEvent id="s"/><eventBasedGateway id="g"/>
	<intermediateCatchEvent id="msg" name="approved"><messageEventDefinition/></intermediateCatchEvent>
	<intermediateCatchEvent id="tmo"><timerEventDefinition><timeDuration>P30D</timeDuration></timerEventDefinition></intermediateCatchEvent>
	<callActivity id="call" calledElement="fulfilment"/><endEvent id="e"/><endEvent id="e2"/>`+
		flow("1", "s", "g")+flow("2", "g", "msg")+flow("3", "g", "tmo")+flow("4", "msg", "call")+flow("5", "call", "e")+flow("6", "tmo", "e2"))
	e, ctx := eng(), context.Background()
	in, fx, _ := e.Start(ctx, p, "i", "v", "", nil)
	if len(fx.NewTimers) != 1 {
		t.Fatal("event gateway timer not armed")
	}
	fx, n, err := e.Message(ctx, p, in, "approved", map[string]any{"approvedAmount": 100})
	if err != nil || n != 1 || len(fx.StartChildren) != 1 || fx.StartChildren[0].ProcessKey != "fulfilment" {
		t.Fatalf("message: %v %d %+v", err, n, fx)
	}
	if _, err := e.ChildCompleted(ctx, p, in, fx.StartChildren[0].ParentToken, nil); err != nil {
		t.Fatal(err)
	}
	if in.Status != Completed {
		t.Fatalf("status %s", in.Status)
	}
}

func TestScriptAndMove(t *testing.T) {
	p := proc(t, `<startEvent id="s"/><scriptTask id="calc"><script>total = base * 2; big = total > 10</script></scriptTask><userTask id="a"/><userTask id="b"/><endEvent id="e"/>`+
		flow("1", "s", "calc")+flow("2", "calc", "a")+flow("3", "a", "b")+flow("4", "b", "e"))
	e, ctx := eng(), context.Background()
	in, _, _ := e.Start(ctx, p, "i", "v", "", map[string]any{"base": 7})
	if in.Vars["total"] != 14 || in.Vars["big"] != true {
		t.Fatalf("vars %+v", in.Vars)
	}
	if _, err := e.MoveToken(ctx, p, in, waiting(in, "a").ID, "b", "supervisor"); err != nil {
		t.Fatal(err)
	}
	if waiting(in, "b") == nil {
		t.Fatal("not moved")
	}
}

func TestDuration(t *testing.T) {
	d, err := ParseDuration("P1DT2H30M")
	if err != nil || d != 26*time.Hour+30*time.Minute {
		t.Fatal(d, err)
	}
}

func TestJobsAndIncidents(t *testing.T) {
	p := proc(t, `<startEvent id="s"/><serviceTask id="call" cm:url="http://x"/>
	<boundaryEvent id="rej" attachedToRef="call"><errorEventDefinition errorRef="REJECTED"/></boundaryEvent>
	<scriptTask id="calc"><script>y = 10 / x</script></scriptTask><userTask id="ok"/><userTask id="handle"/><endEvent id="e"/>`+
		flow("1", "s", "call")+flow("2", "call", "calc")+flow("3", "calc", "ok")+flow("4", "ok", "e")+flow("5", "rej", "handle")+flow("6", "handle", "e"))
	e, ctx := eng(), context.Background()

	// service task becomes a job; engine waits
	in, fx, err := e.Start(ctx, p, "i", "v", "", map[string]any{"x": "bad"})
	if err != nil || len(fx.NewJobs) != 1 || waiting(in, "call").Wait != WaitJob {
		t.Fatalf("job not created: %v %+v", err, fx)
	}
	// job succeeds; script fails -> incident, case keeps running
	fx, err = e.CompleteJob(ctx, p, in, fx.NewJobs[0].TokenID, map[string]any{"status": 200})
	if err != nil || in.Status != Running || len(fx.Incidents) != 1 || waiting(in, "calc").Wait != WaitIncident {
		t.Fatalf("incident expected: %v %s %+v", err, in.Status, fx)
	}
	// operator fixes data and retries
	in.Vars["x"] = 2
	if _, err := e.Retry(ctx, p, in, waiting(in, "calc").ID, "ops"); err != nil || waiting(in, "ok") == nil || in.Vars["y"] != 5.0 {
		t.Fatalf("retry: %v %+v", err, in.Vars)
	}

	// business error from the connector is caught by the boundary event
	in2, fx2, _ := e.Start(ctx, p, "i2", "v", "", nil)
	if _, err := e.FailJob(ctx, p, in2, fx2.NewJobs[0].TokenID, &BPMNError{Code: "REJECTED"}); err != nil || waiting(in2, "handle") == nil {
		t.Fatalf("bpmn error not caught: %v %+v", err, in2.Tokens)
	}
	// technical error -> incident on the service task
	in3, fx3, _ := e.Start(ctx, p, "i3", "v", "", nil)
	if _, err := e.FailJob(ctx, p, in3, fx3.NewJobs[0].TokenID, fmt.Errorf("timeout")); err != nil || waiting(in3, "call").Wait != WaitIncident {
		t.Fatalf("technical failure: %v %+v", err, in3.Tokens)
	}
}

func BenchmarkSteps(b *testing.B) {
	body := `<startEvent id="s"/><parallelGateway id="f"/>`
	flows := flow("0", "s", "f")
	for i := 0; i < 10; i++ {
		body += fmt.Sprintf(`<scriptTask id="t%d"><script>v%d = %d * 2</script></scriptTask>`, i, i, i)
		flows += flow(fmt.Sprintf("a%d", i), "f", fmt.Sprintf("t%d", i)) + flow(fmt.Sprintf("b%d", i), fmt.Sprintf("t%d", i), "j")
	}
	ps, _ := bpmn.ParseString(hdr + body + `<parallelGateway id="j"/><endEvent id="e"/>` + flows + flow("z", "j", "e") + ftr)
	e, ctx := eng(), context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := e.Start(ctx, ps[0], "i", "v", "", nil); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.N*24)/b.Elapsed().Seconds(), "steps/s")
}

package api

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
)

// metrics writes counters in the Prometheus text format (no client library).
func metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	m := &app.Metrics
	c := func(name, help string, v int64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, v)
	}
	c("cm_engine_steps_total", "Engine steps recorded in the event log.", m.Steps.Load())
	c("cm_jobs_completed_total", "Jobs completed successfully.", m.JobsDone.Load())
	c("cm_jobs_retried_total", "Job attempts that failed and were rescheduled.", m.JobsRetried.Load())
	c("cm_jobs_failed_total", "Jobs that gave up (BPMN error or incident).", m.JobsFailed.Load())
	c("cm_incidents_total", "Incidents raised.", m.Incidents.Load())
	c("cm_case_conflicts_total", "Optimistic-lock conflicts retried.", m.Conflicts.Load())
	c("cm_timers_fired_total", "Timers fired.", m.TimersFired.Load())
	c("cm_http_requests_total", "HTTP API requests.", httpRequests.Load())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Fprintf(w, "# TYPE cm_go_goroutines gauge\ncm_go_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(w, "# TYPE cm_go_heap_bytes gauge\ncm_go_heap_bytes %d\n", ms.HeapAlloc)
}

// OpsHandler serves health and metrics for worker-only processes.
func OpsHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	m.HandleFunc("GET /metrics", metrics)
	return m
}

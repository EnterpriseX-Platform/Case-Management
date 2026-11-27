// Command server runs the Case Management API and the timer worker.
//
//	DATABASE_URL   postgres://... (empty = in-memory store, dev only)
//	TENANT         tenant id (default "default")
//	ADDR           listen address (default :8080)
//	CALLBACK_SECRET  HMAC secret for external-screen callbacks (required with a database)
//	PUBLIC_URL     public base URL of this API (for callback links)
//	SEED_PACK      path to a solution pack (.zip) imported at start when the store is empty
//	MODE           which services this process runs, comma separated:
//	                 api          HTTP API (scale with users)
//	                 engine       in-process jobs: decision tables (scale with case volume)
//	                 integration  calls to other systems (slow remotes never delay the engine)
//	                 scheduler    timers, SLA and escalations (one active, others stand by)
//	                 notify       e-mail delivery from the outbox
//	               "worker" = engine,integration,scheduler,notify; "all" (default) = everything.
//	               Every service can run any number of replicas.
//	WORKER_ID      worker name used for job leases (default: hostname-pid)
//	OIDC_ISSUER    enable OIDC bearer auth (e.g. https://sso.example/realms/main)
//	OIDC_AUDIENCE  expected audience / client id (optional)
//	OIDC_ROLES_CLAIM  claim holding roles, dotted (default "roles"; Keycloak: realm_access.roles)
//	OIDC_ROLE_PREFIX  only use roles with this prefix (prefix removed)
//	CM_CONNECTOR_ALLOW  hosts studio connectors may call, e.g. "erp.example.com,.internal.example" (empty = any)
//	CM_SECRET_<NAME>    secrets referenced by connectors as ${secret:NAME}
//	LOG_FORMAT     json for one JSON object per line (default text); LOG_LEVEL=debug logs every request
//	RATE_LIMIT     requests per second per user (default 30, burst 2x; 0 = off)
//	DATABASE_READ_URL   read replica for reports and monitoring (optional)
//	CM_ARCHIVE_AFTER_DAYS  move cases closed this many days ago to the archive (0 = off)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/api"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/app"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/connector"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/importer"
	"github.com/EnterpriseX-Platform/Case-Management/backend/internal/store"
	"github.com/EnterpriseX-Platform/Case-Management/backend/migrations"
)

func main() {
	api.SetupLogging()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var st store.Store
	secret := os.Getenv("CALLBACK_SECRET")
	if url := os.Getenv("DATABASE_URL"); url != "" {
		pg, err := store.NewPostgres(ctx, url, os.Getenv("TENANT"))
		if err != nil {
			slog.Error("database", "err", err)
			os.Exit(1)
		}
		if err := pg.Migrate(ctx, migrations.All()...); err != nil {
			slog.Error("migrate", "err", err)
			os.Exit(1)
		}
		if secret == "" {
			slog.Error("CALLBACK_SECRET is required when DATABASE_URL is set")
			os.Exit(1)
		}
		st = pg
	} else {
		slog.Warn("DATABASE_URL not set: using in-memory store (data is lost on restart)")
		st = store.NewMemory()
		if secret == "" {
			secret = "dev-only-secret"
		}
	}

	svc := app.New(st, connector.New(os.Getenv("RULES_URL"), os.Getenv("RULES_TOKEN")), []byte(secret))
	if ru := os.Getenv("DATABASE_READ_URL"); ru != "" {
		if pg, ok := st.(*store.Postgres); ok {
			r, err := pg.Reader(ctx, ru)
			if err != nil {
				slog.Error("read replica", "err", err)
				os.Exit(1)
			}
			svc.R = r
			slog.Info("reports read from the replica")
		}
	}

	if pack := os.Getenv("SEED_PACK"); pack != "" {
		if ps, _ := st.ListProcesses(ctx); len(ps) == 0 {
			if err := seed(ctx, svc, pack); err != nil {
				slog.Error("seed pack", "err", err)
			}
		}
	}

	connector.KeyFunc = app.IdempotencyKey
	roles, err := parseMode(os.Getenv("MODE"))
	if err != nil {
		slog.Error("MODE", "err", err)
		os.Exit(1)
	}
	worker := os.Getenv("WORKER_ID")
	if worker == "" {
		h, _ := os.Hostname()
		worker = fmt.Sprintf("%s-%d", h, os.Getpid())
	}
	var queues []string
	if roles["engine"] {
		queues = append(queues, store.QueueEngine)
	}
	if roles["integration"] {
		queues = append(queues, store.QueueIntegration)
	}
	if len(queues) > 0 {
		go loop(ctx, time.Second, "jobs", func() (int, error) { return svc.RunQueues(ctx, worker, queues, 50) })
	}
	if roles["scheduler"] {
		go schedule(ctx, st, svc)
	}
	if roles["notify"] {
		go loop(ctx, 5*time.Second, "outbox", func() (int, error) { return svc.RunOutbox(ctx, 50) })
	}
	slog.Info("services", "mode", keys(roles), "worker", worker)
	if !roles["api"] {
		// background services still expose health and metrics
		srv := &http.Server{Addr: envOr("ADDR", ":8081"), Handler: api.OpsHandler(), ReadHeaderTimeout: 10 * time.Second}
		go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
		_ = srv.ListenAndServe()
		return
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{Addr: addr, Handler: apiServer(svc).Handler(),
		ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	slog.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}

// apiServer picks the authentication: OIDC bearer tokens when OIDC_ISSUER is
// set (production), otherwise trusted X-User / X-Roles headers (development).
func apiServer(svc *app.Service) *api.Server {
	srv := &api.Server{S: svc, PublicURL: os.Getenv("PUBLIC_URL")}
	if iss := os.Getenv("OIDC_ISSUER"); iss != "" {
		o := api.NewOIDC(iss, os.Getenv("OIDC_AUDIENCE"), os.Getenv("OIDC_ROLES_CLAIM"))
		o.RolePrefix = os.Getenv("OIDC_ROLE_PREFIX")
		srv.Auth = o.Auth
		slog.Info("auth: OIDC", "issuer", iss)
	} else {
		slog.Warn("auth: development headers (set OIDC_ISSUER in production)")
	}
	return srv
}

var allRoles = []string{"api", "engine", "integration", "scheduler", "notify"}

// parseMode turns MODE into the set of services to run.
func parseMode(m string) (map[string]bool, error) {
	out := map[string]bool{}
	if strings.TrimSpace(m) == "" {
		m = "all"
	}
	for _, r := range strings.Split(m, ",") {
		switch r = strings.TrimSpace(r); r {
		case "all":
			for _, x := range allRoles {
				out[x] = true
			}
		case "worker":
			for _, x := range allRoles[1:] {
				out[x] = true
			}
		case "api", "engine", "integration", "scheduler", "notify":
			out[r] = true
		default:
			return nil, fmt.Errorf("unknown service %q (use %s, worker or all)", r, strings.Join(allRoles, ", "))
		}
	}
	return out, nil
}

func keys(m map[string]bool) string {
	var out []string
	for _, r := range allRoles {
		if m[r] {
			out = append(out, r)
		}
	}
	return strings.Join(out, ",")
}

// schedule runs timers and SLA checks on exactly one replica at a time:
// the others wait for the leader lock and take over if the leader dies.
func schedule(ctx context.Context, st store.Store, svc *app.Service) {
	run := func(ctx context.Context) {
		slog.Info("scheduler: active")
		done := make(chan struct{}, 4)
		go func() {
			loop(ctx, 5*time.Second, "timers", func() (int, error) { return svc.RunTimers(ctx) })
			done <- struct{}{}
		}()
		go func() {
			loop(ctx, 30*time.Second, "sla", func() (int, error) { return svc.RunSLA(ctx) })
			done <- struct{}{}
		}()
		go func() {
			if sc, ok := st.(store.Scaler); ok {
				if ms, err := sc.DailyMetrics(ctx, time.Now().AddDate(-2, 0, 0), time.Now(), ""); err == nil && len(ms) == 0 {
					_ = svc.RebuildRollup(ctx) // first start after an upgrade
				}
			}
			loop(ctx, 10*time.Minute, "rollup", func() (int, error) { return svc.RunRollup(ctx) })
			done <- struct{}{}
		}()
		go func() {
			loop(ctx, 15*time.Minute, "archive", func() (int, error) { return svc.RunArchive(ctx) })
			done <- struct{}{}
		}()
		for i := 0; i < 4; i++ {
			<-done
		}
		if ctx.Err() != nil {
			slog.Warn("scheduler: stepped down")
		}
	}
	l, ok := st.(store.Leader)
	if !ok {
		run(ctx)
		return
	}
	for ctx.Err() == nil {
		if err := l.Lead(ctx, "scheduler", run); err != nil && ctx.Err() == nil {
			slog.Error("scheduler: election", "err", err)
			time.Sleep(3 * time.Second)
		}
	}
}

// loop polls fn; when it found work it runs again immediately.
func loop(ctx context.Context, every time.Duration, name string, fn func() (int, error)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		n, err := fn()
		if err != nil {
			slog.Error(name, "err", err)
		}
		if n > 0 && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func seed(ctx context.Context, svc *app.Service, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b, err := importer.Parse(path, data, importer.Options{})
	if err != nil {
		return err
	}
	sys := app.Actor{ID: "system:seed", Roles: []string{app.RoleAdmin}}
	job, err := importer.Commit(ctx, svc, sys, b, path)
	if err != nil {
		return err
	}
	slog.Info("seed pack imported", "summary", job.Summary)
	return nil
}

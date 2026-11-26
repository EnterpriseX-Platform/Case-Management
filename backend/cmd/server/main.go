// Command server runs the Case Management API and the timer worker.
//
//	DATABASE_URL   postgres://... (empty = in-memory store, dev only)
//	TENANT         tenant id (default "default")
//	ADDR           listen address (default :8080)
//	CALLBACK_SECRET  HMAC secret for external-screen callbacks (required with a database)
//	PUBLIC_URL     public base URL of this API (for callback links)
//	SEED_PACK      path to a solution pack (.zip) imported at start when the store is empty
//	MODE           all (default) | api | worker — run API and/or background workers;
//	               scale each independently (any number of replicas of each)
//	WORKER_ID      worker name used for job leases (default: hostname-pid)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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

	if pack := os.Getenv("SEED_PACK"); pack != "" {
		if ps, _ := st.ListProcesses(ctx); len(ps) == 0 {
			if err := seed(ctx, svc, pack); err != nil {
				slog.Error("seed pack", "err", err)
			}
		}
	}

	connector.KeyFunc = app.IdempotencyKey
	mode := os.Getenv("MODE")
	if mode == "" {
		mode = "all"
	}
	if mode == "all" || mode == "worker" {
		worker := os.Getenv("WORKER_ID")
		if worker == "" {
			h, _ := os.Hostname()
			worker = fmt.Sprintf("%s-%d", h, os.Getpid())
		}
		go loop(ctx, time.Second, "jobs", func() (int, error) { return svc.RunJobs(ctx, worker, 50) })
		go loop(ctx, 5*time.Second, "timers", func() (int, error) { return svc.RunTimers(ctx) })
		slog.Info("worker started", "id", worker)
	}
	if mode == "worker" {
		// workers still expose health and metrics
		srv := &http.Server{Addr: envOr("ADDR", ":8081"), Handler: api.OpsHandler(), ReadHeaderTimeout: 10 * time.Second}
		go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
		_ = srv.ListenAndServe()
		return
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{Addr: addr, Handler: (&api.Server{S: svc, PublicURL: os.Getenv("PUBLIC_URL")}).Handler(),
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

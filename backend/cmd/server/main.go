// Command server runs the Case Management API and the timer worker.
//
//	DATABASE_URL   postgres://... (empty = in-memory store, dev only)
//	TENANT         tenant id (default "default")
//	ADDR           listen address (default :8080)
//	CALLBACK_SECRET  HMAC secret for external-screen callbacks (required with a database)
//	PUBLIC_URL     public base URL of this API (for callback links)
//	SEED_PACK      path to a solution pack (.zip) imported at start when the store is empty
package main

import (
	"context"
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
		if err := pg.Migrate(ctx, migrations.Init); err != nil {
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

	go func() { // timer worker
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := svc.RunTimers(ctx); err != nil {
					slog.Error("timers", "err", err)
				} else if n > 0 {
					slog.Info("timers fired", "n", n)
				}
			}
		}
	}()

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

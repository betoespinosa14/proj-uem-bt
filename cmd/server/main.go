// Command server runs the UEM control plane: REST API, live updates, the
// agent gateway, and background jobs.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/uem/uem/internal/server/api"
	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/core"
	"github.com/uem/uem/internal/server/hub"
	"github.com/uem/uem/internal/server/migrations"
	"github.com/uem/uem/internal/server/notify"
	"github.com/uem/uem/internal/server/scheduler"
	"github.com/uem/uem/internal/server/store"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := env("DATABASE_URL", "postgres://uem:uem@localhost:5432/uem?sslmode=disable")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := waitForDB(ctx, pool); err != nil {
		return err
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	if err := migrations.Up(ctx, sqlDB); err != nil {
		return err
	}
	sqlDB.Close()

	st := store.New(pool)
	if err := st.MarkAllOffline(ctx); err != nil {
		return err
	}
	auditLog := audit.New(st)
	hooks := notify.New(st, envBool("UEM_WEBHOOK_ALLOW_PRIVATE", false))
	hooks.Run(ctx, 4)
	auditLog.SetSink(func(ctx context.Context, e store.AuditEntry) {
		hooks.Emit(ctx, e.OrgID, "audit."+e.Action, e)
	})

	c := &core.Core{
		Store: st, Audit: auditLog, Hub: hub.New(), Hooks: hooks,
		TelemetryInterval: 60 * time.Second,
		InventoryInterval: 6 * time.Hour,
	}
	sched := scheduler.New(c)
	if err := sched.Start(ctx); err != nil {
		return err
	}

	publicURL := env("UEM_PUBLIC_URL", "")
	var origins []string
	if o := os.Getenv("UEM_ALLOWED_ORIGINS"); o != "" {
		origins = strings.Split(o, ",")
	}
	srv := api.New(api.Config{
		PublicURL:      publicURL,
		SecureCookies:  envBool("UEM_SECURE_COOKIES", strings.HasPrefix(publicURL, "https://")),
		TrustProxy:     envBool("UEM_TRUST_PROXY", false),
		AgentDistDir:   env("UEM_AGENT_DIST", "dist"),
		OriginPatterns: origins,
		AllowSignup:    envBool("UEM_ALLOW_SIGNUP", true),
	}, c, sched)

	httpSrv := &http.Server{
		Addr:              env("UEM_LISTEN", ":8080"),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	slog.Info("UEM server listening", "addr", httpSrv.Addr)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// healthcheck probes the local server; used by the container HEALTHCHECK
// since the distroless image has no curl/wget.
func healthcheck() int {
	addr := env("UEM_LISTEN", ":8080")
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		return 1
	}
	resp.Body.Close()
	return 0
}

func waitForDB(ctx context.Context, pool *pgxpool.Pool) error {
	var err error
	for range 30 {
		if err = pool.Ping(ctx); err == nil {
			return nil
		}
		slog.Info("waiting for database", "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return err
}

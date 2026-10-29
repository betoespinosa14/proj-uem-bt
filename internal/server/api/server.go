// Package api is the HTTP layer: the admin REST API, the browser live
// WebSocket, and the agent gateway (enrollment + agent WebSocket).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/auth"
	"github.com/uem/uem/internal/server/core"
	"github.com/uem/uem/internal/server/scheduler"
	"github.com/uem/uem/internal/server/store"
)

type Config struct {
	PublicURL      string        // externally reachable base URL, used in install commands
	SecureCookies  bool          // set Secure on cookies (HTTPS deployments)
	TrustProxy     bool          // honor X-Forwarded-For / X-Forwarded-Proto from a reverse proxy
	SessionTTL     time.Duration
	AgentDistDir   string        // directory holding cross-compiled agent binaries
	OriginPatterns []string      // extra allowed Origins for the browser WebSocket (dev)
	AllowSignup    bool          // allow creating new organizations from the login page
}

type Server struct {
	cfg   Config
	core  *core.Core
	store *store.Store
	audit *audit.Log
	sched *scheduler.Scheduler

	loginLimiter  *limiter
	enrollLimiter *limiter
}

func New(cfg Config, c *core.Core, sched *scheduler.Scheduler) *Server {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	return &Server{
		cfg: cfg, core: c, store: c.Store, audit: c.Audit, sched: sched,
		loginLimiter:  newLimiter(10, 15*time.Minute),
		enrollLimiter: newLimiter(30, time.Minute),
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, s.realIP, middleware.Recoverer, securityHeaders)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.store.Pool.Ping(r.Context()); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	r.Get("/install.sh", s.installScript("install.sh", "text/x-shellscript"))
	r.Get("/install.ps1", s.installScript("install.ps1", "text/plain"))
	r.Get("/downloads/{file}", s.download)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.NoCache)

		// Agent gateway: authenticated by enrollment token / device secret.
		r.Post("/agent/enroll", s.agentEnroll)
		r.Get("/agent/ws", s.agentWS)

		r.Post("/auth/signup", s.signup)
		r.Post("/auth/login", s.login)

		r.Group(func(r chi.Router) {
			r.Use(s.requireSession, s.csrf)

			r.Post("/auth/logout", s.logout)
			r.Get("/auth/me", s.me)
			r.Get("/live", s.live)
			r.Get("/dashboard", s.dashboard)

			r.Get("/devices", s.listDevices)
			r.Get("/devices/export.csv", s.exportDevices)
			r.Get("/devices/{id}", s.getDevice)
			r.With(s.perm(auth.PermDevicesWrite)).Patch("/devices/{id}", s.updateDevice)
			r.With(s.perm(auth.PermDevicesWrite)).Delete("/devices/{id}", s.retireDevice)
			r.Get("/devices/{id}/software", s.deviceSoftware)
			r.Get("/devices/{id}/software.csv", s.exportSoftware)
			r.Get("/devices/{id}/usage", s.deviceUsage)
			r.Get("/devices/{id}/usage.csv", s.exportDeviceUsage)
			r.Get("/usage", s.listFleetUsage)
			r.Get("/usage/export.csv", s.exportFleetUsage)
			r.Get("/devices/{id}/telemetry", s.deviceTelemetry)
			r.Get("/devices/{id}/compliance", s.deviceCompliance)
			r.Get("/devices/{id}/commands", s.deviceCommands)
			r.With(s.perm(auth.PermCommandsRun)).Post("/devices/{id}/commands", s.createCommand)
			r.With(s.perm(auth.PermCommandsRun)).Post("/commands/bulk", s.bulkCommand)
			r.Get("/commands/{id}", s.getCommand)

			r.Get("/groups", s.listGroups)
			r.With(s.perm(auth.PermDevicesWrite)).Post("/groups", s.createGroup)
			r.With(s.perm(auth.PermDevicesWrite)).Patch("/groups/{id}", s.updateGroup)
			r.With(s.perm(auth.PermDevicesWrite)).Delete("/groups/{id}", s.deleteGroup)

			r.Get("/enrollment-tokens", s.listTokens)
			r.With(s.perm(auth.PermEnrollmentWrite)).Post("/enrollment-tokens", s.createToken)
			r.With(s.perm(auth.PermEnrollmentWrite)).Delete("/enrollment-tokens/{id}", s.revokeToken)
			r.Get("/enrollment-tokens/{id}/install", s.installCommands)

			r.Get("/scripts", s.listScripts)
			r.Get("/scripts/{id}", s.getScript)
			r.With(s.perm(auth.PermScriptsWrite)).Post("/scripts", s.createScript)
			r.With(s.perm(auth.PermScriptsWrite)).Patch("/scripts/{id}", s.updateScript)
			r.With(s.perm(auth.PermScriptsWrite)).Delete("/scripts/{id}", s.deleteScript)

			r.Get("/compliance/fields", s.complianceFields)
			r.Get("/compliance/rules", s.listRules)
			r.Get("/compliance/rules/{id}", s.getRule)
			r.With(s.perm(auth.PermComplianceWrite)).Post("/compliance/rules", s.createRule)
			r.With(s.perm(auth.PermComplianceWrite)).Patch("/compliance/rules/{id}", s.updateRule)
			r.With(s.perm(auth.PermComplianceWrite)).Delete("/compliance/rules/{id}", s.deleteRule)
			r.With(s.perm(auth.PermComplianceWrite)).Post("/compliance/evaluate", s.evaluateAll)

			r.Get("/tasks", s.listTasks)
			r.With(s.perm(auth.PermTasksWrite)).Post("/tasks", s.createTask)
			r.With(s.perm(auth.PermTasksWrite)).Patch("/tasks/{id}", s.updateTask)
			r.With(s.perm(auth.PermTasksWrite)).Delete("/tasks/{id}", s.deleteTask)
			r.With(s.perm(auth.PermTasksWrite)).Post("/tasks/{id}/run", s.runTask)

			r.Get("/alerts", s.listAlerts)
			r.With(s.perm(auth.PermAlertsWrite)).Post("/alerts/{id}/resolve", s.resolveAlert)

			r.With(s.perm(auth.PermAuditRead)).Get("/audit", s.listAudit)
			r.With(s.perm(auth.PermAuditRead)).Get("/audit/export.csv", s.exportAudit)
			r.With(s.perm(auth.PermAuditRead)).Get("/audit/verify", s.verifyAudit)

			r.With(s.perm(auth.PermUsersWrite)).Get("/users", s.listUsers)
			r.With(s.perm(auth.PermUsersWrite)).Post("/users", s.createUser)
			r.With(s.perm(auth.PermUsersWrite)).Patch("/users/{id}", s.updateUser)
			r.With(s.perm(auth.PermUsersWrite)).Delete("/users/{id}", s.deleteUser)

			r.With(s.perm(auth.PermWebhooksWrite)).Get("/webhooks", s.listWebhooks)
			r.With(s.perm(auth.PermWebhooksWrite)).Post("/webhooks", s.createWebhook)
			r.With(s.perm(auth.PermWebhooksWrite)).Patch("/webhooks/{id}", s.updateWebhook)
			r.With(s.perm(auth.PermWebhooksWrite)).Delete("/webhooks/{id}", s.deleteWebhook)
			r.With(s.perm(auth.PermWebhooksWrite)).Post("/webhooks/{id}/test", s.testWebhook)

			r.Get("/org", s.getOrg)
			r.With(s.perm(auth.PermSettingsWrite)).Patch("/org", s.updateOrg)
		})
		r.NotFound(func(w http.ResponseWriter, r *http.Request) { writeErr(w, http.StatusNotFound, "not found") })
	})
	return r
}

// ---- Request context ----

type ctxKey int

const principalKey ctxKey = iota

type principal struct {
	User    store.User
	Session store.Session
}

func who(r *http.Request) principal {
	return r.Context().Value(principalKey).(principal)
}

func orgOf(r *http.Request) string { return who(r).Session.OrgID }

func (s *Server) actor(r *http.Request) audit.Actor {
	p := who(r)
	return audit.Actor{Type: "user", ID: p.User.ID, Email: p.User.Email, IP: clientIP(r)}
}

// record writes an audit entry attributed to the current user.
func (s *Server) record(r *http.Request, action, targetType, targetID string, details any) {
	s.audit.Record(r.Context(), audit.Event{OrgID: orgOf(r), Actor: s.actor(r), Action: action,
		TargetType: targetType, TargetID: targetID, Details: details})
}

// ---- Middleware ----

const (
	sessionCookie = "uem_session"
	csrfCookie    = "uem_csrf"
)

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeErr(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		sess, user, err := s.store.GetSession(r.Context(), auth.HashToken(c.Value))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "session expired")
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, principal{User: user, Session: sess})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// csrf enforces the double-submit token on state-changing requests.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !auth.ConstantTimeEqual(r.Header.Get("X-CSRF-Token"), who(r).Session.CSRFToken) {
				writeErr(w, http.StatusForbidden, "invalid CSRF token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) perm(p string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !auth.Can(who(r).User.Role, p) {
				writeErr(w, http.StatusForbidden, "you do not have permission to do that")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

type ipKeyT struct{}

// realIP stores the client IP, trusting proxy headers only when configured.
func (s *Server) realIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if s.cfg.TrustProxy {
			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				ip = strings.TrimSpace(strings.Split(xff, ",")[0])
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ipKeyT{}, ip)))
	})
}

func clientIP(r *http.Request) string {
	ip, _ := r.Context().Value(ipKeyT{}).(string)
	return ip
}

// ---- Helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("write response", "err", err)
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func items[T any](w http.ResponseWriter, list []T, err error) {
	if err != nil {
		slog.Error("list query failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

// fail maps domain errors to HTTP responses.
func fail(w http.ResponseWriter, err error) {
	var ve *core.ValidationError
	switch {
	case errors.As(err, &ve):
		writeErr(w, http.StatusBadRequest, ve.Msg)
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrConflict):
		writeErr(w, http.StatusConflict, "a record with that name already exists")
	default:
		slog.Error("request failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
	}
}

func badRequest(msg string) error { return &core.ValidationError{Msg: msg} }

// decode reads a JSON body (max 1 MiB) into v, rejecting unknown fields.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return badRequest("invalid JSON body: " + err.Error())
	}
	return nil
}

func ok(w http.ResponseWriter) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) }

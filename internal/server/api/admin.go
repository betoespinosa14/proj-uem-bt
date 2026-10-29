package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/internal/server/auth"
	"github.com/uem/uem/internal/server/compliance"
	"github.com/uem/uem/internal/server/notify"
	"github.com/uem/uem/internal/server/scheduler"
	"github.com/uem/uem/internal/server/store"
)

// ---- Groups ----

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListGroups(r.Context(), orgOf(r))
	items(w, list, err)
}

type groupRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	BlockedApps []string `json:"blocked_apps"`
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		fail(w, badRequest("name is required"))
		return
	}
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	g, err := s.store.CreateGroup(r.Context(), orgOf(r), strings.TrimSpace(*req.Name), desc, cleanList(req.BlockedApps))
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "group.create", "group", g.ID, g)
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
		if *req.Name == "" {
			fail(w, badRequest("name cannot be empty"))
			return
		}
	}
	if req.BlockedApps != nil {
		req.BlockedApps = cleanList(req.BlockedApps)
	}
	g, err := s.store.UpdateGroup(r.Context(), orgOf(r), chi.URLParam(r, "id"), req.Name, req.Description, req.BlockedApps)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "group.update", "group", g.ID, g)
	if req.BlockedApps != nil {
		s.core.PushGroupPolicy(r.Context(), orgOf(r), g.ID)
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	g, err := s.store.GetGroup(r.Context(), orgOf(r), id)
	if err != nil {
		fail(w, err)
		return
	}
	devices, _ := s.store.ListDevices(r.Context(), orgOf(r), store.DeviceFilter{GroupID: id, Status: "online"})
	if err := s.store.DeleteGroup(r.Context(), orgOf(r), id); err != nil {
		fail(w, err)
		return
	}
	for _, d := range devices {
		s.core.PushPolicy(r.Context(), d.ID)
	}
	s.record(r, "group.delete", "group", id, map[string]any{"name": g.Name})
	ok(w)
}

// ---- Enrollment ----

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListTokens(r.Context(), orgOf(r))
	items(w, list, err)
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description    string  `json:"description"`
		GroupID        *string `json:"group_id"`
		ExpiresInHours *int    `json:"expires_in_hours"`
		MaxUses        *int    `json:"max_uses"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if req.GroupID != nil && *req.GroupID == "" {
		req.GroupID = nil
	}
	if req.GroupID != nil {
		if err := s.store.GroupBelongs(r.Context(), orgOf(r), *req.GroupID); err != nil {
			fail(w, badRequest("group not found"))
			return
		}
	}
	var expires *time.Time
	if req.ExpiresInHours != nil {
		if *req.ExpiresInHours <= 0 || *req.ExpiresInHours > 24*365 {
			fail(w, badRequest("expires_in_hours must be between 1 and 8760"))
			return
		}
		t := time.Now().Add(time.Duration(*req.ExpiresInHours) * time.Hour)
		expires = &t
	}
	if req.MaxUses != nil && *req.MaxUses <= 0 {
		fail(w, badRequest("max_uses must be positive"))
		return
	}
	t, err := s.store.CreateToken(r.Context(), orgOf(r), "enr_"+auth.RandomToken(24), strings.TrimSpace(req.Description),
		req.GroupID, expires, req.MaxUses, who(r).User.ID)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "enrollment_token.create", "enrollment_token", t.ID, map[string]any{
		"description": t.Description, "group_id": t.GroupID, "expires_at": t.ExpiresAt, "max_uses": t.MaxUses})
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.RevokeToken(r.Context(), orgOf(r), id); err != nil {
		fail(w, err)
		return
	}
	s.record(r, "enrollment_token.revoke", "enrollment_token", id, nil)
	ok(w)
}

// publicURL picks the base URL agents should use: org setting, then server
// config, then whatever the browser used to reach us.
func (s *Server) publicURL(r *http.Request, org store.Org) string {
	if org.ServerURL != "" {
		return strings.TrimRight(org.ServerURL, "/")
	}
	if s.cfg.PublicURL != "" {
		return strings.TrimRight(s.cfg.PublicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || (s.cfg.TrustProxy && r.Header.Get("X-Forwarded-Proto") == "https") {
		scheme = "https"
	}
	host := r.Host
	if s.cfg.TrustProxy && r.Header.Get("X-Forwarded-Host") != "" {
		host = r.Header.Get("X-Forwarded-Host")
	}
	return scheme + "://" + host
}

func (s *Server) installCommands(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetToken(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	org, err := s.store.GetOrg(r.Context(), orgOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	base := s.publicURL(r, org)
	writeJSON(w, http.StatusOK, map[string]string{
		"linux": fmt.Sprintf("curl -fsSL %s/install.sh | sudo sh -s -- --server %s --token %s", base, base, t.Token),
		"macos": fmt.Sprintf("curl -fsSL %s/install.sh | sudo sh -s -- --server %s --token %s", base, base, t.Token),
		"windows": fmt.Sprintf(`powershell -ExecutionPolicy Bypass -Command "& ([scriptblock]::Create((irm %s/install.ps1))) -Server '%s' -Token '%s'"`,
			base, base, t.Token),
	})
}

// ---- Scripts ----

func (s *Server) listScripts(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListScripts(r.Context(), orgOf(r))
	items(w, list, err)
}

func (s *Server) getScript(w http.ResponseWriter, r *http.Request) {
	sc, err := s.store.GetScript(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sc)
}

type scriptRequest struct {
	Name           *string `json:"name"`
	Description    *string `json:"description"`
	Shell          *string `json:"shell"`
	Body           *string `json:"body"`
	TimeoutSeconds *int    `json:"timeout_seconds"`
}

func (req scriptRequest) apply(sc *store.Script) error {
	if req.Name != nil {
		sc.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		sc.Description = *req.Description
	}
	if req.Shell != nil {
		sc.Shell = *req.Shell
	}
	if req.Body != nil {
		sc.Body = *req.Body
	}
	if req.TimeoutSeconds != nil {
		sc.TimeoutSeconds = *req.TimeoutSeconds
	}
	switch {
	case sc.Name == "":
		return badRequest("name is required")
	case !slices.Contains([]string{"bash", "sh", "zsh", "powershell", "cmd"}, sc.Shell):
		return badRequest("shell must be one of bash, sh, zsh, powershell, cmd")
	case strings.TrimSpace(sc.Body) == "":
		return badRequest("body is required")
	case len(sc.Body) > 256*1024:
		return badRequest("script body is too large (max 256 KiB)")
	case sc.TimeoutSeconds <= 0 || sc.TimeoutSeconds > 3600:
		return badRequest("timeout_seconds must be between 1 and 3600")
	}
	return nil
}

func (s *Server) createScript(w http.ResponseWriter, r *http.Request) {
	var req scriptRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	sc := store.Script{TimeoutSeconds: 300}
	if err := req.apply(&sc); err != nil {
		fail(w, err)
		return
	}
	out, err := s.store.CreateScript(r.Context(), orgOf(r), sc)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "script.create", "script", out.ID, out)
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updateScript(w http.ResponseWriter, r *http.Request) {
	var req scriptRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	sc, err := s.store.GetScript(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	if err := req.apply(&sc); err != nil {
		fail(w, err)
		return
	}
	out, err := s.store.UpdateScript(r.Context(), orgOf(r), sc)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "script.update", "script", out.ID, out)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteScript(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteScript(r.Context(), orgOf(r), id); err != nil {
		fail(w, err)
		return
	}
	s.record(r, "script.delete", "script", id, nil)
	ok(w)
}

// ---- Compliance ----

func (s *Server) complianceFields(w http.ResponseWriter, r *http.Request) {
	items(w, compliance.Fields, nil)
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListRules(r.Context(), orgOf(r))
	items(w, list, err)
}

func (s *Server) getRule(w http.ResponseWriter, r *http.Request) {
	x, err := s.store.GetRule(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, x)
}

type ruleRequest struct {
	Name        *string            `json:"name"`
	Description *string            `json:"description"`
	Enabled     *bool              `json:"enabled"`
	Severity    *string            `json:"severity"`
	OSFamilies  []string           `json:"os_families"`
	GroupID     json.RawMessage    `json:"group_id"` // absent = unchanged, null or "" = all groups
	Match       *string            `json:"match"`
	Conditions  []store.Condition  `json:"conditions"`
	Actions     *store.RuleActions `json:"actions"`
}

func (s *Server) applyRule(r *http.Request, req ruleRequest, x *store.Rule) error {
	if req.Name != nil {
		x.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		x.Description = *req.Description
	}
	if req.Enabled != nil {
		x.Enabled = *req.Enabled
	}
	if req.Severity != nil {
		x.Severity = *req.Severity
	}
	if req.OSFamilies != nil {
		x.OSFamilies = cleanList(req.OSFamilies)
	}
	if len(req.GroupID) > 0 {
		var g *string
		if err := json.Unmarshal(req.GroupID, &g); err != nil {
			return badRequest("group_id must be a string or null")
		}
		if g != nil && *g == "" {
			g = nil
		}
		x.GroupID = g
	}
	if req.Match != nil {
		x.Match = *req.Match
	}
	if req.Conditions != nil {
		x.Conditions = req.Conditions
	}
	if req.Actions != nil {
		x.Actions = *req.Actions
	}
	switch {
	case x.Name == "":
		return badRequest("name is required")
	case !slices.Contains([]string{"low", "medium", "high", "critical"}, x.Severity):
		return badRequest("severity must be low, medium, high or critical")
	case x.Match != "all" && x.Match != "any":
		return badRequest("match must be all or any")
	}
	if err := compliance.Validate(x.Conditions); err != nil {
		return badRequest(err.Error())
	}
	if x.GroupID != nil {
		if err := s.store.GroupBelongs(r.Context(), orgOf(r), *x.GroupID); err != nil {
			return badRequest("group not found")
		}
	}
	if cmd := x.Actions.Command; cmd != nil {
		if cmd.Type == "" {
			x.Actions.Command = nil
		} else if _, err := s.core.ResolvePayload(r.Context(), orgOf(r), cmd.Type, cmd.Payload); err != nil {
			return badRequest("remediation command: " + err.Error())
		}
	}
	return nil
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	x := store.Rule{Enabled: true, Severity: "medium", Match: "all", Actions: store.RuleActions{Alert: true}}
	if err := s.applyRule(r, req, &x); err != nil {
		fail(w, err)
		return
	}
	out, err := s.store.CreateRule(r.Context(), orgOf(r), x)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "compliance.rule.create", "rule", out.ID, out)
	s.reevaluate(r)
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	x, err := s.store.GetRule(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.applyRule(r, req, &x); err != nil {
		fail(w, err)
		return
	}
	out, err := s.store.UpdateRule(r.Context(), orgOf(r), x)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "compliance.rule.update", "rule", out.ID, out)
	s.reevaluate(r)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	x, err := s.store.GetRule(r.Context(), orgOf(r), id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.DeleteRule(r.Context(), orgOf(r), id); err != nil {
		fail(w, err)
		return
	}
	s.record(r, "compliance.rule.delete", "rule", id, map[string]any{"name": x.Name})
	s.reevaluate(r)
	ok(w)
}

// reevaluate refreshes compliance in the background after a rule change.
func (s *Server) reevaluate(r *http.Request) {
	ctx, orgID := context.WithoutCancel(r.Context()), orgOf(r)
	go func() {
		if _, err := s.core.EvaluateOrg(ctx, orgID); err != nil {
			slog.Error("re-evaluate compliance", "org", orgID, "err", err)
		}
	}()
}

func (s *Server) evaluateAll(w http.ResponseWriter, r *http.Request) {
	n, err := s.core.EvaluateOrg(r.Context(), orgOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "compliance.evaluate", "org", orgOf(r), map[string]any{"devices": n})
	writeJSON(w, http.StatusOK, map[string]int{"evaluated": n})
}

// ---- Scheduled tasks ----

func (s *Server) withNextRun(t store.Task) store.Task {
	if t.Enabled {
		t.NextRunAt = s.sched.NextRun(t.ID)
	}
	return t
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListTasks(r.Context(), orgOf(r))
	for i := range list {
		list[i] = s.withNextRun(list[i])
	}
	items(w, list, err)
}

type taskRequest struct {
	Name    *string           `json:"name"`
	Enabled *bool             `json:"enabled"`
	Cron    *string           `json:"cron"`
	Target  *store.TaskTarget `json:"target"`
	Action  *store.TaskAction `json:"action"`
}

func (s *Server) applyTask(r *http.Request, req taskRequest, t *store.Task) error {
	if req.Name != nil {
		t.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		t.Enabled = *req.Enabled
	}
	if req.Cron != nil {
		t.Cron = strings.TrimSpace(*req.Cron)
	}
	if req.Target != nil {
		t.Target = *req.Target
	}
	if req.Action != nil {
		t.Action = *req.Action
	}
	if t.Target.DeviceIDs == nil {
		t.Target.DeviceIDs = []string{}
	}
	if t.Name == "" {
		return badRequest("name is required")
	}
	if err := scheduler.ValidateCron(t.Cron); err != nil {
		return badRequest(err.Error())
	}
	switch {
	case t.Target.All:
	case t.Target.GroupID != nil && *t.Target.GroupID != "":
		if err := s.store.GroupBelongs(r.Context(), orgOf(r), *t.Target.GroupID); err != nil {
			return badRequest("target group not found")
		}
	case len(t.Target.DeviceIDs) > 0:
		found, err := s.store.ListDevices(r.Context(), orgOf(r), store.DeviceFilter{IDs: t.Target.DeviceIDs})
		if err != nil {
			return err
		}
		if len(found) != len(cleanList(t.Target.DeviceIDs)) {
			return badRequest("one or more target devices were not found")
		}
	default:
		return badRequest("target must be all devices, a group, or a list of devices")
	}
	if t.Action.Type != scheduler.ActionComplianceCheck {
		payload, err := s.core.ResolvePayload(r.Context(), orgOf(r), t.Action.Type, t.Action.Payload)
		if err != nil {
			return err
		}
		// Keep the script_id reference (not the inlined body) so edits to a
		// library script apply to future runs; validation above proved it resolves.
		if len(t.Action.Payload) == 0 {
			t.Action.Payload = payload
		}
	} else {
		t.Action.Payload = []byte(`{}`)
	}
	return nil
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var req taskRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	t := store.Task{Enabled: true}
	if err := s.applyTask(r, req, &t); err != nil {
		fail(w, err)
		return
	}
	out, err := s.store.CreateTask(r.Context(), orgOf(r), t)
	if err != nil {
		fail(w, err)
		return
	}
	_ = s.sched.Reload(r.Context())
	s.record(r, "task.create", "task", out.ID, out)
	writeJSON(w, http.StatusCreated, s.withNextRun(out))
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	var req taskRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	t, err := s.store.GetTask(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.applyTask(r, req, &t); err != nil {
		fail(w, err)
		return
	}
	out, err := s.store.UpdateTask(r.Context(), orgOf(r), t)
	if err != nil {
		fail(w, err)
		return
	}
	_ = s.sched.Reload(r.Context())
	s.record(r, "task.update", "task", out.ID, out)
	writeJSON(w, http.StatusOK, s.withNextRun(out))
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteTask(r.Context(), orgOf(r), id); err != nil {
		fail(w, err)
		return
	}
	_ = s.sched.Reload(r.Context())
	s.record(r, "task.delete", "task", id, nil)
	ok(w)
}

func (s *Server) runTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetTask(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	n, err := s.sched.Run(r.Context(), t, s.actor(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"devices": n})
}

// ---- Alerts ----

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "open" && status != "resolved" {
		status = ""
	}
	list, err := s.store.ListAlerts(r.Context(), orgOf(r), status, 500)
	items(w, list, err)
}

func (s *Server) resolveAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.ResolveAlert(r.Context(), orgOf(r), id); err != nil {
		fail(w, err)
		return
	}
	s.record(r, "alert.resolve", "alert", id, nil)
	ok(w)
}

// ---- Audit ----

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	list, err := s.store.ListAudit(r.Context(), orgOf(r), store.AuditFilter{
		Action: q.Get("action"), Actor: q.Get("actor"), TargetID: q.Get("target_id"), BeforeID: before, Limit: limit,
	})
	if err != nil {
		fail(w, err)
		return
	}
	var next *int64
	if len(list) == limit {
		id := list[len(list)-1].ID
		next = &id
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "next_before_id": next})
}

func (s *Server) exportAudit(w http.ResponseWriter, r *http.Request) {
	s.record(r, "audit.export", "org", orgOf(r), nil)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="audit-log-%s.csv"`, time.Now().UTC().Format("20060102-150405")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "timestamp", "actor_type", "actor_id", "actor_email", "ip", "action", "target_type", "target_id", "details", "hash"})
	err := s.store.EachAudit(r.Context(), orgOf(r), func(e store.AuditEntry) error {
		return cw.Write([]string{
			strconv.FormatInt(e.ID, 10), e.TS.UTC().Format(time.RFC3339Nano), e.ActorType,
			csvSafe(deref(e.ActorID)), csvSafe(deref(e.ActorEmail)), csvSafe(deref(e.IP)), csvSafe(e.Action),
			csvSafe(deref(e.TargetType)), csvSafe(deref(e.TargetID)), csvSafe(string(e.Details)), e.Hash,
		})
	})
	if err != nil {
		fmt.Fprintf(w, "\n# export aborted: %v\n", err)
	}
	cw.Flush()
}

// csvSafe neutralizes spreadsheet formula injection in exported cells.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *Server) verifyAudit(w http.ResponseWriter, r *http.Request) {
	res, err := s.audit.Verify(r.Context(), orgOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "audit.verify", "org", orgOf(r), res)
	writeJSON(w, http.StatusOK, res)
}

// ---- Users ----

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListUsers(r.Context(), orgOf(r))
	items(w, list, err)
}

// Only owners may grant the owner role or modify an existing owner.
func canManageRole(actorRole, targetRole string) bool {
	return targetRole != "owner" || actorRole == "owner"
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if !validEmail(req.Email) {
		fail(w, badRequest("a valid email is required"))
		return
	}
	if !auth.ValidRole(req.Role) {
		fail(w, badRequest("role must be owner, admin, helpdesk or auditor"))
		return
	}
	if !canManageRole(who(r).User.Role, req.Role) {
		writeErr(w, http.StatusForbidden, "only owners can create owners")
		return
	}
	if err := validatePassword(req.Password); err != nil {
		fail(w, err)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		fail(w, err)
		return
	}
	u, err := s.store.CreateUser(r.Context(), orgOf(r), req.Email, strings.TrimSpace(req.Name), req.Role, hash)
	if err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "a user with that email already exists")
			return
		}
		fail(w, err)
		return
	}
	s.record(r, "user.create", "user", u.ID, map[string]any{"email": u.Email, "role": u.Role})
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     *string `json:"name"`
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	org, id, me := orgOf(r), chi.URLParam(r, "id"), who(r).User
	target, err := s.store.GetUser(r.Context(), org, id)
	if err != nil {
		fail(w, err)
		return
	}
	if !canManageRole(me.Role, target.Role) {
		writeErr(w, http.StatusForbidden, "only owners can modify owners")
		return
	}
	if req.Role != nil {
		if !auth.ValidRole(*req.Role) {
			fail(w, badRequest("invalid role"))
			return
		}
		if !canManageRole(me.Role, *req.Role) {
			writeErr(w, http.StatusForbidden, "only owners can grant the owner role")
			return
		}
		if target.Role == "owner" && *req.Role != "owner" {
			if n, err := s.store.CountOwners(r.Context(), org); err != nil || n <= 1 {
				fail(w, badRequest("an organization must keep at least one owner"))
				return
			}
		}
	}
	var hash *string
	if req.Password != nil {
		if err := validatePassword(*req.Password); err != nil {
			fail(w, err)
			return
		}
		h, err := auth.HashPassword(*req.Password)
		if err != nil {
			fail(w, err)
			return
		}
		hash = &h
	}
	u, err := s.store.UpdateUser(r.Context(), org, id, req.Name, req.Role, hash)
	if err != nil {
		fail(w, err)
		return
	}
	// Role and password changes revoke the user's other sessions.
	if (req.Role != nil && *req.Role != target.Role) || hash != nil {
		if id != me.ID {
			_ = s.store.DeleteUserSessions(r.Context(), id)
		}
	}
	s.record(r, "user.update", "user", id, map[string]any{
		"email": u.Email, "role_from": target.Role, "role_to": u.Role, "password_changed": hash != nil})
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	org, id, me := orgOf(r), chi.URLParam(r, "id"), who(r).User
	if id == me.ID {
		fail(w, badRequest("you cannot delete your own account"))
		return
	}
	target, err := s.store.GetUser(r.Context(), org, id)
	if err != nil {
		fail(w, err)
		return
	}
	if !canManageRole(me.Role, target.Role) {
		writeErr(w, http.StatusForbidden, "only owners can delete owners")
		return
	}
	if err := s.store.DeleteUser(r.Context(), org, id); err != nil {
		fail(w, err)
		return
	}
	s.record(r, "user.delete", "user", id, map[string]any{"email": target.Email, "role": target.Role})
	ok(w)
}

// ---- Webhooks ----

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListWebhooks(r.Context(), orgOf(r))
	items(w, list, err)
}

type webhookRequest struct {
	Name    *string  `json:"name"`
	URL     *string  `json:"url"`
	Secret  *string  `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

func (req webhookRequest) apply(h *store.Webhook) error {
	if req.Name != nil {
		h.Name = strings.TrimSpace(*req.Name)
	}
	if req.URL != nil {
		h.URL = strings.TrimSpace(*req.URL)
	}
	if req.Secret != nil && *req.Secret != "" {
		h.Secret = *req.Secret
	}
	if req.Events != nil {
		h.Events = cleanList(req.Events)
	}
	if req.Enabled != nil {
		h.Enabled = *req.Enabled
	}
	if h.Name == "" {
		return badRequest("name is required")
	}
	u, err := url.Parse(h.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return badRequest("url must be an http(s) URL")
	}
	for _, e := range h.Events {
		if !slices.Contains(notify.Events, e) {
			return badRequest(fmt.Sprintf("unknown event %q (valid: %s)", e, strings.Join(notify.Events, ", ")))
		}
	}
	if len(h.Events) == 0 {
		return badRequest("subscribe to at least one event")
	}
	return nil
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	h := store.Webhook{Enabled: true, Secret: "whsec_" + auth.RandomToken(24)}
	if err := req.apply(&h); err != nil {
		fail(w, err)
		return
	}
	out, err := s.store.CreateWebhook(r.Context(), orgOf(r), h)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "webhook.create", "webhook", out.ID, map[string]any{"name": out.Name, "url": out.URL, "events": out.Events})
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updateWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	h, err := s.store.GetWebhook(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	if err := req.apply(&h); err != nil {
		fail(w, err)
		return
	}
	out, err := s.store.UpdateWebhook(r.Context(), orgOf(r), h)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "webhook.update", "webhook", out.ID, map[string]any{"name": out.Name, "url": out.URL, "events": out.Events, "enabled": out.Enabled})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteWebhook(r.Context(), orgOf(r), id); err != nil {
		fail(w, err)
		return
	}
	s.record(r, "webhook.delete", "webhook", id, nil)
	ok(w)
}

func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request) {
	h, err := s.store.GetWebhook(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	status, err := s.core.Hooks.SendTest(r.Context(), h)
	res := map[string]any{"status": status, "ok": err == nil && status >= 200 && status < 300}
	if err != nil {
		res["error"] = err.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

// ---- Org ----

func (s *Server) getOrg(w http.ResponseWriter, r *http.Request) {
	o, err := s.store.GetOrg(r.Context(), orgOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) updateOrg(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name             *string `json:"name"`
		ServerURL        *string `json:"server_url"`
		UsageTracking    *bool   `json:"usage_tracking"`
		IdleThresholdSec *int    `json:"idle_threshold_sec"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		fail(w, badRequest("name cannot be empty"))
		return
	}
	if req.ServerURL != nil && *req.ServerURL != "" {
		u, err := url.Parse(*req.ServerURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			fail(w, badRequest("server_url must be an http(s) URL"))
			return
		}
	}
	if req.IdleThresholdSec != nil && (*req.IdleThresholdSec < 60 || *req.IdleThresholdSec > 900) {
		fail(w, badRequest("idle_threshold_sec must be between 60 and 900"))
		return
	}
	o, err := s.store.UpdateOrg(r.Context(), orgOf(r), store.OrgPatch{
		Name: req.Name, ServerURL: req.ServerURL, UsageTracking: req.UsageTracking, IdleThresholdSec: req.IdleThresholdSec,
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "org.update", "org", o.ID, o)
	if req.UsageTracking != nil || req.IdleThresholdSec != nil {
		s.core.PushOrgPolicy(r.Context(), o.ID)
	}
	writeJSON(w, http.StatusOK, o)
}

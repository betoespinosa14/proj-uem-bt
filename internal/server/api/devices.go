package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/internal/server/store"
)

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.Dashboard(r.Context(), orgOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := s.store.ListDevices(r.Context(), orgOf(r), store.DeviceFilter{
		Query: strings.TrimSpace(q.Get("q")), Status: q.Get("status"), OSFamily: q.Get("os"),
		Compliance: q.Get("compliance"), GroupID: q.Get("group_id"),
	})
	items(w, list, err)
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GetDeviceDetail(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    *string         `json:"name"`
		GroupID json.RawMessage `json:"group_id"` // absent = unchanged, null = remove from group
		Tags    []string        `json:"tags"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	org, id := orgOf(r), chi.URLParam(r, "id")
	if req.Name != nil {
		*req.Name = strings.TrimSpace(*req.Name)
		if *req.Name == "" {
			fail(w, badRequest("name cannot be empty"))
			return
		}
	}
	var groupID *string
	clearGroup := string(req.GroupID) == "null"
	if len(req.GroupID) > 0 && !clearGroup {
		var g string
		if err := json.Unmarshal(req.GroupID, &g); err != nil {
			fail(w, badRequest("group_id must be a string or null"))
			return
		}
		if err := s.store.GroupBelongs(r.Context(), org, g); err != nil {
			fail(w, badRequest("group not found"))
			return
		}
		groupID = &g
	}
	if req.Tags != nil {
		req.Tags = cleanList(req.Tags)
	}
	before, err := s.store.GetDevice(r.Context(), org, id)
	if err != nil {
		fail(w, err)
		return
	}
	d, err := s.store.UpdateDevice(r.Context(), org, id, req.Name, groupID, clearGroup, req.Tags)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "device.update", "device", id, map[string]any{
		"device_name": d.Name, "name": req.Name, "group_id": d.GroupID, "tags": req.Tags})
	if !equalPtr(before.GroupID, d.GroupID) {
		s.core.PushPolicy(r.Context(), id)
	}
	_ = s.core.EvaluateDevice(r.Context(), org, id) // group/tags can change which rules apply
	s.core.PublishDevice(r.Context(), org, id)
	d, _ = s.store.GetDevice(r.Context(), org, id)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) retireDevice(w http.ResponseWriter, r *http.Request) {
	org, id := orgOf(r), chi.URLParam(r, "id")
	d, err := s.store.GetDevice(r.Context(), org, id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.store.RetireDevice(r.Context(), org, id); err != nil {
		fail(w, err)
		return
	}
	s.core.Hub.DisconnectAgent(id)
	s.record(r, "device.retire", "device", id, map[string]any{"device_name": d.Name, "serial": d.Serial})
	ok(w)
}

func (s *Server) deviceSoftware(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSoftware(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	items(w, list, err)
}

func (s *Server) deviceTelemetry(w http.ResponseWriter, r *http.Request) {
	hours, err := strconv.Atoi(r.URL.Query().Get("hours"))
	if err != nil || hours <= 0 {
		hours = 24
	}
	if hours > 24*30 {
		hours = 24 * 30
	}
	list, err := s.store.ListTelemetry(r.Context(), orgOf(r), chi.URLParam(r, "id"), time.Now().Add(-time.Duration(hours)*time.Hour))
	items(w, list, err)
}

func (s *Server) deviceCompliance(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDeviceResults(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	items(w, list, err)
}

func (s *Server) deviceCommands(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDeviceCommands(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	items(w, list, err)
}

type commandRequest struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func (s *Server) createCommand(w http.ResponseWriter, r *http.Request) {
	var req commandRequest
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	cmd, err := s.core.IssueCommand(r.Context(), orgOf(r), chi.URLParam(r, "id"), req.Type, req.Payload, s.actor(r), "user")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cmd)
}

func (s *Server) bulkCommand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceIDs []string        `json:"device_ids"`
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	if len(req.DeviceIDs) == 0 || len(req.DeviceIDs) > 1000 {
		fail(w, badRequest("device_ids must contain 1 to 1000 devices"))
		return
	}
	// Validate once up front so a bad payload fails the whole request.
	if _, err := s.core.ResolvePayload(r.Context(), orgOf(r), req.Type, req.Payload); err != nil {
		fail(w, err)
		return
	}
	out := []store.Command{}
	for _, id := range req.DeviceIDs {
		cmd, err := s.core.IssueCommand(r.Context(), orgOf(r), id, req.Type, req.Payload, s.actor(r), "user")
		if err != nil {
			continue // devices not in this org are skipped
		}
		out = append(out, cmd)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": out})
}

func (s *Server) getCommand(w http.ResponseWriter, r *http.Request) {
	c, err := s.store.GetCommand(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// cleanList trims entries and drops empties and duplicates.
func cleanList(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

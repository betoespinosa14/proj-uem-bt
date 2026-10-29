package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/internal/server/compliance"
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

func deviceFilter(r *http.Request) store.DeviceFilter {
	q := r.URL.Query()
	return store.DeviceFilter{
		Query: strings.TrimSpace(q.Get("q")), Status: q.Get("status"), OSFamily: q.Get("os"),
		Compliance: q.Get("compliance"), GroupID: q.Get("group_id"),
	}
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDevices(r.Context(), orgOf(r), deviceFilter(r))
	items(w, list, err)
}

// exportDevices writes the (filtered) device list as a CSV report.
func (s *Server) exportDevices(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListDevices(r.Context(), orgOf(r), deviceFilter(r))
	if err != nil {
		fail(w, err)
		return
	}
	software, err := s.store.ListOrgSoftware(r.Context(), orgOf(r))
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "devices.export", "org", orgOf(r), map[string]any{"count": len(list)})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="devices-%s.csv"`, time.Now().UTC().Format("20060102-150405")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"name", "hostname", "status", "compliance", "os_family", "os_name", "os_version", "os_build", "arch",
		"manufacturer", "model", "serial", "ip_address", "mac_address", "logged_in_user", "group", "tags",
		"agent_version", "enrolled_at", "last_seen_at", "cpu_pct", "mem_pct", "disk_pct", "battery_pct",
		"software_count", "gambling_apps", "games",
	})
	for _, d := range list {
		var gambling, games []string
		for _, sw := range software[d.ID] {
			switch compliance.Classify(sw) {
			case compliance.FlagGambling:
				gambling = append(gambling, sw.Name)
			case compliance.FlagGame:
				games = append(games, sw.Name)
			}
		}
		lastSeen := ""
		if d.LastSeenAt != nil {
			lastSeen = d.LastSeenAt.UTC().Format(time.RFC3339)
		}
		_ = cw.Write([]string{
			csvSafe(d.Name), csvSafe(d.Hostname), d.Status, d.ComplianceState, d.OSFamily, csvSafe(d.OSName),
			csvSafe(d.OSVersion), csvSafe(d.OSBuild), d.Arch, csvSafe(d.Manufacturer), csvSafe(d.Model),
			csvSafe(d.Serial), d.IPAddress, d.MACAddress, csvSafe(d.LoggedInUser), csvSafe(deref(d.GroupName)),
			csvSafe(strings.Join(d.Tags, "; ")), d.AgentVersion, d.EnrolledAt.UTC().Format(time.RFC3339), lastSeen,
			pct(d.CPUPct), pct(d.MemPct), pct(d.DiskPct), pct(d.BatteryPct),
			strconv.Itoa(len(software[d.ID])), csvSafe(strings.Join(gambling, "; ")), csvSafe(strings.Join(games, "; ")),
		})
	}
	cw.Flush()
}

func pct(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', 1, 64)
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

// listSoftware returns a device's software with category and flag set.
func (s *Server) listSoftware(r *http.Request) ([]store.Software, error) {
	list, err := s.store.ListSoftware(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	for i := range list {
		list[i].Category = compliance.Category(list[i])
		list[i].Flag = compliance.Classify(list[i])
	}
	return list, err
}

func (s *Server) deviceSoftware(w http.ResponseWriter, r *http.Request) {
	list, err := s.listSoftware(r)
	items(w, list, err)
}

// exportSoftware writes a device's installed software as a CSV report,
// honouring the same q and category filters as the Software tab.
func (s *Server) exportSoftware(w http.ResponseWriter, r *http.Request) {
	dev, err := s.store.GetDevice(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	list, err := s.listSoftware(r)
	if err != nil {
		fail(w, err)
		return
	}
	needle := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	category := r.URL.Query().Get("category")
	s.record(r, "device.software.export", "device", dev.ID, map[string]any{"device_name": dev.Name})
	cw := csvResponse(w, "software-"+fileSafe(dev.Name))
	_ = cw.Write([]string{"device", "name", "version", "publisher", "type", "flag", "source", "installed"})
	for _, sw := range list {
		switch {
		case category == "flagged" && sw.Flag == "",
			(category == compliance.CategorySystem || category == compliance.CategoryThirdParty) && sw.Category != category,
			needle != "" && !strings.Contains(strings.ToLower(sw.Name+" "+sw.Publisher), needle):
			continue
		}
		typ := "System"
		if sw.Category == compliance.CategoryThirdParty {
			typ = "Third-party"
		}
		_ = cw.Write([]string{
			csvSafe(dev.Name), csvSafe(sw.Name), csvSafe(sw.Version), csvSafe(sw.Publisher), typ, sw.Flag,
			csvSafe(sw.Source), csvSafe(deref(sw.InstallDate)),
		})
	}
	cw.Flush()
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

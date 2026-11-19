package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/internal/server/compliance"
	"github.com/uem/uem/internal/server/store"
)

// usagePeriod parses ?days= (1, 7, 30, 90 or 365; default 30) and returns the
// number of days and the first and last day of the period (last nil: up to
// today). Days are the server's local dates, like the dates agents report
// usage under, so 1 means today. ?yesterday=true means yesterday only.
func usagePeriod(r *http.Request) (int, time.Time, *time.Time) {
	y, m, d := time.Now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if r.URL.Query().Get("yesterday") == "true" {
		day := today.AddDate(0, 0, -1)
		return 1, day, &day
	}
	days := 30
	switch r.URL.Query().Get("days") {
	case "1":
		days = 1
	case "7":
		days = 7
	case "90":
		days = 90
	case "365":
		days = 365
	}
	return days, today.AddDate(0, 0, -(days - 1)), nil
}

// byApp reports whether ?group=app asks for totals per app across users
// (the default, group=app_user, is one row per app and user).
func byApp(r *http.Request) bool { return r.URL.Query().Get("group") == "app" }

func (s *Server) deviceUsage(w http.ResponseWriter, r *http.Request) {
	_, since, until := usagePeriod(r)
	list, err := s.store.DeviceUsage(r.Context(), orgOf(r), chi.URLParam(r, "id"), since, until, byApp(r))
	items(w, list, err)
}

func (s *Server) exportDeviceUsage(w http.ResponseWriter, r *http.Request) {
	dev, err := s.store.GetDevice(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	days, since, until := usagePeriod(r)
	grouped := byApp(r)
	list, err := s.store.DeviceUsage(r.Context(), orgOf(r), dev.ID, since, until, grouped)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "device.usage.export", "device", dev.ID, map[string]any{"device_name": dev.Name, "days": days, "by_app": grouped})
	cw := csvResponse(w, fmt.Sprintf("usage-%s", fileSafe(dev.Name)))
	who := "user"
	if grouped {
		who = "users"
	}
	_ = cw.Write([]string{"device", "app", who, "active_hours", "open_hours", "days_used", "last_used", "period_days"})
	for _, u := range list {
		user := csvSafe(u.User)
		if grouped {
			user = strconv.Itoa(u.Users)
		}
		_ = cw.Write([]string{csvSafe(dev.Name), csvSafe(u.App), user, activeHours(u.ActiveSeconds), hours(u.Minutes),
			strconv.Itoa(u.DaysUsed), u.LastUsed.UTC().Format(time.RFC3339), strconv.Itoa(days)})
	}
	cw.Flush()
}

// thirdParty keeps only third-party apps: built-in OS components aren't
// licensed products, so they don't belong in an "unused" report.
func thirdParty[T any](list []T, sw func(T) store.Software) []T {
	out := list[:0]
	for _, u := range list {
		if compliance.Category(sw(u)) == compliance.CategoryThirdParty {
			out = append(out, u)
		}
	}
	return out
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// fleetFilter parses the App usage filters: ?days=, ?app=, ?unused=true and
// ?group_id= (only devices in that group) and ?device_id= (only that device).
func fleetFilter(r *http.Request) (int, store.FleetUsageFilter, error) {
	days, since, until := usagePeriod(r)
	q := r.URL.Query()
	group := strings.TrimSpace(q.Get("group_id"))
	if group != "" && !uuidRe.MatchString(group) {
		return days, store.FleetUsageFilter{}, badRequest("group_id must be a group ID")
	}
	device := strings.TrimSpace(q.Get("device_id"))
	if device != "" && !uuidRe.MatchString(device) {
		return days, store.FleetUsageFilter{}, badRequest("device_id must be a device ID")
	}
	return days, store.FleetUsageFilter{App: strings.TrimSpace(q.Get("app")), Since: since, Until: until, Unused: q.Get("unused") == "true", GroupID: group, DeviceID: device}, nil
}

// thirdPartyApps tells, per lower-cased app name, whether an app is
// third-party: any install of it is (Edge: built into Windows, installed by
// hand on Macs), or it is missing from the inventory. Same rule as the
// dashboard's idle ranking.
func (s *Server) thirdPartyApps(r *http.Request, names []string) (map[string]bool, error) {
	info, err := s.store.SoftwareInfo(r.Context(), orgOf(r), names)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(names))
	for _, n := range names {
		k := strings.ToLower(n)
		installs := info[k]
		third := len(installs) == 0
		for _, sw := range installs {
			if compliance.Category(sw) == compliance.CategoryThirdParty {
				third = true
			}
		}
		out[k] = third
	}
	return out, nil
}

// markThirdParty sets each row's third-party flag and, with
// ?third_party=true, keeps third-party rows only. The "Not used" list is
// third-party only already.
func markThirdParty[T any](s *Server, r *http.Request, f store.FleetUsageFilter, list []T, row func(*T) (store.Software, *bool)) ([]T, error) {
	if f.Unused {
		list = thirdParty(list, func(u T) store.Software { sw, _ := row(&u); return sw })
		for i := range list {
			_, flag := row(&list[i])
			*flag = true
		}
		return list, nil
	}
	names := make([]string, 0, len(list))
	for i := range list {
		sw, _ := row(&list[i])
		names = append(names, sw.Name)
	}
	third, err := s.thirdPartyApps(r, names)
	if err != nil {
		return nil, err
	}
	only := r.URL.Query().Get("third_party") == "true"
	out := list[:0]
	for i := range list {
		sw, flag := row(&list[i])
		*flag = third[strings.ToLower(sw.Name)]
		if !only || *flag {
			out = append(out, list[i])
		}
	}
	return out, nil
}

func (s *Server) fleetUsage(r *http.Request) ([]store.FleetUsageRow, error) {
	_, f, err := fleetFilter(r)
	if err != nil {
		return nil, err
	}
	list, err := s.store.FleetUsage(r.Context(), orgOf(r), f)
	if err != nil {
		return nil, err
	}
	return markThirdParty(s, r, f, list, func(u *store.FleetUsageRow) (store.Software, *bool) {
		return store.Software{Name: u.App, Publisher: u.Publisher, Source: u.Source}, &u.ThirdParty
	})
}

func (s *Server) fleetUsageByApp(r *http.Request) ([]store.FleetAppRow, error) {
	_, f, err := fleetFilter(r)
	if err != nil {
		return nil, err
	}
	list, err := s.store.FleetUsageByApp(r.Context(), orgOf(r), f)
	if err != nil {
		return nil, err
	}
	return markThirdParty(s, r, f, list, func(u *store.FleetAppRow) (store.Software, *bool) {
		return store.Software{Name: u.App, Publisher: u.Publisher, Source: u.Source}, &u.ThirdParty
	})
}

// usageSummary serves the App usage dashboard: the same period and app
// search as the table, totalled.
func (s *Server) usageSummary(w http.ResponseWriter, r *http.Request) {
	days, f, err := fleetFilter(r)
	if err != nil {
		fail(w, err)
		return
	}
	sum, err := s.store.UsageSummary(r.Context(), orgOf(r), f)
	if err != nil {
		fail(w, err)
		return
	}
	// Today is shown against the last working day in full, as a reference for
	// a normal day (Yesterday stands alone).
	if days == 1 && f.Until == nil {
		if sum.Compare, err = s.store.PreviousWorkday(r.Context(), orgOf(r), f); err != nil {
			fail(w, err)
			return
		}
	}
	// Same rule as the table's "Not used" view: installed third-party apps
	// with no active use in the period.
	f.Unused = true
	unused, err := s.store.FleetUsageByApp(r.Context(), orgOf(r), f)
	if err != nil {
		fail(w, err)
		return
	}
	sum.Totals.UnusedApps = len(thirdParty(unused, func(u store.FleetAppRow) store.Software {
		return store.Software{Name: u.App, Publisher: u.Publisher, Source: u.Source}
	}))
	// Same split for the idle ranking, so Terminal or System Settings left
	// open don't head a license audit. Like the background card, an app is
	// third-party if any install of it is (Edge: built into Windows, installed
	// by hand on Macs), and so is an app missing from the inventory.
	names := make([]string, len(sum.IdleApps))
	for i, a := range sum.IdleApps {
		names[i] = a.App
	}
	third, err := s.thirdPartyApps(r, names)
	if err != nil {
		fail(w, err)
		return
	}
	for i := range sum.IdleApps {
		sum.IdleApps[i].ThirdParty = third[strings.ToLower(sum.IdleApps[i].App)]
	}
	// System processes (bash, systemd, OS built-ins) vs vendor software, by
	// the same rule the "Not used" list uses.
	for i := range sum.Background {
		b := &sum.Background[i]
		b.ThirdParty = compliance.Category(store.Software{Name: b.App, Publisher: b.Publisher, Source: b.Source}) == compliance.CategoryThirdParty
		if b.ThirdParty {
			sum.Totals.BackgroundThirdParty++
		}
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) listFleetUsage(w http.ResponseWriter, r *http.Request) {
	if byApp(r) {
		list, err := s.fleetUsageByApp(r)
		items(w, list, err)
		return
	}
	list, err := s.fleetUsage(r)
	items(w, list, err)
}

func (s *Server) exportFleetUsage(w http.ResponseWriter, r *http.Request) {
	days, f, err := fleetFilter(r)
	if err != nil {
		fail(w, err)
		return
	}
	audit := map[string]any{"days": days, "app": f.App, "group_id": f.GroupID, "device_id": f.DeviceID, "by_app": byApp(r)}
	if byApp(r) {
		list, err := s.fleetUsageByApp(r)
		if err != nil {
			fail(w, err)
			return
		}
		audit["count"] = len(list)
		s.record(r, "usage.export", "org", orgOf(r), audit)
		cw := csvResponse(w, "app-usage-by-app")
		_ = cw.Write([]string{"app", "users", "devices_used_on", "installed_on", "active_hours", "open_hours", "last_used", "period_days"})
		for _, u := range list {
			_ = cw.Write([]string{csvSafe(u.App), strconv.Itoa(u.Users), strconv.Itoa(u.Devices), strconv.Itoa(u.InstalledOn),
				activeHours(u.ActiveSeconds), hours(u.Minutes), timeOrEmpty(u.LastUsed), strconv.Itoa(days)})
		}
		cw.Flush()
		return
	}
	list, err := s.fleetUsage(r)
	if err != nil {
		fail(w, err)
		return
	}
	audit["count"] = len(list)
	s.record(r, "usage.export", "org", orgOf(r), audit)
	cw := csvResponse(w, "app-usage")
	_ = cw.Write([]string{"app", "device", "user", "installed", "active_hours", "open_hours", "days_used", "last_used", "period_days"})
	for _, u := range list {
		_ = cw.Write([]string{csvSafe(u.App), csvSafe(u.DeviceName), csvSafe(u.User), strconv.FormatBool(u.Installed),
			activeHours(u.ActiveSeconds), hours(u.Minutes), strconv.Itoa(u.DaysUsed), timeOrEmpty(u.LastUsed), strconv.Itoa(days)})
	}
	cw.Flush()
}

func timeOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func activeHours(seconds int) string { return strconv.FormatFloat(float64(seconds)/3600, 'f', 2, 64) }

func hours(minutes int) string { return strconv.FormatFloat(float64(minutes)/60, 'f', 1, 64) }

// csvResponse sets download headers for a timestamped CSV file.
func csvResponse(w http.ResponseWriter, name string) *csv.Writer {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, name, time.Now().UTC().Format("20060102-150405")))
	return csv.NewWriter(w)
}

// fileSafe strips characters that would break a Content-Disposition filename.
func fileSafe(name string) string {
	return strings.Map(func(c rune) rune {
		if c == '"' || c == '/' || c == '\\' || c < ' ' {
			return '_'
		}
		return c
	}, name)
}

func (s *Server) listInfrastructure(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.InfrastructureApps(r.Context(), orgOf(r))
	items(w, list, err)
}

// markInfrastructure marks an app as infrastructure (open by design, e.g. a
// remote-access host) so App usage stops ranking its open time as idle.
func (s *Server) markInfrastructure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App string `json:"app"`
	}
	if err := decode(r, &req); err != nil {
		fail(w, err)
		return
	}
	app := strings.TrimSpace(req.App)
	if app == "" || len(app) > 256 {
		fail(w, badRequest("app is required (up to 256 characters)"))
		return
	}
	if err := s.store.MarkInfrastructure(r.Context(), orgOf(r), app, who(r).User.ID); err != nil {
		fail(w, err)
		return
	}
	s.record(r, "usage.infrastructure.add", "org", orgOf(r), map[string]any{"app": app})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) unmarkInfrastructure(w http.ResponseWriter, r *http.Request) {
	app := strings.TrimSpace(r.URL.Query().Get("app"))
	if app == "" {
		fail(w, badRequest("app is required"))
		return
	}
	removed, err := s.store.UnmarkInfrastructure(r.Context(), orgOf(r), app)
	if err != nil {
		fail(w, err)
		return
	}
	if removed {
		s.record(r, "usage.infrastructure.remove", "org", orgOf(r), map[string]any{"app": app})
	}
	w.WriteHeader(http.StatusNoContent)
}

// exportReclaim is the license-reclamation list for procurement: one row per
// installed third-party app per device with no use in the period.
func (s *Server) exportReclaim(w http.ResponseWriter, r *http.Request) {
	days, f, err := fleetFilter(r)
	if err != nil {
		fail(w, err)
		return
	}
	list, err := s.store.ReclaimList(r.Context(), orgOf(r), f)
	if err != nil {
		fail(w, err)
		return
	}
	list = thirdParty(list, func(x store.ReclaimRow) store.Software {
		return store.Software{Name: x.App, Publisher: x.Publisher, Source: x.Source}
	})
	s.record(r, "usage.export", "org", orgOf(r), map[string]any{"kind": "reclaim", "days": days, "app": f.App, "group_id": f.GroupID, "device_id": f.DeviceID, "count": len(list)})
	cw := csvResponse(w, "license-reclaim")
	_ = cw.Write([]string{"app", "publisher", "version", "device", "device_user", "device_last_seen", "last_used", "installed_on", "period_days"})
	for _, x := range list {
		last := "never"
		if x.LastUsed != nil {
			last = timeOrEmpty(x.LastUsed)
		}
		_ = cw.Write([]string{csvSafe(x.App), csvSafe(x.Publisher), csvSafe(x.Version), csvSafe(x.DeviceName), csvSafe(x.DeviceUser),
			timeOrEmpty(x.DeviceSeen), last, csvSafe(x.InstallDate), strconv.Itoa(days)})
	}
	cw.Flush()
}

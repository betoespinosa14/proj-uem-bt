package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/uem/uem/internal/server/compliance"
	"github.com/uem/uem/internal/server/store"
)

// usagePeriod parses ?days= (30, 90 or 365; default 30) and returns the
// number of days and the first day of the period.
func usagePeriod(r *http.Request) (int, time.Time) {
	days := 30
	switch r.URL.Query().Get("days") {
	case "90":
		days = 90
	case "365":
		days = 365
	}
	y, m, d := time.Now().Date()
	return days, time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
}

// byApp reports whether ?group=app asks for totals per app across users
// (the default, group=app_user, is one row per app and user).
func byApp(r *http.Request) bool { return r.URL.Query().Get("group") == "app" }

func (s *Server) deviceUsage(w http.ResponseWriter, r *http.Request) {
	_, since := usagePeriod(r)
	list, err := s.store.DeviceUsage(r.Context(), orgOf(r), chi.URLParam(r, "id"), since, byApp(r))
	items(w, list, err)
}

func (s *Server) exportDeviceUsage(w http.ResponseWriter, r *http.Request) {
	dev, err := s.store.GetDevice(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	days, since := usagePeriod(r)
	grouped := byApp(r)
	list, err := s.store.DeviceUsage(r.Context(), orgOf(r), dev.ID, since, grouped)
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

func fleetFilter(r *http.Request) (int, store.FleetUsageFilter) {
	days, since := usagePeriod(r)
	q := r.URL.Query()
	return days, store.FleetUsageFilter{App: strings.TrimSpace(q.Get("app")), Since: since, Unused: q.Get("unused") == "true"}
}

func (s *Server) fleetUsage(r *http.Request) ([]store.FleetUsageRow, error) {
	_, f := fleetFilter(r)
	list, err := s.store.FleetUsage(r.Context(), orgOf(r), f)
	if err != nil || !f.Unused {
		return list, err
	}
	return thirdParty(list, func(u store.FleetUsageRow) store.Software {
		return store.Software{Name: u.App, Publisher: u.Publisher, Source: u.Source}
	}), nil
}

func (s *Server) fleetUsageByApp(r *http.Request) ([]store.FleetAppRow, error) {
	_, f := fleetFilter(r)
	list, err := s.store.FleetUsageByApp(r.Context(), orgOf(r), f)
	if err != nil || !f.Unused {
		return list, err
	}
	return thirdParty(list, func(u store.FleetAppRow) store.Software {
		return store.Software{Name: u.App, Publisher: u.Publisher, Source: u.Source}
	}), nil
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
	days, _ := fleetFilter(r)
	audit := map[string]any{"days": days, "app": r.URL.Query().Get("app"), "by_app": byApp(r)}
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

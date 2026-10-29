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

func (s *Server) deviceUsage(w http.ResponseWriter, r *http.Request) {
	_, since := usagePeriod(r)
	list, err := s.store.DeviceUsage(r.Context(), orgOf(r), chi.URLParam(r, "id"), since)
	items(w, list, err)
}

func (s *Server) exportDeviceUsage(w http.ResponseWriter, r *http.Request) {
	dev, err := s.store.GetDevice(r.Context(), orgOf(r), chi.URLParam(r, "id"))
	if err != nil {
		fail(w, err)
		return
	}
	days, since := usagePeriod(r)
	list, err := s.store.DeviceUsage(r.Context(), orgOf(r), dev.ID, since)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "device.usage.export", "device", dev.ID, map[string]any{"device_name": dev.Name, "days": days})
	cw := csvResponse(w, fmt.Sprintf("usage-%s", fileSafe(dev.Name)))
	_ = cw.Write([]string{"device", "app", "user", "last_used", "days_used", "hours", "period_days"})
	for _, u := range list {
		_ = cw.Write([]string{csvSafe(dev.Name), csvSafe(u.App), csvSafe(u.User), u.LastUsed.UTC().Format(time.RFC3339),
			strconv.Itoa(u.DaysUsed), hours(u.Minutes), strconv.Itoa(days)})
	}
	cw.Flush()
}

// fleetUsage applies the shared filters. Unused rows are limited to
// third-party apps: built-in OS components aren't licensed products.
func (s *Server) fleetUsage(r *http.Request) (int, []store.FleetUsageRow, error) {
	days, since := usagePeriod(r)
	q := r.URL.Query()
	unused := q.Get("unused") == "true"
	list, err := s.store.FleetUsage(r.Context(), orgOf(r), store.FleetUsageFilter{
		App: strings.TrimSpace(q.Get("app")), Since: since, Unused: unused,
	})
	if err != nil || !unused {
		return days, list, err
	}
	out := list[:0]
	for _, u := range list {
		if compliance.Category(store.Software{Name: u.App, Publisher: u.Publisher, Source: u.Source}) == compliance.CategoryThirdParty {
			out = append(out, u)
		}
	}
	return days, out, nil
}

func (s *Server) listFleetUsage(w http.ResponseWriter, r *http.Request) {
	_, list, err := s.fleetUsage(r)
	items(w, list, err)
}

func (s *Server) exportFleetUsage(w http.ResponseWriter, r *http.Request) {
	days, list, err := s.fleetUsage(r)
	if err != nil {
		fail(w, err)
		return
	}
	s.record(r, "usage.export", "org", orgOf(r), map[string]any{"days": days, "app": r.URL.Query().Get("app"), "count": len(list)})
	cw := csvResponse(w, "app-usage")
	_ = cw.Write([]string{"app", "device", "user", "installed", "last_used", "days_used", "hours", "period_days"})
	for _, u := range list {
		last := ""
		if u.LastUsed != nil {
			last = u.LastUsed.UTC().Format(time.RFC3339)
		}
		_ = cw.Write([]string{csvSafe(u.App), csvSafe(u.DeviceName), csvSafe(u.User), strconv.FormatBool(u.Installed), last,
			strconv.Itoa(u.DaysUsed), hours(u.Minutes), strconv.Itoa(days)})
	}
	cw.Flush()
}

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

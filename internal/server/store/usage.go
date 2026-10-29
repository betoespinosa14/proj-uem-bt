package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/uem/uem/internal/protocol"
)

// UsageTrackingEnabled reports whether the org has application usage
// tracking switched on.
func (s *Store) UsageTrackingEnabled(ctx context.Context, orgID string) (bool, error) {
	var on bool
	err := s.Pool.QueryRow(ctx, `SELECT usage_tracking FROM organizations WHERE id = $1`, orgID).Scan(&on)
	return on, mapErr(err)
}

// RecordUsage adds reported running minutes and active seconds to the
// per-day totals. A day never exceeds 1440 minutes or 86400 active seconds
// per (app, user), whatever the agent sends.
func (s *Store) RecordUsage(ctx context.Context, orgID, deviceID string, entries []protocol.UsageEntry) error {
	b := &pgx.Batch{}
	for _, e := range entries {
		day, err := time.Parse(time.DateOnly, e.Date)
		if err != nil || e.App == "" || e.User == "" || e.Minutes < 0 || e.ActiveSeconds < 0 || e.Minutes+e.ActiveSeconds == 0 {
			continue
		}
		last := e.LastSeen
		if last.IsZero() || last.After(time.Now().Add(5*time.Minute)) {
			last = time.Now()
		}
		b.Queue(`
			INSERT INTO app_usage (device_id, org_id, day, app, username, minutes, active_seconds, last_used)
			VALUES ($1, $2, $3, $4, $5, LEAST($6::int, 1440), LEAST($7::int, 86400), $8)
			ON CONFLICT (device_id, day, app, username) DO UPDATE SET
				minutes = LEAST(app_usage.minutes + EXCLUDED.minutes, 1440),
				active_seconds = LEAST(app_usage.active_seconds + EXCLUDED.active_seconds, 86400),
				last_used = GREATEST(app_usage.last_used, EXCLUDED.last_used)`,
			deviceID, orgID, day, clip(e.App, 256), clip(e.User, 256), e.Minutes, e.ActiveSeconds, last)
	}
	if b.Len() == 0 {
		return nil
	}
	return s.Pool.SendBatch(ctx, b).Close()
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (s *Store) PurgeUsage(ctx context.Context, before time.Time) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM app_usage WHERE day < $1`, before)
	return err
}

// UsageRow summarizes one app's use over a period: by one user, or (when
// grouped by app) by all users, with User empty and Users set.
type UsageRow struct {
	App           string    `json:"app"`
	User          string    `json:"user"`
	Users         int       `json:"users"`
	LastUsed      time.Time `json:"last_used"`
	DaysUsed      int       `json:"days_used"`
	Minutes       int       `json:"minutes"`
	ActiveSeconds int       `json:"active_seconds"`
}

// DeviceUsage returns totals for a device since a day, per (app, user) or,
// with byApp, per app across all users. Most active first.
func (s *Store) DeviceUsage(ctx context.Context, orgID, deviceID string, since time.Time, byApp bool) ([]UsageRow, error) {
	user := "username"
	if byApp {
		user = "''"
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT app, `+user+`, count(DISTINCT username), max(last_used), count(DISTINCT day), sum(minutes), sum(active_seconds)
		FROM app_usage WHERE org_id = $1 AND device_id = $2 AND day >= $3
		GROUP BY app, 2 ORDER BY sum(active_seconds) DESC, max(last_used) DESC`, orgID, deviceID, since)
	return collect(rows, err, func(r pgx.Rows) (UsageRow, error) {
		var u UsageRow
		return u, r.Scan(&u.App, &u.User, &u.Users, &u.LastUsed, &u.DaysUsed, &u.Minutes, &u.ActiveSeconds)
	})
}

// FleetUsageFilter narrows the fleet usage report.
type FleetUsageFilter struct {
	App    string    // substring match on the app name
	Since  time.Time // start of the period
	Unused bool      // only installed apps with no use in the period
}

// FleetUsageRow is one (device, app, user) line of the fleet report. For an
// installed app with no use in the period, User is empty and LastUsed is the
// most recent use ever recorded (nil if never).
type FleetUsageRow struct {
	DeviceID      string     `json:"device_id"`
	DeviceName    string     `json:"device_name"`
	App           string     `json:"app"`
	User          string     `json:"user"`
	Installed     bool       `json:"installed"`
	LastUsed      *time.Time `json:"last_used"`
	DaysUsed      int        `json:"days_used"`
	Minutes       int        `json:"minutes"`
	ActiveSeconds int        `json:"active_seconds"`
	Publisher     string     `json:"-"`
	Source        string     `json:"-"`
}

// FleetUsage joins installed software with usage so installed-but-unused
// apps are reported alongside used ones.
func (s *Store) FleetUsage(ctx context.Context, orgID string, f FleetUsageFilter) ([]FleetUsageRow, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH used AS (
			SELECT device_id, app, username, max(last_used) AS last_used,
				count(DISTINCT day) AS days, sum(minutes) AS minutes, sum(active_seconds) AS active
			FROM app_usage WHERE org_id = $1 AND day >= $2
			GROUP BY device_id, app, username
		), ever AS (
			SELECT device_id, lower(app) AS app, max(last_used) AS last_used
			FROM app_usage WHERE org_id = $1 GROUP BY device_id, lower(app)
		), installed AS (
			SELECT DISTINCT ON (device_id, lower(name)) device_id, name, publisher, source
			FROM device_software WHERE org_id = $1
		)
		SELECT d.id, d.name, COALESCE(u.app, i.name), COALESCE(u.username, ''), i.name IS NOT NULL,
			COALESCE(u.last_used, e.last_used), COALESCE(u.days, 0), COALESCE(u.minutes, 0), COALESCE(u.active, 0),
			COALESCE(i.publisher, ''), COALESCE(i.source, '')
		FROM installed i
		FULL OUTER JOIN used u ON u.device_id = i.device_id AND lower(u.app) = lower(i.name)
		JOIN devices d ON d.id = COALESCE(i.device_id, u.device_id) AND d.retired_at IS NULL
		LEFT JOIN ever e ON e.device_id = d.id AND e.app = lower(COALESCE(u.app, i.name))
		WHERE ($3 = '' OR lower(COALESCE(u.app, i.name)) LIKE '%' || lower($3) || '%')
			AND (CASE WHEN $4 THEN u.app IS NULL ELSE u.app IS NOT NULL OR $3 <> '' END)
		ORDER BY lower(COALESCE(u.app, i.name)), COALESCE(u.active, 0) DESC, d.name, u.username
		LIMIT 10000`, orgID, f.Since, f.App, f.Unused)
	return collect(rows, err, func(r pgx.Rows) (FleetUsageRow, error) {
		var u FleetUsageRow
		return u, r.Scan(&u.DeviceID, &u.DeviceName, &u.App, &u.User, &u.Installed,
			&u.LastUsed, &u.DaysUsed, &u.Minutes, &u.ActiveSeconds, &u.Publisher, &u.Source)
	})
}

// FleetAppRow totals one app across every user and device. For an app no one
// used in the period, LastUsed is the most recent use ever (nil if never).
type FleetAppRow struct {
	App           string     `json:"app"`
	Users         int        `json:"users"`
	Devices       int        `json:"devices"`      // devices it was used on in the period
	InstalledOn   int        `json:"installed_on"` // devices it is installed on now
	LastUsed      *time.Time `json:"last_used"`
	Minutes       int        `json:"minutes"`
	ActiveSeconds int        `json:"active_seconds"`
	Publisher     string     `json:"-"`
	Source        string     `json:"-"`
}

// FleetUsageByApp totals usage per app (case-insensitive name) across the
// org, alongside how many devices have it installed. Most active first.
func (s *Store) FleetUsageByApp(ctx context.Context, orgID string, f FleetUsageFilter) ([]FleetAppRow, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH used AS (
			SELECT lower(u.app) AS k, min(u.app) AS app, count(DISTINCT u.username) AS users,
				count(DISTINCT u.device_id) AS devices, max(u.last_used) AS last_used,
				sum(u.minutes) AS minutes, sum(u.active_seconds) AS active
			FROM app_usage u JOIN devices d ON d.id = u.device_id AND d.retired_at IS NULL
			WHERE u.org_id = $1 AND u.day >= $2
			GROUP BY lower(u.app)
		), installed AS (
			SELECT lower(s.name) AS k, min(s.name) AS app, count(DISTINCT s.device_id) AS installed_on,
				min(s.publisher) AS publisher, min(s.source) AS source
			FROM device_software s JOIN devices d ON d.id = s.device_id AND d.retired_at IS NULL
			WHERE s.org_id = $1
			GROUP BY lower(s.name)
		), ever AS (
			SELECT lower(app) AS k, max(last_used) AS last_used FROM app_usage WHERE org_id = $1 GROUP BY lower(app)
		)
		SELECT COALESCE(u.app, i.app), COALESCE(u.users, 0), COALESCE(u.devices, 0), COALESCE(i.installed_on, 0),
			COALESCE(u.last_used, e.last_used), COALESCE(u.minutes, 0), COALESCE(u.active, 0),
			COALESCE(i.publisher, ''), COALESCE(i.source, '')
		FROM installed i
		FULL OUTER JOIN used u ON u.k = i.k
		LEFT JOIN ever e ON e.k = COALESCE(u.k, i.k)
		WHERE ($3 = '' OR COALESCE(u.k, i.k) LIKE '%' || lower($3) || '%')
			AND (CASE WHEN $4 THEN u.k IS NULL ELSE u.k IS NOT NULL OR $3 <> '' END)
		ORDER BY COALESCE(u.active, 0) DESC, COALESCE(u.minutes, 0) DESC, COALESCE(u.k, i.k)
		LIMIT 10000`, orgID, f.Since, f.App, f.Unused)
	return collect(rows, err, func(r pgx.Rows) (FleetAppRow, error) {
		var u FleetAppRow
		return u, r.Scan(&u.App, &u.Users, &u.Devices, &u.InstalledOn, &u.LastUsed, &u.Minutes, &u.ActiveSeconds, &u.Publisher, &u.Source)
	})
}

// OnlineDeviceIDs lists the online devices of an org (for policy re-push).
func (s *Store) OnlineDeviceIDs(ctx context.Context, orgID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id FROM devices WHERE org_id = $1 AND status = 'online' AND retired_at IS NULL`, orgID)
	return collect(rows, err, func(r pgx.Rows) (string, error) {
		var id string
		return id, r.Scan(&id)
	})
}

// UsageSettingsForDevice returns the usage-tracking switch and idle
// threshold (seconds) of a device's org.
func (s *Store) UsageSettingsForDevice(ctx context.Context, deviceID string) (bool, int, error) {
	var on bool
	var idle int
	err := s.Pool.QueryRow(ctx, `
		SELECT o.usage_tracking, o.idle_threshold_sec FROM devices d JOIN organizations o ON o.id = d.org_id WHERE d.id = $1`,
		deviceID).Scan(&on, &idle)
	return on, idle, mapErr(err)
}

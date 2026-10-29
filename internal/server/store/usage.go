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

// RecordUsage adds reported minutes to the per-day totals. A day never
// exceeds 1440 minutes per (app, user), whatever the agent sends.
func (s *Store) RecordUsage(ctx context.Context, orgID, deviceID string, entries []protocol.UsageEntry) error {
	b := &pgx.Batch{}
	for _, e := range entries {
		day, err := time.Parse(time.DateOnly, e.Date)
		if err != nil || e.App == "" || e.User == "" || e.Minutes <= 0 {
			continue
		}
		last := e.LastSeen
		if last.IsZero() || last.After(time.Now().Add(5*time.Minute)) {
			last = time.Now()
		}
		b.Queue(`
			INSERT INTO app_usage (device_id, org_id, day, app, username, minutes, last_used)
			VALUES ($1, $2, $3, $4, $5, LEAST($6::int, 1440), $7)
			ON CONFLICT (device_id, day, app, username) DO UPDATE SET
				minutes = LEAST(app_usage.minutes + EXCLUDED.minutes, 1440),
				last_used = GREATEST(app_usage.last_used, EXCLUDED.last_used)`,
			deviceID, orgID, day, clip(e.App, 256), clip(e.User, 256), e.Minutes, last)
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

// UsageRow summarizes one app's use by one user over a period.
type UsageRow struct {
	App      string    `json:"app"`
	User     string    `json:"user"`
	LastUsed time.Time `json:"last_used"`
	DaysUsed int       `json:"days_used"`
	Minutes  int       `json:"minutes"`
}

// DeviceUsage returns per (app, user) totals for a device since a day.
func (s *Store) DeviceUsage(ctx context.Context, orgID, deviceID string, since time.Time) ([]UsageRow, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT app, username, max(last_used), count(DISTINCT day), sum(minutes)
		FROM app_usage WHERE org_id = $1 AND device_id = $2 AND day >= $3
		GROUP BY app, username ORDER BY max(last_used) DESC`, orgID, deviceID, since)
	return collect(rows, err, func(r pgx.Rows) (UsageRow, error) {
		var u UsageRow
		return u, r.Scan(&u.App, &u.User, &u.LastUsed, &u.DaysUsed, &u.Minutes)
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
	DeviceID   string     `json:"device_id"`
	DeviceName string     `json:"device_name"`
	App        string     `json:"app"`
	User       string     `json:"user"`
	Installed  bool       `json:"installed"`
	LastUsed   *time.Time `json:"last_used"`
	DaysUsed   int        `json:"days_used"`
	Minutes    int        `json:"minutes"`
	Publisher  string     `json:"-"`
	Source     string     `json:"-"`
}

// FleetUsage joins installed software with usage so installed-but-unused
// apps are reported alongside used ones.
func (s *Store) FleetUsage(ctx context.Context, orgID string, f FleetUsageFilter) ([]FleetUsageRow, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH used AS (
			SELECT device_id, app, username, max(last_used) AS last_used,
				count(DISTINCT day) AS days, sum(minutes) AS minutes
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
			COALESCE(u.last_used, e.last_used), COALESCE(u.days, 0), COALESCE(u.minutes, 0),
			COALESCE(i.publisher, ''), COALESCE(i.source, '')
		FROM installed i
		FULL OUTER JOIN used u ON u.device_id = i.device_id AND lower(u.app) = lower(i.name)
		JOIN devices d ON d.id = COALESCE(i.device_id, u.device_id) AND d.retired_at IS NULL
		LEFT JOIN ever e ON e.device_id = d.id AND e.app = lower(COALESCE(u.app, i.name))
		WHERE ($3 = '' OR lower(COALESCE(u.app, i.name)) LIKE '%' || lower($3) || '%')
			AND (CASE WHEN $4 THEN u.app IS NULL ELSE u.app IS NOT NULL OR $3 <> '' END)
		ORDER BY lower(COALESCE(u.app, i.name)), d.name, u.username
		LIMIT 10000`, orgID, f.Since, f.App, f.Unused)
	return collect(rows, err, func(r pgx.Rows) (FleetUsageRow, error) {
		var u FleetUsageRow
		return u, r.Scan(&u.DeviceID, &u.DeviceName, &u.App, &u.User, &u.Installed,
			&u.LastUsed, &u.DaysUsed, &u.Minutes, &u.Publisher, &u.Source)
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

// UsageTrackingForDevice reports the usage-tracking setting of a device's org.
func (s *Store) UsageTrackingForDevice(ctx context.Context, deviceID string) (bool, error) {
	var on bool
	err := s.Pool.QueryRow(ctx, `
		SELECT o.usage_tracking FROM devices d JOIN organizations o ON o.id = d.org_id WHERE d.id = $1`, deviceID).Scan(&on)
	return on, mapErr(err)
}

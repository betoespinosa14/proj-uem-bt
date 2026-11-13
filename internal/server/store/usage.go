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
	App      string    // substring match on the app name
	Since    time.Time // start of the period
	Unused   bool      // only installed apps with no use in the period
	GroupID  string    // only devices in this group ("" for all)
	DeviceID string    // only this device ("" for all)
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
		JOIN devices d ON d.id = COALESCE(i.device_id, u.device_id) AND d.retired_at IS NULL AND ($5 = '' OR d.group_id::text = $5) AND ($6 = '' OR d.id::text = $6)
		LEFT JOIN ever e ON e.device_id = d.id AND e.app = lower(COALESCE(u.app, i.name))
		WHERE ($3 = '' OR lower(COALESCE(u.app, i.name)) LIKE '%' || lower($3) || '%')
			AND (CASE WHEN $4 THEN u.app IS NULL ELSE u.app IS NOT NULL OR $3 <> '' END)
		ORDER BY lower(COALESCE(u.app, i.name)), COALESCE(u.active, 0) DESC, d.name, u.username
		LIMIT 10000`, orgID, f.Since, f.App, f.Unused, f.GroupID, f.DeviceID)
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
			FROM app_usage u JOIN devices d ON d.id = u.device_id AND d.retired_at IS NULL AND ($5 = '' OR d.group_id::text = $5) AND ($6 = '' OR d.id::text = $6)
			WHERE u.org_id = $1 AND u.day >= $2
			GROUP BY lower(u.app)
		), installed AS (
			SELECT lower(s.name) AS k, min(s.name) AS app, count(DISTINCT s.device_id) AS installed_on,
				min(s.publisher) AS publisher, min(s.source) AS source
			FROM device_software s JOIN devices d ON d.id = s.device_id AND d.retired_at IS NULL AND ($5 = '' OR d.group_id::text = $5) AND ($6 = '' OR d.id::text = $6)
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
		LIMIT 10000`, orgID, f.Since, f.App, f.Unused, f.GroupID, f.DeviceID)
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

// DeviceOrgSettings are the org settings that shape a device's policy.
type DeviceOrgSettings struct {
	UsageTracking    bool
	IdleThresholdSec int
	ServerURL        string
}

// OrgSettingsForDevice returns the policy-relevant settings of a device's org.
func (s *Store) OrgSettingsForDevice(ctx context.Context, deviceID string) (DeviceOrgSettings, error) {
	var o DeviceOrgSettings
	err := s.Pool.QueryRow(ctx, `
		SELECT o.usage_tracking, o.idle_threshold_sec, o.server_url
		FROM devices d JOIN organizations o ON o.id = d.org_id WHERE d.id = $1`,
		deviceID).Scan(&o.UsageTracking, &o.IdleThresholdSec, &o.ServerURL)
	return o, mapErr(err)
}

// UsageSummary is the fleet usage dashboard for a period.
//
// Use is split by behaviour, per (device, app, user): it is foreground use if
// someone was active in the app at least once in the period, and
// background-only otherwise. Background-only processes (system daemons,
// helpers, remote-access hosts, apps under service accounts) run for days and
// would swamp open time, so they are totalled and listed separately instead
// of counted as use.
type UsageSummary struct {
	Totals     UsageTotals       `json:"totals"`
	Daily      []UsageDay        `json:"daily"`
	TopApps    []UsageAppStat    `json:"top_apps"`
	TopUsers   []UsageUser       `json:"top_users"`
	IdleApps   []UsageAppStat    `json:"idle_apps"`
	Background []UsageBackground `json:"background"`
	// Apps marked as infrastructure: their active time still counts, but
	// their open time is left out of open totals, idle_apps and background.
	Infrastructure []UsageAppStat `json:"infrastructure"`
}

type UsageTotals struct {
	ActiveSeconds        int `json:"active_seconds"`
	OpenMinutes          int `json:"open_minutes"` // foreground use only
	AppsUsed             int `json:"apps_used"`
	ActiveUsers          int `json:"active_users"`
	Devices              int `json:"devices"`
	BackgroundMinutes    int `json:"background_minutes"`
	BackgroundApps       int `json:"background_apps"`
	BackgroundThirdParty int `json:"background_third_party"` // third-party rows in Background; set by the caller
	UnusedApps           int `json:"unused_apps"`            // set by the caller (third-party apps only)
}

type UsageDay struct {
	Day           string `json:"day"` // YYYY-MM-DD, device-local
	ActiveSeconds int    `json:"active_seconds"`
	Minutes       int    `json:"minutes"` // foreground open time
	Devices       int    `json:"devices"` // devices that reported anything that day; 0 means no data
	Apps          int    `json:"apps"`    // distinct apps used in the foreground that day
}

type UsageAppStat struct {
	App           string `json:"app"`
	ActiveSeconds int    `json:"active_seconds"`
	Minutes       int    `json:"minutes"`
	Users         int    `json:"users"`
	Devices       int    `json:"devices"`
}

type UsageUser struct {
	User          string   `json:"user"`
	ActiveSeconds int      `json:"active_seconds"`
	OpenMinutes   int      `json:"open_minutes"`
	Apps          int      `json:"apps"`
	Devices       int      `json:"devices"`
	DeviceNames   []string `json:"device_names"` // sorted
	// Employee is the employee name of the device where this account was
	// most active ("" if that device has none), for display in place of the account.
	Employee string `json:"employee"`
}

type UsageBackground struct {
	App        string `json:"app"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	User       string `json:"user"`
	Minutes    int    `json:"minutes"`
	ThirdParty bool   `json:"third_party"` // set by the caller from Publisher/Source
	Publisher  string `json:"-"`
	Source     string `json:"-"`
}

// usageRows is the org's usage since $2 on devices that aren't retired,
// optionally only apps whose name contains $3, devices in group $4 and device $5, with each row marked fg when
// its (device, app, user) had any active time in the period, and infra when
// an admin marked the app as infrastructure.
const usageRows = `
	WITH scoped AS (
		SELECT u.*, d.name AS device_name, d.employee_name
		FROM app_usage u JOIN devices d ON d.id = u.device_id AND d.retired_at IS NULL AND ($4 = '' OR d.group_id::text = $4) AND ($5 = '' OR d.id::text = $5)
		WHERE u.org_id = $1 AND u.day >= $2 AND ($3 = '' OR lower(u.app) LIKE '%' || lower($3) || '%')
	), u AS (
		SELECT s.*, sum(s.active_seconds) OVER (PARTITION BY s.device_id, lower(s.app), s.username) > 0 AS fg,
			lower(s.app) IN (SELECT lower(app) FROM usage_infrastructure_apps WHERE org_id = $1) AS infra
		FROM scoped s
	)`

const usageTopN = 10
const usageBackgroundN = 40

// UsageSummary totals usage for the dashboard view.
func (s *Store) UsageSummary(ctx context.Context, orgID string, f FleetUsageFilter) (UsageSummary, error) {
	since, app, group, dev := f.Since, f.App, f.GroupID, f.DeviceID
	var out UsageSummary
	t := &out.Totals
	err := s.Pool.QueryRow(ctx, usageRows+`
		SELECT COALESCE(sum(active_seconds), 0), COALESCE(sum(minutes) FILTER (WHERE fg AND NOT infra), 0),
			count(DISTINCT lower(app)) FILTER (WHERE fg), count(DISTINCT username) FILTER (WHERE fg),
			count(DISTINCT device_id), COALESCE(sum(minutes) FILTER (WHERE NOT fg AND NOT infra), 0),
			count(DISTINCT lower(app)) FILTER (WHERE NOT fg AND NOT infra)
		FROM u`, orgID, since, app, group, dev).
		Scan(&t.ActiveSeconds, &t.OpenMinutes, &t.AppsUsed, &t.ActiveUsers, &t.Devices, &t.BackgroundMinutes, &t.BackgroundApps)
	if err != nil {
		return out, err
	}

	rows, err := s.Pool.Query(ctx, usageRows+`
		SELECT to_char(day, 'YYYY-MM-DD'), sum(active_seconds), COALESCE(sum(minutes) FILTER (WHERE fg AND NOT infra), 0), count(DISTINCT device_id),
			count(DISTINCT lower(app)) FILTER (WHERE fg)
		FROM u GROUP BY day ORDER BY day`, orgID, since, app, group, dev)
	if out.Daily, err = collect(rows, err, func(r pgx.Rows) (UsageDay, error) {
		var d UsageDay
		return d, r.Scan(&d.Day, &d.ActiveSeconds, &d.Minutes, &d.Devices, &d.Apps)
	}); err != nil {
		return out, err
	}

	// Per app, foreground use only.
	apps := func(where, order string, limit int) ([]UsageAppStat, error) {
		rows, err := s.Pool.Query(ctx, usageRows+`
			SELECT min(app), sum(active_seconds), sum(minutes), count(DISTINCT username), count(DISTINCT device_id)
			FROM u WHERE `+where+` GROUP BY lower(app) ORDER BY `+order+`, min(app) LIMIT $6`, orgID, since, app, group, dev, limit)
		return collect(rows, err, func(r pgx.Rows) (UsageAppStat, error) {
			var a UsageAppStat
			return a, r.Scan(&a.App, &a.ActiveSeconds, &a.Minutes, &a.Users, &a.Devices)
		})
	}
	if out.TopApps, err = apps("fg", "sum(active_seconds) DESC", usageTopN); err != nil {
		return out, err
	}
	// Most idle first: open the longest while not being used. Infrastructure
	// apps are open by design, so they are listed on their own instead.
	idle := "greatest(sum(minutes) * 60 - sum(active_seconds), 0) DESC"
	if out.IdleApps, err = apps("fg AND NOT infra", idle, usageTopN); err != nil {
		return out, err
	}
	if out.Infrastructure, err = apps("infra", "sum(minutes) DESC", 100); err != nil {
		return out, err
	}

	rows, err = s.Pool.Query(ctx, usageRows+`
		SELECT username, sum(active_seconds), COALESCE(sum(minutes) FILTER (WHERE NOT infra), 0), count(DISTINCT lower(app)), count(DISTINCT device_id),
			array_agg(DISTINCT device_name ORDER BY device_name),
			COALESCE((array_agg(employee_name ORDER BY active_seconds DESC) FILTER (WHERE employee_name <> ''))[1], '')
		FROM u WHERE fg GROUP BY username ORDER BY sum(active_seconds) DESC, username LIMIT $6`, orgID, since, app, group, dev, usageTopN)
	if out.TopUsers, err = collect(rows, err, func(r pgx.Rows) (UsageUser, error) {
		var u UsageUser
		return u, r.Scan(&u.User, &u.ActiveSeconds, &u.OpenMinutes, &u.Apps, &u.Devices, &u.DeviceNames, &u.Employee)
	}); err != nil {
		return out, err
	}

	rows, err = s.Pool.Query(ctx, usageRows+`
		, bg AS (
			SELECT min(app) AS app, device_id, min(device_name) AS device_name, username, sum(minutes) AS minutes
			FROM u WHERE NOT fg AND NOT infra GROUP BY device_id, lower(app), username
			ORDER BY sum(minutes) DESC, min(app) LIMIT $6
		)
		SELECT bg.app, bg.device_id, bg.device_name, bg.username, bg.minutes, COALESCE(sw.publisher, ''), COALESCE(sw.source, '')
		FROM bg LEFT JOIN LATERAL (
			SELECT publisher, source FROM device_software
			WHERE device_id = bg.device_id AND lower(name) = lower(bg.app) LIMIT 1
		) sw ON true
		ORDER BY bg.minutes DESC, bg.app`, orgID, since, app, group, dev, usageBackgroundN)
	out.Background, err = collect(rows, err, func(r pgx.Rows) (UsageBackground, error) {
		var b UsageBackground
		return b, r.Scan(&b.App, &b.DeviceID, &b.DeviceName, &b.User, &b.Minutes, &b.Publisher, &b.Source)
	})
	return out, err
}

// InfrastructureApps lists the apps an org marked as infrastructure.
type InfrastructureApp struct {
	App       string    `json:"app"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) InfrastructureApps(ctx context.Context, orgID string) ([]InfrastructureApp, error) {
	rows, err := s.Pool.Query(ctx, `SELECT app, created_at FROM usage_infrastructure_apps WHERE org_id = $1 ORDER BY lower(app)`, orgID)
	return collect(rows, err, func(r pgx.Rows) (InfrastructureApp, error) {
		var a InfrastructureApp
		return a, r.Scan(&a.App, &a.CreatedAt)
	})
}

// MarkInfrastructure marks an app as infrastructure; marking it again is a no-op.
func (s *Store) MarkInfrastructure(ctx context.Context, orgID, app, userID string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO usage_infrastructure_apps (org_id, app, created_by) VALUES ($1, $2, NULLIF($3, '')::uuid)
		ON CONFLICT (org_id, lower(app)) DO NOTHING`, orgID, clip(app, 256), userID)
	return mapErr(err)
}

// UnmarkInfrastructure reports whether the app was marked.
func (s *Store) UnmarkInfrastructure(ctx context.Context, orgID, app string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM usage_infrastructure_apps WHERE org_id = $1 AND lower(app) = lower($2)`, orgID, app)
	return tag.RowsAffected() > 0, mapErr(err)
}

// ReclaimRow is one installed app on one device with no use in the period:
// a candidate license to take back.
type ReclaimRow struct {
	App         string
	Publisher   string
	Version     string
	Source      string
	InstallDate string
	DeviceName  string
	DeviceUser  string
	DeviceSeen  *time.Time
	LastUsed    *time.Time // most recent use ever; nil if never
}

// ReclaimList returns installed apps with no use since a day, per device, on
// devices that aren't retired. The caller keeps third-party apps only.
func (s *Store) ReclaimList(ctx context.Context, orgID string, f FleetUsageFilter) ([]ReclaimRow, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT ON (lower(sw.name), d.name, d.id) sw.name, COALESCE(sw.publisher, ''), COALESCE(sw.version, ''), COALESCE(sw.source, ''),
			COALESCE(sw.install_date::text, ''), d.name, COALESCE(d.logged_in_user, ''), d.last_seen_at,
			(SELECT max(last_used) FROM app_usage e WHERE e.device_id = d.id AND lower(e.app) = lower(sw.name))
		FROM device_software sw JOIN devices d ON d.id = sw.device_id AND d.retired_at IS NULL AND ($4 = '' OR d.group_id::text = $4) AND ($5 = '' OR d.id::text = $5)
		WHERE sw.org_id = $1 AND ($3 = '' OR lower(sw.name) LIKE '%' || lower($3) || '%')
			AND NOT EXISTS (SELECT 1 FROM app_usage u WHERE u.device_id = d.id AND lower(u.app) = lower(sw.name) AND u.day >= $2)
		ORDER BY lower(sw.name), d.name, d.id
		LIMIT 50000`, orgID, f.Since, f.App, f.GroupID, f.DeviceID)
	return collect(rows, err, func(r pgx.Rows) (ReclaimRow, error) {
		var x ReclaimRow
		return x, r.Scan(&x.App, &x.Publisher, &x.Version, &x.Source, &x.InstallDate, &x.DeviceName, &x.DeviceUser, &x.DeviceSeen, &x.LastUsed)
	})
}

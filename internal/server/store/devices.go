package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/uem/uem/internal/protocol"
)

// ---- Groups ----

const groupSelect = `
	SELECT g.id, g.name, g.description, g.blocked_apps, g.created_at,
	       (SELECT count(*) FROM devices d WHERE d.group_id = g.id AND d.retired_at IS NULL)
	FROM device_groups g`

func scanGroup(r pgx.Row) (Group, error) {
	var g Group
	err := r.Scan(&g.ID, &g.Name, &g.Description, &g.BlockedApps, &g.CreatedAt, &g.DeviceCount)
	g.BlockedApps = nonNil(g.BlockedApps)
	return g, err
}

func (s *Store) ListGroups(ctx context.Context, orgID string) ([]Group, error) {
	rows, err := s.Pool.Query(ctx, groupSelect+` WHERE g.org_id = $1 ORDER BY g.name`, orgID)
	return collect(rows, err, func(r pgx.Rows) (Group, error) { return scanGroup(r) })
}

func (s *Store) GetGroup(ctx context.Context, orgID, id string) (Group, error) {
	g, err := scanGroup(s.Pool.QueryRow(ctx, groupSelect+` WHERE g.org_id = $1 AND g.id = $2`, orgID, id))
	return g, mapErr(err)
}

func (s *Store) CreateGroup(ctx context.Context, orgID, name, desc string, blocked []string) (Group, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO device_groups (org_id, name, description, blocked_apps) VALUES ($1, $2, $3, $4) RETURNING id`,
		orgID, name, desc, nonNil(blocked)).Scan(&id)
	if err != nil {
		return Group{}, mapErr(err)
	}
	return s.GetGroup(ctx, orgID, id)
}

func (s *Store) UpdateGroup(ctx context.Context, orgID, id string, name, desc *string, blocked []string) (Group, error) {
	err := expectOne(s.Pool.Exec(ctx, `
		UPDATE device_groups SET name = COALESCE($3, name), description = COALESCE($4, description),
		       blocked_apps = COALESCE($5, blocked_apps)
		WHERE org_id = $1 AND id = $2`, orgID, id, name, desc, blocked))
	if err != nil {
		return Group{}, err
	}
	return s.GetGroup(ctx, orgID, id)
}

func (s *Store) DeleteGroup(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `DELETE FROM device_groups WHERE org_id = $1 AND id = $2`, orgID, id))
}

// GroupBelongs verifies a client-supplied group ID is in the caller's org.
func (s *Store) GroupBelongs(ctx context.Context, orgID, groupID string) error {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT true FROM device_groups WHERE org_id = $1 AND id = $2`, orgID, groupID).Scan(&ok)
	return mapErr(err)
}

// ---- Enrollment tokens ----

const tokenSelect = `
	SELECT t.id, t.org_id, t.token, t.description, t.group_id, g.name, t.expires_at, t.max_uses, t.uses, t.revoked, t.created_at
	FROM enrollment_tokens t LEFT JOIN device_groups g ON g.id = t.group_id`

func scanToken(r pgx.Row) (EnrollmentToken, error) {
	var t EnrollmentToken
	err := r.Scan(&t.ID, &t.OrgID, &t.Token, &t.Description, &t.GroupID, &t.GroupName, &t.ExpiresAt, &t.MaxUses, &t.Uses, &t.Revoked, &t.CreatedAt)
	return t, err
}

func (s *Store) ListTokens(ctx context.Context, orgID string) ([]EnrollmentToken, error) {
	rows, err := s.Pool.Query(ctx, tokenSelect+` WHERE t.org_id = $1 ORDER BY t.created_at DESC`, orgID)
	return collect(rows, err, func(r pgx.Rows) (EnrollmentToken, error) { return scanToken(r) })
}

func (s *Store) GetToken(ctx context.Context, orgID, id string) (EnrollmentToken, error) {
	t, err := scanToken(s.Pool.QueryRow(ctx, tokenSelect+` WHERE t.org_id = $1 AND t.id = $2`, orgID, id))
	return t, mapErr(err)
}

func (s *Store) CreateToken(ctx context.Context, orgID, token, desc string, groupID *string, expiresAt *time.Time, maxUses *int, createdBy string) (EnrollmentToken, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO enrollment_tokens (org_id, token, description, group_id, expires_at, max_uses, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		orgID, token, desc, groupID, expiresAt, maxUses, createdBy).Scan(&id)
	if err != nil {
		return EnrollmentToken{}, mapErr(err)
	}
	return s.GetToken(ctx, orgID, id)
}

func (s *Store) RevokeToken(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `UPDATE enrollment_tokens SET revoked = true WHERE org_id = $1 AND id = $2`, orgID, id))
}

// ErrTokenInvalid covers unknown, revoked, expired, and exhausted tokens alike
// so enrollment failures leak nothing about which tokens exist.
var ErrTokenInvalid = errors.New("enrollment token is invalid or expired")

// EnrollDevice atomically consumes one use of the token and creates the device.
func (s *Store) EnrollDevice(ctx context.Context, token string, facts protocol.Facts, secretHash string) (Device, error) {
	var deviceID, orgID string
	err := s.InTx(ctx, func(tx pgx.Tx) error {
		t, err := scanToken(tx.QueryRow(ctx, tokenSelect+` WHERE t.token = $1 FOR UPDATE OF t`, token))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrTokenInvalid
			}
			return err
		}
		if t.Revoked || (t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now())) || (t.MaxUses != nil && t.Uses >= *t.MaxUses) {
			return ErrTokenInvalid
		}
		if _, err := tx.Exec(ctx, `UPDATE enrollment_tokens SET uses = uses + 1 WHERE id = $1`, t.ID); err != nil {
			return err
		}
		orgID = t.OrgID
		name := facts.Hostname
		if name == "" {
			name = "unnamed-device"
		}
		return tx.QueryRow(ctx, `
			INSERT INTO devices (org_id, group_id, secret_hash, name, hostname, os_family, os_name, os_version, arch, serial, agent_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			t.OrgID, t.GroupID, secretHash, name, facts.Hostname, facts.OSFamily, facts.OSName, facts.OSVersion,
			facts.Arch, facts.Serial, facts.AgentVersion).Scan(&deviceID)
	})
	if err != nil {
		if errors.Is(err, ErrTokenInvalid) {
			return Device{}, err
		}
		return Device{}, mapErr(err)
	}
	return s.GetDevice(ctx, orgID, deviceID)
}

// ---- Devices ----

const deviceSelect = `
	SELECT d.id, d.org_id, d.name, d.hostname, d.os_family, d.os_name, d.os_version, d.os_build, d.arch,
	       d.serial, d.model, d.manufacturer, d.agent_version, d.ip_address, d.mac_address, d.logged_in_user,
	       d.status, d.last_seen_at, d.enrolled_at, d.compliance_state, d.group_id, g.name, d.tags,
	       d.cpu_pct, d.mem_pct, d.disk_pct, d.battery_pct
	FROM devices d LEFT JOIN device_groups g ON g.id = d.group_id`

func scanDevice(r pgx.Row) (Device, error) {
	var d Device
	err := r.Scan(&d.ID, &d.OrgID, &d.Name, &d.Hostname, &d.OSFamily, &d.OSName, &d.OSVersion, &d.OSBuild, &d.Arch,
		&d.Serial, &d.Model, &d.Manufacturer, &d.AgentVersion, &d.IPAddress, &d.MACAddress, &d.LoggedInUser,
		&d.Status, &d.LastSeenAt, &d.EnrolledAt, &d.ComplianceState, &d.GroupID, &d.GroupName, &d.Tags,
		&d.CPUPct, &d.MemPct, &d.DiskPct, &d.BatteryPct)
	d.Tags = nonNil(d.Tags)
	return d, err
}

type DeviceFilter struct {
	Query      string
	Status     string
	OSFamily   string
	Compliance string
	GroupID    string
	IDs        []string
}

func (s *Store) ListDevices(ctx context.Context, orgID string, f DeviceFilter) ([]Device, error) {
	where := []string{"d.org_id = $1", "d.retired_at IS NULL"}
	args := []any{orgID}
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Query != "" {
		add(`(d.name ILIKE $%[1]d OR d.hostname ILIKE $%[1]d OR d.serial ILIKE $%[1]d OR d.ip_address ILIKE $%[1]d
		      OR d.logged_in_user ILIKE $%[1]d OR array_to_string(d.tags, ' ') ILIKE $%[1]d)`,
			"%"+escapeLike(f.Query)+"%")
	}
	if f.Status != "" {
		add("d.status = $%d", f.Status)
	}
	if f.OSFamily != "" {
		add("d.os_family = $%d", f.OSFamily)
	}
	if f.Compliance != "" {
		add("d.compliance_state = $%d", f.Compliance)
	}
	if f.GroupID != "" {
		add("d.group_id::text = $%d", f.GroupID)
	}
	if f.IDs != nil {
		add("d.id::text = ANY($%d)", f.IDs)
	}
	q := deviceSelect + " WHERE " + strings.Join(where, " AND ") + " ORDER BY d.name, d.id"
	rows, err := s.Pool.Query(ctx, q, args...)
	return collect(rows, err, func(r pgx.Rows) (Device, error) { return scanDevice(r) })
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *Store) GetDevice(ctx context.Context, orgID, id string) (Device, error) {
	d, err := scanDevice(s.Pool.QueryRow(ctx, deviceSelect+` WHERE d.org_id = $1 AND d.id = $2 AND d.retired_at IS NULL`, orgID, id))
	return d, mapErr(err)
}

func (s *Store) GetDeviceDetail(ctx context.Context, orgID, id string) (DeviceDetail, error) {
	d, err := s.GetDevice(ctx, orgID, id)
	if err != nil {
		return DeviceDetail{}, err
	}
	dd := DeviceDetail{Device: d, Inventory: json.RawMessage("null")}
	var inv []byte
	err = s.Pool.QueryRow(ctx, `SELECT data FROM device_inventory WHERE device_id = $1`, id).Scan(&inv)
	if err == nil {
		dd.Inventory = inv
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return dd, err
	}
	return dd, nil
}

// DeviceAuth is what the agent endpoint needs to authenticate a device.
type DeviceAuth struct {
	ID         string
	OrgID      string
	SecretHash string
	GroupID    *string
}

// GetDeviceAuth is an unscoped lookup by device ID used only to verify agent credentials.
func (s *Store) GetDeviceAuth(ctx context.Context, id string) (DeviceAuth, error) {
	var a DeviceAuth
	err := s.Pool.QueryRow(ctx, `SELECT id, org_id, secret_hash, group_id FROM devices WHERE id = $1 AND retired_at IS NULL`, id).
		Scan(&a.ID, &a.OrgID, &a.SecretHash, &a.GroupID)
	return a, mapErr(err)
}

func (s *Store) UpdateDevice(ctx context.Context, orgID, id string, name *string, groupID *string, clearGroup bool, tags []string) (Device, error) {
	err := expectOne(s.Pool.Exec(ctx, `
		UPDATE devices SET name = COALESCE($3, name),
		       group_id = CASE WHEN $5 THEN NULL ELSE COALESCE($4, group_id) END,
		       tags = COALESCE($6, tags)
		WHERE org_id = $1 AND id = $2 AND retired_at IS NULL`, orgID, id, name, groupID, clearGroup, tags))
	if err != nil {
		return Device{}, err
	}
	return s.GetDevice(ctx, orgID, id)
}

// AddDeviceTag adds a tag if not already present.
func (s *Store) AddDeviceTag(ctx context.Context, orgID, id, tag string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE devices SET tags = array_append(tags, $3)
		WHERE org_id = $1 AND id = $2 AND NOT ($3 = ANY(tags))`, orgID, id, tag)
	return err
}

// RetireDevice soft-deletes a device; its credential stops working immediately.
func (s *Store) RetireDevice(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `
		UPDATE devices SET retired_at = now(), status = 'offline', secret_hash = ''
		WHERE org_id = $1 AND id = $2 AND retired_at IS NULL`, orgID, id))
}

func (s *Store) SetDeviceOnline(ctx context.Context, id string, hello protocol.Hello) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE devices SET status = 'online', last_seen_at = now(),
		       agent_version = COALESCE(NULLIF($2, ''), agent_version),
		       hostname = COALESCE(NULLIF($3, ''), hostname)
		WHERE id = $1`, id, hello.AgentVersion, hello.Facts.Hostname)
	return err
}

func (s *Store) SetDeviceOffline(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE devices SET status = 'offline' WHERE id = $1`, id)
	return err
}

func (s *Store) TouchDevice(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE devices SET last_seen_at = now(), status = 'online' WHERE id = $1`, id)
	return err
}

// MarkStaleOffline flips devices whose last heartbeat is older than cutoff
// and returns their IDs so the caller can broadcast the change.
func (s *Store) MarkStaleOffline(ctx context.Context, cutoff time.Time) ([]DeviceRef, error) {
	rows, err := s.Pool.Query(ctx, `
		UPDATE devices SET status = 'offline'
		WHERE status = 'online' AND (last_seen_at IS NULL OR last_seen_at < $1)
		RETURNING id, org_id`, cutoff)
	return collect(rows, err, func(r pgx.Rows) (DeviceRef, error) {
		var d DeviceRef
		return d, r.Scan(&d.ID, &d.OrgID)
	})
}

// MarkAllOffline is run at startup: no agent is connected to a fresh process.
func (s *Store) MarkAllOffline(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE devices SET status = 'offline' WHERE status = 'online'`)
	return err
}

type DeviceRef struct {
	ID    string
	OrgID string
}

func (s *Store) RecordTelemetry(ctx context.Context, id string, t protocol.Telemetry) error {
	return s.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO device_telemetry (device_id, ts, cpu_pct, mem_pct, disk_pct, battery_pct)
			VALUES ($1, $2, $3, $4, $5, $6)`, id, t.TS, t.CPUPct, t.MemPct, t.DiskPct, t.BatteryPct); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE devices SET cpu_pct = $2, mem_pct = $3, disk_pct = $4, battery_pct = $5,
			       ip_address = COALESCE(NULLIF($6, ''), ip_address), logged_in_user = $7,
			       last_seen_at = now(), status = 'online'
			WHERE id = $1`, id, t.CPUPct, t.MemPct, t.DiskPct, t.BatteryPct, t.PrimaryIP, t.LoggedIn)
		return err
	})
}

func (s *Store) PurgeTelemetry(ctx context.Context, before time.Time) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM device_telemetry WHERE ts < $1`, before)
	return err
}

// SaveInventory stores the latest snapshot, replaces the software list and
// denormalizes headline fields onto the device row.
func (s *Store) SaveInventory(ctx context.Context, orgID, id string, inv protocol.Inventory) error {
	software := inv.Software
	inv.Software = nil // stored in its own table
	data, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	loggedIn := ""
	if len(inv.Users.LoggedIn) > 0 {
		loggedIn = inv.Users.LoggedIn[0]
	}
	return s.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO device_inventory (device_id, org_id, collected_at, data) VALUES ($1, $2, $3, $4)
			ON CONFLICT (device_id) DO UPDATE SET collected_at = EXCLUDED.collected_at, data = EXCLUDED.data`,
			id, orgID, inv.CollectedAt, data); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM device_software WHERE device_id = $1`, id); err != nil {
			return err
		}
		if len(software) > 0 {
			rows := make([][]any, 0, len(software))
			for _, sw := range software {
				rows = append(rows, []any{id, orgID, sw.Name, sw.Version, sw.Publisher, sw.Source, sw.InstallDate})
			}
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{"device_software"},
				[]string{"device_id", "org_id", "name", "version", "publisher", "source", "install_date"},
				pgx.CopyFromRows(rows)); err != nil {
				return err
			}
		}
		var diskPct *float64
		for _, d := range inv.Hardware.Disks {
			if d.TotalBytes > 0 && (d.Mount == "/" || strings.EqualFold(d.Mount, "C:") || strings.EqualFold(d.Mount, `C:\`)) {
				p := float64(d.UsedBytes) / float64(d.TotalBytes) * 100
				diskPct = &p
			}
		}
		_, err := tx.Exec(ctx, `
			UPDATE devices SET hostname = $2, os_family = $3, os_name = $4, os_version = $5, os_build = $6, arch = $7,
			       serial = $8, model = $9, manufacturer = $10, ip_address = $11, mac_address = $12,
			       logged_in_user = $13, disk_pct = COALESCE($14, disk_pct), last_seen_at = now(), status = 'online'
			WHERE id = $1`,
			id, inv.Network.Hostname, inv.OS.Family, inv.OS.Name, inv.OS.Version, inv.OS.Build, inv.OS.Arch,
			inv.Hardware.Serial, inv.Hardware.Model, inv.Hardware.Manufacturer, inv.Network.PrimaryIP,
			inv.Network.PrimaryMAC, loggedIn, diskPct)
		return err
	})
}

// GetInventory returns the latest stored inventory (without software).
func (s *Store) GetInventory(ctx context.Context, orgID, id string) (*protocol.Inventory, error) {
	var data []byte
	err := s.Pool.QueryRow(ctx, `SELECT data FROM device_inventory WHERE org_id = $1 AND device_id = $2`, orgID, id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var inv protocol.Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *Store) ListSoftware(ctx context.Context, orgID, id string) ([]Software, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT name, version, publisher, source, install_date FROM device_software
		WHERE org_id = $1 AND device_id = $2 ORDER BY lower(name)`, orgID, id)
	return collect(rows, err, func(r pgx.Rows) (Software, error) {
		var sw Software
		return sw, r.Scan(&sw.Name, &sw.Version, &sw.Publisher, &sw.Source, &sw.InstallDate)
	})
}

// ListOrgSoftware returns installed software for every device in an org,
// keyed by device ID.
func (s *Store) ListOrgSoftware(ctx context.Context, orgID string) (map[string][]Software, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT device_id, name, version, publisher, source, install_date FROM device_software
		WHERE org_id = $1 ORDER BY lower(name)`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]Software{}
	for rows.Next() {
		var id string
		var sw Software
		if err := rows.Scan(&id, &sw.Name, &sw.Version, &sw.Publisher, &sw.Source, &sw.InstallDate); err != nil {
			return nil, err
		}
		out[id] = append(out[id], sw)
	}
	return out, rows.Err()
}

func (s *Store) ListTelemetry(ctx context.Context, orgID, id string, since time.Time) ([]TelemetryPoint, error) {
	// The org check is on the device row; telemetry has no org column of its own.
	rows, err := s.Pool.Query(ctx, `
		SELECT t.ts, t.cpu_pct, t.mem_pct, t.disk_pct, t.battery_pct
		FROM device_telemetry t JOIN devices d ON d.id = t.device_id
		WHERE d.org_id = $1 AND t.device_id = $2 AND t.ts >= $3 ORDER BY t.ts`, orgID, id, since)
	return collect(rows, err, func(r pgx.Rows) (TelemetryPoint, error) {
		var p TelemetryPoint
		return p, r.Scan(&p.TS, &p.CPUPct, &p.MemPct, &p.DiskPct, &p.BatteryPct)
	})
}

func (s *Store) SetComplianceState(ctx context.Context, id, state string) (changed bool, err error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE devices SET compliance_state = $2 WHERE id = $1 AND compliance_state <> $2`, id, state)
	return tag.RowsAffected() > 0, err
}

// BlockedAppsForDevice returns the effective app blocklist from the device's group.
func (s *Store) BlockedAppsForDevice(ctx context.Context, id string) ([]string, error) {
	var apps []string
	err := s.Pool.QueryRow(ctx, `
		SELECT COALESCE(g.blocked_apps, '{}') FROM devices d LEFT JOIN device_groups g ON g.id = d.group_id
		WHERE d.id = $1`, id).Scan(&apps)
	return nonNil(apps), mapErr(err)
}

func (s *Store) Dashboard(ctx context.Context, orgID string) (Dashboard, error) {
	var d Dashboard
	err := s.Pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'online'),
		       count(*) FILTER (WHERE status = 'offline'),
		       count(*) FILTER (WHERE compliance_state = 'compliant'),
		       count(*) FILTER (WHERE compliance_state = 'non_compliant'),
		       count(*) FILTER (WHERE compliance_state = 'unknown'),
		       (SELECT count(*) FROM alerts WHERE org_id = $1 AND status = 'open')
		FROM devices WHERE org_id = $1 AND retired_at IS NULL`, orgID).
		Scan(&d.DevicesTotal, &d.Online, &d.Offline, &d.Compliant, &d.NonCompliant, &d.Unknown, &d.OpenAlerts)
	if err != nil {
		return d, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT COALESCE(NULLIF(os_family, ''), 'unknown'), count(*) FROM devices
		WHERE org_id = $1 AND retired_at IS NULL GROUP BY 1 ORDER BY 2 DESC`, orgID)
	if d.ByOS, err = collect(rows, err, func(r pgx.Rows) (OSCount, error) {
		var c OSCount
		return c, r.Scan(&c.OSFamily, &c.Count)
	}); err != nil {
		return d, err
	}
	if d.RecentAlerts, err = s.ListAlerts(ctx, orgID, "open", 8); err != nil {
		return d, err
	}
	d.RecentCommands, err = s.listCommands(ctx, `c.org_id = $1`, 8, orgID)
	return d, err
}

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Snapshot is one day's Dashboard figures for an organization or one department.
type Snapshot struct {
	Day            string `json:"day"` // YYYY-MM-DD, server local date
	Devices        int    `json:"devices"`
	Online         int    `json:"online"`
	Offline        int    `json:"offline"`
	Compliant      int    `json:"compliant"`
	NonCompliant   int    `json:"non_compliant"`
	OpenAlerts     int    `json:"open_alerts"`
	Reported       int    `json:"reported"`
	Encrypted      int    `json:"encrypted"`
	Firewall       int    `json:"firewall"`
	Antivirus      int    `json:"antivirus"`
	UpdatesPending int    `json:"updates_pending"`
	AgentCurrent   int    `json:"agent_current"`
	HealthIssues   int    `json:"health_issues"` // devices on the Device health card
}

// SnapshotOf takes the trend figures from a Dashboard.
func SnapshotOf(d Dashboard, agentCurrent int) Snapshot {
	devices := map[string]bool{}
	for _, h := range d.Health {
		devices[h.DeviceID] = true
	}
	return Snapshot{
		Devices: d.DevicesTotal, Online: d.Online, Offline: d.Offline, Compliant: d.Compliant,
		NonCompliant: d.NonCompliant, OpenAlerts: d.OpenAlerts, Reported: d.Posture.Reported,
		Encrypted: d.Posture.Encrypted, Firewall: d.Posture.Firewall, Antivirus: d.Posture.Antivirus,
		UpdatesPending: d.Posture.UpdatesPending, AgentCurrent: agentCurrent, HealthIssues: len(devices),
	}
}

// SaveSnapshot writes or replaces the snapshot for one organization, department ('' = all) and day.
func (s *Store) SaveSnapshot(ctx context.Context, orgID, groupKey string, day time.Time, n Snapshot) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO fleet_snapshots (org_id, group_key, day, devices, online, offline, compliant, non_compliant,
			open_alerts, reported, encrypted, firewall, antivirus, updates_pending, agent_current, health_issues, updated_at)
		VALUES ($1, $2, $3::date, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, now())
		ON CONFLICT (org_id, group_key, day) DO UPDATE SET
			devices = EXCLUDED.devices, online = EXCLUDED.online, offline = EXCLUDED.offline,
			compliant = EXCLUDED.compliant, non_compliant = EXCLUDED.non_compliant, open_alerts = EXCLUDED.open_alerts,
			reported = EXCLUDED.reported, encrypted = EXCLUDED.encrypted, firewall = EXCLUDED.firewall,
			antivirus = EXCLUDED.antivirus, updates_pending = EXCLUDED.updates_pending,
			agent_current = EXCLUDED.agent_current, health_issues = EXCLUDED.health_issues, updated_at = now()`,
		orgID, groupKey, day.Format(time.DateOnly), n.Devices, n.Online, n.Offline, n.Compliant, n.NonCompliant,
		n.OpenAlerts, n.Reported, n.Encrypted, n.Firewall, n.Antivirus, n.UpdatesPending, n.AgentCurrent, n.HealthIssues)
	return mapErr(err)
}

// ListSnapshots returns the snapshots from `since` (a date) onwards, oldest first.
func (s *Store) ListSnapshots(ctx context.Context, orgID, groupKey string, since time.Time) ([]Snapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT to_char(day, 'YYYY-MM-DD'), devices, online, offline, compliant, non_compliant, open_alerts, reported,
		       encrypted, firewall, antivirus, updates_pending, agent_current, health_issues
		FROM fleet_snapshots WHERE org_id = $1 AND group_key = $2 AND day >= $3::date ORDER BY day`,
		orgID, groupKey, since.Format(time.DateOnly))
	return collect(rows, err, func(r pgx.Rows) (Snapshot, error) {
		var n Snapshot
		return n, r.Scan(&n.Day, &n.Devices, &n.Online, &n.Offline, &n.Compliant, &n.NonCompliant, &n.OpenAlerts,
			&n.Reported, &n.Encrypted, &n.Firewall, &n.Antivirus, &n.UpdatesPending, &n.AgentCurrent, &n.HealthIssues)
	})
}

func (s *Store) PurgeSnapshots(ctx context.Context, before time.Time) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM fleet_snapshots WHERE day < $1::date`, before.Format(time.DateOnly))
	return err
}

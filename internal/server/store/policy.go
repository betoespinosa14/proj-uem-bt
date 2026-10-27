package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- Compliance rules ----

const ruleSelect = `
	SELECT r.id, r.org_id, r.name, r.description, r.enabled, r.severity, r.os_families, r.group_id, r.match,
	       r.conditions, r.actions, r.created_at, r.updated_at,
	       (SELECT count(*) FROM compliance_results cr JOIN devices d ON d.id = cr.device_id
	        WHERE cr.rule_id = r.id AND NOT cr.passed AND d.retired_at IS NULL)
	FROM compliance_rules r`

func scanRule(r pgx.Row) (Rule, error) {
	var x Rule
	err := r.Scan(&x.ID, &x.OrgID, &x.Name, &x.Description, &x.Enabled, &x.Severity, &x.OSFamilies, &x.GroupID, &x.Match,
		&x.Conditions, &x.Actions, &x.CreatedAt, &x.UpdatedAt, &x.ViolatingDevices)
	x.OSFamilies = nonNil(x.OSFamilies)
	if x.Conditions == nil {
		x.Conditions = []Condition{}
	}
	return x, err
}

func (s *Store) ListRules(ctx context.Context, orgID string) ([]Rule, error) {
	rows, err := s.Pool.Query(ctx, ruleSelect+` WHERE r.org_id = $1 ORDER BY r.name`, orgID)
	return collect(rows, err, func(r pgx.Rows) (Rule, error) { return scanRule(r) })
}

func (s *Store) GetRule(ctx context.Context, orgID, id string) (Rule, error) {
	x, err := scanRule(s.Pool.QueryRow(ctx, ruleSelect+` WHERE r.org_id = $1 AND r.id = $2`, orgID, id))
	return x, mapErr(err)
}

func (s *Store) CreateRule(ctx context.Context, orgID string, x Rule) (Rule, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO compliance_rules (org_id, name, description, enabled, severity, os_families, group_id, match, conditions, actions)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		orgID, x.Name, x.Description, x.Enabled, x.Severity, nonNil(x.OSFamilies), x.GroupID, x.Match, x.Conditions, x.Actions).Scan(&id)
	if err != nil {
		return Rule{}, mapErr(err)
	}
	return s.GetRule(ctx, orgID, id)
}

func (s *Store) UpdateRule(ctx context.Context, orgID string, x Rule) (Rule, error) {
	err := expectOne(s.Pool.Exec(ctx, `
		UPDATE compliance_rules SET name = $3, description = $4, enabled = $5, severity = $6, os_families = $7,
		       group_id = $8, match = $9, conditions = $10, actions = $11, updated_at = now()
		WHERE org_id = $1 AND id = $2`,
		orgID, x.ID, x.Name, x.Description, x.Enabled, x.Severity, nonNil(x.OSFamilies), x.GroupID, x.Match, x.Conditions, x.Actions))
	if err != nil {
		return Rule{}, err
	}
	return s.GetRule(ctx, orgID, x.ID)
}

func (s *Store) DeleteRule(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `DELETE FROM compliance_rules WHERE org_id = $1 AND id = $2`, orgID, id))
}

// SaveResult upserts a rule result and reports whether the device newly
// transitioned to failing this rule (passing or unevaluated before).
func (s *Store) SaveResult(ctx context.Context, orgID, deviceID, ruleID string, passed bool, details string) (newlyFailing bool, err error) {
	var prev *bool
	err = s.InTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT passed FROM compliance_results WHERE device_id = $1 AND rule_id = $2 FOR UPDATE`,
			deviceID, ruleID).Scan(&prev)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO compliance_results (device_id, rule_id, org_id, passed, details, evaluated_at)
			VALUES ($1, $2, $3, $4, $5, now())
			ON CONFLICT (device_id, rule_id) DO UPDATE SET passed = EXCLUDED.passed, details = EXCLUDED.details,
			       evaluated_at = EXCLUDED.evaluated_at`, deviceID, ruleID, orgID, passed, details)
		return err
	})
	return err == nil && !passed && (prev == nil || *prev), err
}

// DeleteResultsExcept drops results for rules that no longer apply to the device.
func (s *Store) DeleteResultsExcept(ctx context.Context, deviceID string, keepRuleIDs []string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM compliance_results WHERE device_id = $1 AND NOT (rule_id::text = ANY($2))`,
		deviceID, nonNil(keepRuleIDs))
	return err
}

func (s *Store) ListDeviceResults(ctx context.Context, orgID, deviceID string) ([]ComplianceResult, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id, r.name, r.severity, cr.passed, cr.details, cr.evaluated_at
		FROM compliance_results cr JOIN compliance_rules r ON r.id = cr.rule_id
		WHERE cr.org_id = $1 AND cr.device_id = $2 ORDER BY cr.passed, r.name`, orgID, deviceID)
	return collect(rows, err, func(r pgx.Rows) (ComplianceResult, error) {
		var c ComplianceResult
		return c, r.Scan(&c.RuleID, &c.RuleName, &c.Severity, &c.Passed, &c.Details, &c.EvaluatedAt)
	})
}

// ---- Scheduled tasks ----

const taskCols = `id, org_id, name, enabled, cron, target, action, last_run_at, created_at`

func scanTask(r pgx.Row) (Task, error) {
	var t Task
	err := r.Scan(&t.ID, &t.OrgID, &t.Name, &t.Enabled, &t.Cron, &t.Target, &t.Action, &t.LastRunAt, &t.CreatedAt)
	if t.Target.DeviceIDs == nil {
		t.Target.DeviceIDs = []string{}
	}
	return t, err
}

func (s *Store) ListTasks(ctx context.Context, orgID string) ([]Task, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+taskCols+` FROM scheduled_tasks WHERE org_id = $1 ORDER BY name`, orgID)
	return collect(rows, err, func(r pgx.Rows) (Task, error) { return scanTask(r) })
}

// ListAllEnabledTasks is used by the scheduler across every org.
func (s *Store) ListAllEnabledTasks(ctx context.Context) ([]Task, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+taskCols+` FROM scheduled_tasks WHERE enabled`)
	return collect(rows, err, func(r pgx.Rows) (Task, error) { return scanTask(r) })
}

func (s *Store) GetTask(ctx context.Context, orgID, id string) (Task, error) {
	t, err := scanTask(s.Pool.QueryRow(ctx, `SELECT `+taskCols+` FROM scheduled_tasks WHERE org_id = $1 AND id = $2`, orgID, id))
	return t, mapErr(err)
}

func (s *Store) CreateTask(ctx context.Context, orgID string, t Task) (Task, error) {
	out, err := scanTask(s.Pool.QueryRow(ctx, `
		INSERT INTO scheduled_tasks (org_id, name, enabled, cron, target, action) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+taskCols, orgID, t.Name, t.Enabled, t.Cron, t.Target, t.Action))
	return out, mapErr(err)
}

func (s *Store) UpdateTask(ctx context.Context, orgID string, t Task) (Task, error) {
	out, err := scanTask(s.Pool.QueryRow(ctx, `
		UPDATE scheduled_tasks SET name = $3, enabled = $4, cron = $5, target = $6, action = $7
		WHERE org_id = $1 AND id = $2 RETURNING `+taskCols, orgID, t.ID, t.Name, t.Enabled, t.Cron, t.Target, t.Action))
	return out, mapErr(err)
}

func (s *Store) DeleteTask(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `DELETE FROM scheduled_tasks WHERE org_id = $1 AND id = $2`, orgID, id))
}

func (s *Store) TouchTaskRun(ctx context.Context, id string, at time.Time) error {
	_, err := s.Pool.Exec(ctx, `UPDATE scheduled_tasks SET last_run_at = $2 WHERE id = $1`, id, at)
	return err
}

// ---- Alerts ----

const alertSelect = `
	SELECT a.id, a.org_id, a.device_id, d.name, a.rule_id, a.severity, a.title, a.message, a.status, a.created_at, a.resolved_at
	FROM alerts a LEFT JOIN devices d ON d.id = a.device_id`

func scanAlert(r pgx.Row) (Alert, error) {
	var a Alert
	err := r.Scan(&a.ID, &a.OrgID, &a.DeviceID, &a.DeviceName, &a.RuleID, &a.Severity, &a.Title, &a.Message, &a.Status, &a.CreatedAt, &a.ResolvedAt)
	return a, err
}

func (s *Store) ListAlerts(ctx context.Context, orgID, status string, limit int) ([]Alert, error) {
	rows, err := s.Pool.Query(ctx, alertSelect+`
		WHERE a.org_id = $1 AND ($2 = '' OR a.status = $2) ORDER BY a.created_at DESC LIMIT $3`, orgID, status, limit)
	return collect(rows, err, func(r pgx.Rows) (Alert, error) { return scanAlert(r) })
}

// CreateAlert opens an alert; returns created=false if an identical open
// alert for the same device and rule already exists.
func (s *Store) CreateAlert(ctx context.Context, orgID string, deviceID, ruleID *string, severity, title, message string) (Alert, bool, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO alerts (org_id, device_id, rule_id, severity, title, message) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (device_id, rule_id) WHERE status = 'open' AND rule_id IS NOT NULL DO NOTHING
		RETURNING id`, orgID, deviceID, ruleID, severity, title, message).Scan(&id)
	if err == pgx.ErrNoRows {
		return Alert{}, false, nil
	}
	if err != nil {
		return Alert{}, false, mapErr(err)
	}
	a, err := scanAlert(s.Pool.QueryRow(ctx, alertSelect+` WHERE a.id = $1`, id))
	return a, true, mapErr(err)
}

func (s *Store) ResolveAlert(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `
		UPDATE alerts SET status = 'resolved', resolved_at = now() WHERE org_id = $1 AND id = $2 AND status = 'open'`, orgID, id))
}

// AutoResolveAlert closes the open alert for a device+rule once it passes again.
func (s *Store) AutoResolveAlert(ctx context.Context, deviceID, ruleID string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE alerts SET status = 'resolved', resolved_at = now()
		WHERE device_id = $1 AND rule_id = $2 AND status = 'open'`, deviceID, ruleID)
	return err
}

// ---- Webhooks ----

const webhookCols = `id, org_id, name, url, secret, events, enabled, created_at, last_status, last_delivery_at`

func scanWebhook(r pgx.Row) (Webhook, error) {
	var w Webhook
	err := r.Scan(&w.ID, &w.OrgID, &w.Name, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.CreatedAt, &w.LastStatus, &w.LastDeliveryAt)
	w.Events = nonNil(w.Events)
	return w, err
}

func (s *Store) ListWebhooks(ctx context.Context, orgID string) ([]Webhook, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE org_id = $1 ORDER BY name`, orgID)
	return collect(rows, err, func(r pgx.Rows) (Webhook, error) { return scanWebhook(r) })
}

func (s *Store) GetWebhook(ctx context.Context, orgID, id string) (Webhook, error) {
	w, err := scanWebhook(s.Pool.QueryRow(ctx, `SELECT `+webhookCols+` FROM webhooks WHERE org_id = $1 AND id = $2`, orgID, id))
	return w, mapErr(err)
}

func (s *Store) CreateWebhook(ctx context.Context, orgID string, w Webhook) (Webhook, error) {
	out, err := scanWebhook(s.Pool.QueryRow(ctx, `
		INSERT INTO webhooks (org_id, name, url, secret, events, enabled) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+webhookCols, orgID, w.Name, w.URL, w.Secret, nonNil(w.Events), w.Enabled))
	return out, mapErr(err)
}

func (s *Store) UpdateWebhook(ctx context.Context, orgID string, w Webhook) (Webhook, error) {
	out, err := scanWebhook(s.Pool.QueryRow(ctx, `
		UPDATE webhooks SET name = $3, url = $4, secret = $5, events = $6, enabled = $7
		WHERE org_id = $1 AND id = $2 RETURNING `+webhookCols, orgID, w.ID, w.Name, w.URL, w.Secret, nonNil(w.Events), w.Enabled))
	return out, mapErr(err)
}

func (s *Store) DeleteWebhook(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `DELETE FROM webhooks WHERE org_id = $1 AND id = $2`, orgID, id))
}

func (s *Store) RecordWebhookDelivery(ctx context.Context, id string, status int) error {
	_, err := s.Pool.Exec(ctx, `UPDATE webhooks SET last_status = $2, last_delivery_at = now() WHERE id = $1`, id, status)
	return err
}

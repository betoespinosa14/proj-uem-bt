package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/uem/uem/internal/protocol"
)

const commandSelect = `
	SELECT c.id, c.org_id, c.device_id, d.name, c.type, c.payload, c.status, c.exit_code, c.stdout, c.stderr,
	       c.error, u.email, c.source, c.created_at, c.sent_at, c.completed_at
	FROM commands c JOIN devices d ON d.id = c.device_id LEFT JOIN users u ON u.id = c.issued_by`

func scanCommand(r pgx.Row) (Command, error) {
	var c Command
	err := r.Scan(&c.ID, &c.OrgID, &c.DeviceID, &c.DeviceName, &c.Type, &c.Payload, &c.Status, &c.ExitCode,
		&c.Stdout, &c.Stderr, &c.Error, &c.IssuedByEmail, &c.Source, &c.CreatedAt, &c.SentAt, &c.CompletedAt)
	return c, err
}

func (s *Store) listCommands(ctx context.Context, where string, limit int, args ...any) ([]Command, error) {
	q := fmt.Sprintf("%s WHERE %s ORDER BY c.created_at DESC LIMIT %d", commandSelect, where, limit)
	rows, err := s.Pool.Query(ctx, q, args...)
	return collect(rows, err, func(r pgx.Rows) (Command, error) { return scanCommand(r) })
}

func (s *Store) ListDeviceCommands(ctx context.Context, orgID, deviceID string) ([]Command, error) {
	return s.listCommands(ctx, `c.org_id = $1 AND c.device_id = $2`, 200, orgID, deviceID)
}

func (s *Store) GetCommand(ctx context.Context, orgID, id string) (Command, error) {
	c, err := scanCommand(s.Pool.QueryRow(ctx, commandSelect+` WHERE c.org_id = $1 AND c.id = $2`, orgID, id))
	return c, mapErr(err)
}

// CreateCommand queues a command. issuedBy is empty for system-originated commands.
func (s *Store) CreateCommand(ctx context.Context, orgID, deviceID, typ string, payload json.RawMessage, issuedBy, source string) (Command, error) {
	var by *string
	if issuedBy != "" {
		by = &issuedBy
	}
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO commands (org_id, device_id, type, payload, issued_by, source)
		SELECT $1, $2, $3, $4, $5, $6 WHERE EXISTS (
			SELECT 1 FROM devices WHERE org_id = $1 AND id = $2 AND retired_at IS NULL)
		RETURNING id`, orgID, deviceID, typ, payload, by, source).Scan(&id)
	if err != nil {
		return Command{}, mapErr(err)
	}
	return s.GetCommand(ctx, orgID, id)
}

// PendingCommands returns queued commands for a device, oldest first.
func (s *Store) PendingCommands(ctx context.Context, deviceID string) ([]Command, error) {
	rows, err := s.Pool.Query(ctx, commandSelect+` WHERE c.device_id = $1 AND c.status = 'queued' ORDER BY c.created_at`, deviceID)
	return collect(rows, err, func(r pgx.Rows) (Command, error) { return scanCommand(r) })
}

func (s *Store) MarkCommandSent(ctx context.Context, id string) (Command, error) {
	var orgID string
	err := s.Pool.QueryRow(ctx, `
		UPDATE commands SET status = 'sent', sent_at = now() WHERE id = $1 AND status = 'queued' RETURNING org_id`, id).Scan(&orgID)
	if err != nil {
		return Command{}, mapErr(err)
	}
	return s.GetCommand(ctx, orgID, id)
}

// ApplyCommandResult records a progress or terminal update from the device
// that owns the command. Updates to already-terminal commands are ignored.
func (s *Store) ApplyCommandResult(ctx context.Context, deviceID string, res protocol.CommandResult) (Command, error) {
	var completed *time.Time
	if protocol.IsTerminal(res.Status) {
		now := time.Now()
		completed = &now
	}
	var errMsg *string
	if res.Error != "" {
		errMsg = &res.Error
	}
	var orgID string
	err := s.Pool.QueryRow(ctx, `
		UPDATE commands SET status = $3, exit_code = $4, stdout = $5, stderr = $6, error = $7, completed_at = $8
		WHERE id = $1 AND device_id = $2 AND status IN ('queued','sent','running')
		RETURNING org_id`,
		res.ID, deviceID, res.Status, res.ExitCode, truncate(res.Stdout), truncate(res.Stderr), errMsg, completed).Scan(&orgID)
	if err != nil {
		return Command{}, mapErr(err)
	}
	return s.GetCommand(ctx, orgID, res.ID)
}

// RequeueSent resets commands that were sent but never acknowledged, e.g.
// when the agent disconnected mid-delivery. Running commands are left alone
// because re-running a script that may have partially executed is unsafe.
func (s *Store) RequeueSent(ctx context.Context, deviceID string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE commands SET status = 'queued', sent_at = NULL WHERE device_id = $1 AND status = 'sent'`, deviceID)
	return err
}

// ExpireStaleCommands times out commands stuck in running for longer than maxAge.
func (s *Store) ExpireStaleCommands(ctx context.Context, maxAge time.Duration) ([]Command, error) {
	rows, err := s.Pool.Query(ctx, `
		UPDATE commands SET status = 'timeout', error = 'no result received from agent', completed_at = now()
		WHERE status IN ('sent','running') AND sent_at < $1 RETURNING id, org_id`, time.Now().Add(-maxAge))
	refs, err := collect(rows, err, func(r pgx.Rows) (DeviceRef, error) {
		var d DeviceRef
		return d, r.Scan(&d.ID, &d.OrgID)
	})
	if err != nil {
		return nil, err
	}
	out := make([]Command, 0, len(refs))
	for _, ref := range refs {
		if c, err := s.GetCommand(ctx, ref.OrgID, ref.ID); err == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

func truncate(s string) string {
	if len(s) > protocol.MaxOutputBytes {
		return s[:protocol.MaxOutputBytes] + "\n[output truncated]"
	}
	return s
}

// ---- Scripts ----

const scriptCols = `id, name, description, shell, body, timeout_seconds, created_at, updated_at`

func scanScript(r pgx.Row) (Script, error) {
	var sc Script
	err := r.Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Shell, &sc.Body, &sc.TimeoutSeconds, &sc.CreatedAt, &sc.UpdatedAt)
	return sc, err
}

func (s *Store) ListScripts(ctx context.Context, orgID string) ([]Script, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+scriptCols+` FROM scripts WHERE org_id = $1 ORDER BY name`, orgID)
	return collect(rows, err, func(r pgx.Rows) (Script, error) { return scanScript(r) })
}

func (s *Store) GetScript(ctx context.Context, orgID, id string) (Script, error) {
	sc, err := scanScript(s.Pool.QueryRow(ctx, `SELECT `+scriptCols+` FROM scripts WHERE org_id = $1 AND id = $2`, orgID, id))
	return sc, mapErr(err)
}

func (s *Store) CreateScript(ctx context.Context, orgID string, sc Script) (Script, error) {
	out, err := scanScript(s.Pool.QueryRow(ctx, `
		INSERT INTO scripts (org_id, name, description, shell, body, timeout_seconds) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+scriptCols, orgID, sc.Name, sc.Description, sc.Shell, sc.Body, sc.TimeoutSeconds))
	return out, mapErr(err)
}

func (s *Store) UpdateScript(ctx context.Context, orgID string, sc Script) (Script, error) {
	out, err := scanScript(s.Pool.QueryRow(ctx, `
		UPDATE scripts SET name = $3, description = $4, shell = $5, body = $6, timeout_seconds = $7, updated_at = now()
		WHERE org_id = $1 AND id = $2 RETURNING `+scriptCols,
		orgID, sc.ID, sc.Name, sc.Description, sc.Shell, sc.Body, sc.TimeoutSeconds))
	return out, mapErr(err)
}

func (s *Store) DeleteScript(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `DELETE FROM scripts WHERE org_id = $1 AND id = $2`, orgID, id))
}

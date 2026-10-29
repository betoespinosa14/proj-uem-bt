package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

const orgCols = `id, name, server_url, usage_tracking, idle_threshold_sec, created_at`

const userCols = `id, org_id, email, name, role, password_hash, created_at, last_login_at`

func scanUser(r pgx.Row) (User, error) {
	var u User
	err := r.Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.CreatedAt, &u.LastLoginAt)
	return u, err
}

// CreateOrgWithOwner creates an organization and its first (owner) user.
func (s *Store) CreateOrgWithOwner(ctx context.Context, orgName, name, email, passwordHash string) (Org, User, error) {
	var org Org
	var user User
	err := s.InTx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO organizations (name) VALUES ($1) RETURNING `+orgCols, orgName,
		).Scan(&org.ID, &org.Name, &org.ServerURL, &org.UsageTracking, &org.IdleThresholdSec, &org.CreatedAt)
		if err != nil {
			return err
		}
		user, err = scanUser(tx.QueryRow(ctx,
			`INSERT INTO users (org_id, email, name, role, password_hash) VALUES ($1, $2, $3, 'owner', $4)
			 RETURNING `+userCols, org.ID, email, name, passwordHash))
		return err
	})
	return org, user, mapErr(err)
}

func (s *Store) GetOrg(ctx context.Context, orgID string) (Org, error) {
	var o Org
	err := s.Pool.QueryRow(ctx, `SELECT `+orgCols+` FROM organizations WHERE id = $1`, orgID).
		Scan(&o.ID, &o.Name, &o.ServerURL, &o.UsageTracking, &o.IdleThresholdSec, &o.CreatedAt)
	return o, mapErr(err)
}

// OrgPatch holds the organization settings to change; nil fields are kept.
type OrgPatch struct {
	Name             *string
	ServerURL        *string
	UsageTracking    *bool
	IdleThresholdSec *int
}

func (s *Store) UpdateOrg(ctx context.Context, orgID string, p OrgPatch) (Org, error) {
	var o Org
	err := s.Pool.QueryRow(ctx, `
		UPDATE organizations SET name = COALESCE($2, name), server_url = COALESCE($3, server_url),
			usage_tracking = COALESCE($4, usage_tracking), idle_threshold_sec = COALESCE($5, idle_threshold_sec)
		WHERE id = $1 RETURNING `+orgCols, orgID, p.Name, p.ServerURL, p.UsageTracking, p.IdleThresholdSec).
		Scan(&o.ID, &o.Name, &o.ServerURL, &o.UsageTracking, &o.IdleThresholdSec, &o.CreatedAt)
	return o, mapErr(err)
}

// ListOrgIDs returns every organization; used by background jobs.
func (s *Store) ListOrgIDs(ctx context.Context) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id FROM organizations`)
	return collect(rows, err, func(r pgx.Rows) (string, error) {
		var id string
		return id, r.Scan(&id)
	})
}

// GetUserByEmail is the only unscoped user lookup; it backs login.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	u, err := scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE lower(email) = lower($1)`, email))
	return u, mapErr(err)
}

func (s *Store) GetUser(ctx context.Context, orgID, id string) (User, error) {
	u, err := scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE org_id = $1 AND id = $2`, orgID, id))
	return u, mapErr(err)
}

func (s *Store) ListUsers(ctx context.Context, orgID string) ([]User, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+userCols+` FROM users WHERE org_id = $1 ORDER BY created_at`, orgID)
	return collect(rows, err, func(r pgx.Rows) (User, error) { return scanUser(r) })
}

func (s *Store) CreateUser(ctx context.Context, orgID, email, name, role, passwordHash string) (User, error) {
	u, err := scanUser(s.Pool.QueryRow(ctx, `
		INSERT INTO users (org_id, email, name, role, password_hash) VALUES ($1, $2, $3, $4, $5)
		RETURNING `+userCols, orgID, email, name, role, passwordHash))
	return u, mapErr(err)
}

func (s *Store) UpdateUser(ctx context.Context, orgID, id string, name, role, passwordHash *string) (User, error) {
	u, err := scanUser(s.Pool.QueryRow(ctx, `
		UPDATE users SET name = COALESCE($3, name), role = COALESCE($4, role), password_hash = COALESCE($5, password_hash)
		WHERE org_id = $1 AND id = $2 RETURNING `+userCols, orgID, id, name, role, passwordHash))
	return u, mapErr(err)
}

func (s *Store) DeleteUser(ctx context.Context, orgID, id string) error {
	return expectOne(s.Pool.Exec(ctx, `DELETE FROM users WHERE org_id = $1 AND id = $2`, orgID, id))
}

// CountOwners lets callers refuse to demote or delete the last owner.
func (s *Store) CountOwners(ctx context.Context, orgID string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE org_id = $1 AND role = 'owner'`, orgID).Scan(&n)
	return n, err
}

func (s *Store) TouchLogin(ctx context.Context, userID string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, userID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, sess Session, ip, userAgent string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, org_id, csrf_token, expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sess.ID, sess.UserID, sess.OrgID, sess.CSRFToken, sess.ExpiresAt, ip, userAgent)
	return err
}

// GetSession returns a live session and its user.
func (s *Store) GetSession(ctx context.Context, id string) (Session, User, error) {
	var sess Session
	var u User
	err := s.Pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.org_id, s.csrf_token, s.expires_at,
		       u.id, u.org_id, u.email, u.name, u.role, u.password_hash, u.created_at, u.last_login_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = $1 AND s.expires_at > now()`, id).
		Scan(&sess.ID, &sess.UserID, &sess.OrgID, &sess.CSRFToken, &sess.ExpiresAt,
			&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.CreatedAt, &u.LastLoginAt)
	return sess, u, mapErr(err)
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	return err
}

// DeleteUserSessions revokes all sessions, e.g. after a role or password change.
func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID)
	return err
}

func (s *Store) PurgeExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < $1`, now)
	return err
}

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// MFAState is a user's two-step sign-in data. Secrets never leave the server.
type MFAState struct {
	Secret        string // empty while two-step sign-in is off
	PendingSecret string // set during setup
	LastStep      int64
	EnabledAt     *time.Time
}

func (s *Store) GetMFA(ctx context.Context, userID string) (MFAState, error) {
	var m MFAState
	var secret, pending *string
	err := s.Pool.QueryRow(ctx, `
		SELECT totp_secret, totp_pending_secret, totp_last_step, mfa_enabled_at FROM users WHERE id = $1`, userID).
		Scan(&secret, &pending, &m.LastStep, &m.EnabledAt)
	if secret != nil {
		m.Secret = *secret
	}
	if pending != nil {
		m.PendingSecret = *pending
	}
	return m, mapErr(err)
}

// SetPendingTOTP stores a new secret during setup; it takes effect only once
// EnableMFA confirms it.
func (s *Store) SetPendingTOTP(ctx context.Context, userID, secret string) error {
	return expectOne(s.Pool.Exec(ctx, `UPDATE users SET totp_pending_secret = $2 WHERE id = $1`, userID, secret))
}

// EnableMFA promotes the pending secret and replaces the recovery codes.
// It fails with ErrConflict if the pending secret changed meanwhile.
func (s *Store) EnableMFA(ctx context.Context, userID, secret string, step int64, codeHashes []string) error {
	return s.InTx(ctx, func(tx pgx.Tx) error {
		if err := expectOne(tx.Exec(ctx, `
			UPDATE users SET totp_secret = totp_pending_secret, totp_pending_secret = NULL,
				totp_last_step = $3, mfa_enabled_at = now()
			WHERE id = $1 AND totp_pending_secret = $2`, userID, secret, step)); err != nil {
			if err == ErrNotFound {
				return ErrConflict
			}
			return err
		}
		return replaceRecoveryCodes(ctx, tx, userID, codeHashes)
	})
}

// DisableMFA turns two-step sign-in off and removes its codes and remembered
// browsers.
func (s *Store) DisableMFA(ctx context.Context, userID string) error {
	return s.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE users SET totp_secret = NULL, totp_pending_secret = NULL, totp_last_step = 0, mfa_enabled_at = NULL
			WHERE id = $1`, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM mfa_trusted_browsers WHERE user_id = $1`, userID)
		return err
	})
}

// UseTOTPStep records an accepted code. It succeeds only if the step is newer
// than the last one used, so two requests can't both use the same code.
func (s *Store) UseTOTPStep(ctx context.Context, userID string, step int64) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE users SET totp_last_step = $2 WHERE id = $1 AND totp_last_step < $2 AND mfa_enabled_at IS NOT NULL`,
		userID, step)
	return err == nil && tag.RowsAffected() == 1, err
}

func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID string, codeHashes []string) error {
	return s.InTx(ctx, func(tx pgx.Tx) error { return replaceRecoveryCodes(ctx, tx, userID, codeHashes) })
}

func replaceRecoveryCodes(ctx context.Context, tx pgx.Tx, userID string, codeHashes []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, h := range codeHashes {
		if _, err := tx.Exec(ctx, `INSERT INTO mfa_recovery_codes (user_id, code_hash) VALUES ($1, $2)`, userID, h); err != nil {
			return err
		}
	}
	return nil
}

// UseRecoveryCode marks an unused code as used; false if there's no such code.
func (s *Store) UseRecoveryCode(ctx context.Context, userID, codeHash string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE mfa_recovery_codes SET used_at = now() WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`,
		userID, codeHash)
	return err == nil && tag.RowsAffected() == 1, err
}

// MFASummary is what the Account security page shows.
type MFASummary struct {
	Enabled           bool       `json:"enabled"`
	EnabledAt         *time.Time `json:"enabled_at"`
	RecoveryCodesLeft int        `json:"recovery_codes_left"`
	TrustedBrowsers   int        `json:"trusted_browsers"`
}

func (s *Store) MFASummary(ctx context.Context, userID string) (MFASummary, error) {
	var m MFASummary
	err := s.Pool.QueryRow(ctx, `
		SELECT mfa_enabled_at IS NOT NULL, mfa_enabled_at,
			(SELECT count(*) FROM mfa_recovery_codes WHERE user_id = u.id AND used_at IS NULL),
			(SELECT count(*) FROM mfa_trusted_browsers WHERE user_id = u.id AND expires_at > now())
		FROM users u WHERE id = $1`, userID).Scan(&m.Enabled, &m.EnabledAt, &m.RecoveryCodesLeft, &m.TrustedBrowsers)
	return m, mapErr(err)
}

func (s *Store) AddTrustedBrowser(ctx context.Context, id, userID string, expires time.Time, userAgent string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO mfa_trusted_browsers (id, user_id, expires_at, user_agent) VALUES ($1, $2, $3, $4)`,
		id, userID, expires, userAgent)
	return err
}

// IsTrustedBrowser reports whether a remembered-browser token is valid for the user.
func (s *Store) IsTrustedBrowser(ctx context.Context, id, userID string) bool {
	var one int
	err := s.Pool.QueryRow(ctx, `
		SELECT 1 FROM mfa_trusted_browsers WHERE id = $1 AND user_id = $2 AND expires_at > now()`, id, userID).Scan(&one)
	return err == nil
}

// ForgetTrustedBrowsers signs every remembered browser out of skipping the code.
func (s *Store) ForgetTrustedBrowsers(ctx context.Context, userID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM mfa_trusted_browsers WHERE user_id = $1`, userID)
	return err
}

func (s *Store) PurgeExpiredTrustedBrowsers(ctx context.Context, now time.Time) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM mfa_trusted_browsers WHERE expires_at < $1`, now)
	return err
}

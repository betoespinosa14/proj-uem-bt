package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// AppendAudit inserts an entry into the org's hash chain. The per-org
// advisory lock serializes writers so each entry links to its true predecessor;
// hash computes the entry hash from the predecessor's hash.
func (s *Store) AppendAudit(ctx context.Context, e AuditEntry, hash func(prev string) string) (AuditEntry, error) {
	err := s.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('audit:' || $1))`, e.OrgID); err != nil {
			return err
		}
		prev := ""
		err := tx.QueryRow(ctx, `SELECT hash FROM audit_log WHERE org_id = $1 ORDER BY id DESC LIMIT 1`, e.OrgID).Scan(&prev)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		e.PrevHash = prev
		e.Hash = hash(prev)
		return tx.QueryRow(ctx, `
			INSERT INTO audit_log (org_id, ts, actor_type, actor_id, actor_email, action, target_type, target_id, details, ip, prev_hash, hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
			e.OrgID, e.TS, e.ActorType, e.ActorID, e.ActorEmail, e.Action, e.TargetType, e.TargetID, e.Details, e.IP, e.PrevHash, e.Hash,
		).Scan(&e.ID)
	})
	return e, err
}

const auditCols = `id, org_id, ts, actor_type, actor_id, actor_email, action, target_type, target_id, details, ip, prev_hash, hash`

func scanAudit(r pgx.Row) (AuditEntry, error) {
	var e AuditEntry
	err := r.Scan(&e.ID, &e.OrgID, &e.TS, &e.ActorType, &e.ActorID, &e.ActorEmail, &e.Action, &e.TargetType, &e.TargetID,
		&e.Details, &e.IP, &e.PrevHash, &e.Hash)
	return e, err
}

type AuditFilter struct {
	Action   string // prefix match, e.g. "device." or "device.command"
	Actor    string // email substring
	TargetID string
	BeforeID int64
	Limit    int
}

func (s *Store) ListAudit(ctx context.Context, orgID string, f AuditFilter) ([]AuditEntry, error) {
	where := []string{"org_id = $1"}
	args := []any{orgID}
	add := func(clause string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Action != "" {
		add("action LIKE $%d", escapeLike(f.Action)+"%")
	}
	if f.Actor != "" {
		add("actor_email ILIKE $%d", "%"+escapeLike(f.Actor)+"%")
	}
	if f.TargetID != "" {
		add("target_id = $%d", f.TargetID)
	}
	if f.BeforeID > 0 {
		add("id < $%d", f.BeforeID)
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q := fmt.Sprintf("SELECT %s FROM audit_log WHERE %s ORDER BY id DESC LIMIT %d", auditCols, strings.Join(where, " AND "), limit)
	rows, err := s.Pool.Query(ctx, q, args...)
	return collect(rows, err, func(r pgx.Rows) (AuditEntry, error) { return scanAudit(r) })
}

// EachAudit streams an org's whole audit log in chain order.
func (s *Store) EachAudit(ctx context.Context, orgID string, fn func(AuditEntry) error) error {
	rows, err := s.Pool.Query(ctx, `SELECT `+auditCols+` FROM audit_log WHERE org_id = $1 ORDER BY id`, orgID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			return err
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

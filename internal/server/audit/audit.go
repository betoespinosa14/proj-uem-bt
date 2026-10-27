// Package audit writes the tamper-evident, hash-chained audit log. Each
// entry's hash covers its content and the previous entry's hash, so editing or
// removing any row breaks verification of every later row.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/uem/uem/internal/server/store"
)

// Actor identifies who performed an action.
type Actor struct {
	Type  string // user | system | device
	ID    string
	Email string
	IP    string
}

func System() Actor { return Actor{Type: "system"} }

// Event is one auditable action.
type Event struct {
	OrgID      string
	Actor      Actor
	Action     string // dotted verb, e.g. "device.command.create"
	TargetType string
	TargetID   string
	Details    any
}

// Sink receives every successfully recorded entry (used to fan out to webhooks).
type Sink func(ctx context.Context, e store.AuditEntry)

type Log struct {
	store *store.Store
	sink  Sink
}

func New(s *store.Store) *Log { return &Log{store: s} }

func (l *Log) SetSink(s Sink) { l.sink = s }

// Record appends an entry. Audit failures are logged rather than returned:
// the action already happened, so the caller can't meaningfully roll it back.
func (l *Log) Record(ctx context.Context, ev Event) {
	details, err := canonicalJSON(ev.Details)
	if err != nil {
		details = []byte(`{}`)
	}
	e := store.AuditEntry{
		OrgID:     ev.OrgID,
		TS:        time.Now().UTC().Truncate(time.Microsecond), // Postgres precision, so the hash round-trips
		ActorType: ev.Actor.Type,
		ActorID:   nilIfEmpty(ev.Actor.ID),
		ActorEmail: nilIfEmpty(ev.Actor.Email),
		Action:    ev.Action,
		TargetType: nilIfEmpty(ev.TargetType),
		TargetID:  nilIfEmpty(ev.TargetID),
		Details:   details,
		IP:        nilIfEmpty(ev.Actor.IP),
	}
	saved, err := l.store.AppendAudit(ctx, e, func(prev string) string { return Hash(prev, e) })
	if err != nil {
		slog.Error("audit write failed", "action", ev.Action, "org", ev.OrgID, "err", err)
		return
	}
	if l.sink != nil {
		l.sink(ctx, saved)
	}
}

// Hash computes the chain hash for an entry given its predecessor's hash.
func Hash(prev string, e store.AuditEntry) string {
	h := sha256.New()
	field := func(s string) {
		h.Write([]byte(s))
		h.Write([]byte{0x1f}) // unit separator keeps field boundaries unambiguous
	}
	field(prev)
	field(e.OrgID)
	field(e.TS.UTC().Format(time.RFC3339Nano))
	field(e.ActorType)
	field(deref(e.ActorID))
	field(deref(e.ActorEmail))
	field(e.Action)
	field(deref(e.TargetType))
	field(deref(e.TargetID))
	details, err := canonicalJSON(e.Details)
	if err != nil {
		details = e.Details
	}
	field(string(details))
	field(deref(e.IP))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyResult reports whether an org's chain is intact.
type VerifyResult struct {
	Valid      bool   `json:"valid"`
	Checked    int    `json:"checked"`
	BrokenAtID *int64 `json:"broken_at_id"`
}

// Verify recomputes every hash in the org's chain.
func (l *Log) Verify(ctx context.Context, orgID string) (VerifyResult, error) {
	res := VerifyResult{Valid: true}
	prev := ""
	err := l.store.EachAudit(ctx, orgID, func(e store.AuditEntry) error {
		if !res.Valid {
			return nil
		}
		res.Checked++
		if e.PrevHash != prev || Hash(prev, e) != e.Hash {
			res.Valid = false
			id := e.ID
			res.BrokenAtID = &id
		}
		prev = e.Hash
		return nil
	})
	return res, err
}

// canonicalJSON produces a stable encoding (sorted keys, normalized numbers)
// so details hash identically before storage and after a jsonb round-trip.
func canonicalJSON(v any) ([]byte, error) {
	var raw []byte
	switch x := v.(type) {
	case nil:
		return []byte(`{}`), nil
	case json.RawMessage:
		raw = x
	case []byte:
		raw = x
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return json.Marshal(generic)
}

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

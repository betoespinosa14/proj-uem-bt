package audit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/uem/uem/internal/server/store"
)

func entry() store.AuditEntry {
	email := "ada@example.com"
	return store.AuditEntry{
		OrgID: "org", TS: time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC), ActorType: "user",
		ActorEmail: &email, Action: "device.command.create", Details: json.RawMessage(`{"b":2,"a":"<x>"}`),
	}
}

func TestHashIsStableAcrossJSONReencoding(t *testing.T) {
	e := entry()
	h1 := Hash("prev", e)
	// jsonb reorders keys and changes whitespace; the hash must not change.
	e.Details = json.RawMessage(`{"a": "<x>", "b": 2}`)
	if h2 := Hash("prev", e); h1 != h2 {
		t.Fatal("hash changed after semantically identical re-encoding")
	}
}

func TestHashDetectsChanges(t *testing.T) {
	base := Hash("prev", entry())
	mutations := map[string]func(*store.AuditEntry){
		"action":  func(e *store.AuditEntry) { e.Action = "device.retire" },
		"details": func(e *store.AuditEntry) { e.Details = json.RawMessage(`{"a":"<x>","b":3}`) },
		"time":    func(e *store.AuditEntry) { e.TS = e.TS.Add(time.Microsecond) },
		"actor":   func(e *store.AuditEntry) { s := "eve@example.com"; e.ActorEmail = &s },
	}
	for name, mutate := range mutations {
		e := entry()
		mutate(&e)
		if Hash("prev", e) == base {
			t.Errorf("changing %s did not change the hash", name)
		}
	}
	if Hash("other", entry()) == base {
		t.Error("changing prev hash did not change the hash")
	}
}

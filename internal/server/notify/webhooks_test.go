package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uem/uem/internal/server/store"
)

func TestMatches(t *testing.T) {
	if !Matches([]string{"audit.*"}, "audit.device.retire") {
		t.Error("wildcard should match")
	}
	if Matches([]string{"audit.*"}, "alert.created") {
		t.Error("wildcard should not match other prefixes")
	}
	if !Matches([]string{"alert.created"}, "alert.created") || Matches([]string{"alert.created"}, "alert") {
		t.Error("exact match failed")
	}
}

func TestSignIsHMACSHA256(t *testing.T) {
	// Known vector: HMAC-SHA256(key="key", "The quick brown fox jumps over the lazy dog")
	got := Sign("key", []byte("The quick brown fox jumps over the lazy dog"))
	if got != "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8" {
		t.Fatalf("unexpected signature %s", got)
	}
}

func TestPrivateDestinationsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	w := New(nil, false)
	_, err := w.post(context.Background(), store.Webhook{URL: srv.URL, Secret: "s"}, "test", []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "not a public address") {
		t.Fatalf("expected loopback delivery to be refused, got %v", err)
	}
	allowed := New(nil, true)
	status, err := allowed.post(context.Background(), store.Webhook{URL: srv.URL, Secret: "s"}, "test", []byte(`{}`))
	if err != nil || status != 200 {
		t.Fatalf("allowPrivate delivery failed: %d %v", status, err)
	}
}

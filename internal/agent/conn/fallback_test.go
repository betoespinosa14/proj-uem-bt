package conn

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/uem/uem/internal/agent/config"
	"github.com/uem/uem/internal/protocol"
)

func TestFallbackCandidates(t *testing.T) {
	got := fallbackCandidates("https://beacon.example.com/", []string{
		" https://203.0.113.5/ ",
		"https://BEACON.example.com", // the main address itself
		"https://203.0.113.5",        // duplicate
		"ftp://x", "not a url", "",
		"http://192.168.1.10:8080",
	})
	want := []string{"https://203.0.113.5", "http://192.168.1.10:8080"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := fallbackCandidates("https://a", nil); len(got) != 0 {
		t.Fatalf("empty list: got %q", got)
	}
}

func TestPickFallbackNeedsRepeatedFailures(t *testing.T) {
	a, _ := newIdleAgent("https://main.invalid", []string{"https://down.invalid", "https://up.invalid"})
	a.health = func(_ context.Context, s string) error {
		if s == "https://up.invalid" {
			return nil
		}
		return context.DeadlineExceeded
	}
	for i := 0; i < fallbackAfter-1; i++ {
		a.primaryFailed()
		if a.pickFallback(context.Background()) {
			t.Fatalf("switched after %d failures", i+1)
		}
	}
	a.primaryFailed()
	if !a.pickFallback(context.Background()) {
		t.Fatal("didn't switch after repeated failures")
	}
	if s, trial := a.serverTarget(); s != "https://up.invalid" || trial {
		t.Fatalf("serverTarget = %q, %v; want the healthy fallback", s, trial)
	}
	a.fallbackFailed()
	if s, _ := a.serverTarget(); s != "https://main.invalid" {
		t.Fatalf("after the fallback failed, serverTarget = %q; want the main address", s)
	}
}

func TestPickFallbackWithoutList(t *testing.T) {
	a, _ := newIdleAgent("https://main.invalid", nil)
	a.health = func(context.Context, string) error { return nil }
	for i := 0; i < 5; i++ {
		a.primaryFailed()
	}
	if a.pickFallback(context.Background()) {
		t.Fatal("switched with no fallbacks known")
	}
}

func TestSetFallbacksSavesChanges(t *testing.T) {
	a, saved := newIdleAgent("https://main.example", nil)
	a.setFallbacks([]string{"https://203.0.113.5/", "https://main.example"})
	if got := <-saved; !reflect.DeepEqual(got, []string{"https://203.0.113.5"}) {
		t.Fatalf("saved %q", got)
	}
	a.setFallbacks([]string{"https://203.0.113.5"}) // unchanged: no save
	select {
	case got := <-saved:
		t.Fatalf("saved again: %q", got)
	default:
	}
	a.setFallbacks(nil) // removed on the server
	if got := <-saved; len(got) != 0 {
		t.Fatalf("saved %q, want empty", got)
	}
}

// The main address is unreachable: the agent connects through the fallback,
// then returns once the main address answers again.
func TestAgentUsesFallbackAndReturns(t *testing.T) {
	fb := newFakeServer(t, true)
	down := newFakeServer(t, true)
	mainURL := down.URL
	down.Close() // the main address refuses connections

	fb.policy = &protocol.Policy{ServerURL: mainURL, FallbackURLs: []string{fb.URL}}
	a, _ := runAgent(t, mainURL, func(a *Agent) {
		a.cfg.FallbackURLs = []string{fb.URL}
		a.primaryEvery = 100 * time.Millisecond
	})
	waitFor(t, 30*time.Second, "a connection through the fallback", func() bool { return fb.connections() > 0 })
	if s, _ := a.serverTarget(); s != fb.URL {
		t.Fatalf("serverTarget = %q, want the fallback", s)
	}

	// The main address answers again (simulated by pointing it at a live server).
	back := newFakeServer(t, true)
	a.mu.Lock()
	a.cfg.ServerURL = back.URL
	a.mu.Unlock()
	waitFor(t, 10*time.Second, "the return to the main address", func() bool { return back.connections() > 0 })
	if s, _ := a.serverTarget(); s != back.URL {
		t.Fatalf("serverTarget = %q, want the main address", s)
	}
}

// newIdleAgent returns an agent that isn't running, with saved fallback lists
// delivered on the channel.
func newIdleAgent(server string, fallbacks []string) (*Agent, chan []string) {
	a := New(config.Config{ServerURL: server, DeviceID: "dev1", DeviceSecret: "secret1", FallbackURLs: fallbacks}, "test",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	saved := make(chan []string, 4)
	a.SaveFallbacks = func(u []string) error { saved <- u; return nil }
	return a, saved
}

func waitFor(t *testing.T, limit time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

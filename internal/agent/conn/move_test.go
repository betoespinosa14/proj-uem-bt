package conn

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/uem/uem/internal/agent/config"
	"github.com/uem/uem/internal/protocol"
)

func TestMoveTarget(t *testing.T) {
	now := time.Now()
	failed := map[string]time.Time{
		"https://recent.example":  now.Add(-time.Minute),
		"https://old-try.example": now.Add(-time.Hour),
	}
	cases := []struct {
		current, announced, want string
		ok                       bool
	}{
		{"http://192.168.1.7:8080", "", "", false},
		{"http://192.168.1.7:8080", "http://192.168.1.7:8080/", "", false}, // same address
		{"http://192.168.1.7:8080", "HTTP://192.168.1.7:8080", "", false},  // same, other case
		{"http://192.168.1.7:8080", "https://uem.example.com/", "https://uem.example.com", true},
		{"http://192.168.1.7:8080", " https://uem.example.com ", "https://uem.example.com", true},
		{"http://192.168.1.7:8080", "ftp://uem.example.com", "", false},
		{"http://192.168.1.7:8080", "uem.example.com", "", false}, // no scheme
		{"http://192.168.1.7:8080", "https://recent.example", "", false},
		{"http://192.168.1.7:8080", "https://old-try.example", "https://old-try.example", true},
	}
	for _, c := range cases {
		got, ok := moveTarget(c.current, c.announced, failed, now)
		if got != c.want || ok != c.ok {
			t.Errorf("moveTarget(%q, %q) = %q, %v; want %q, %v", c.current, c.announced, got, ok, c.want, c.ok)
		}
	}
}

// fakeServer accepts agent connections, optionally sends a policy, and
// records how many agents connected.
type fakeServer struct {
	*httptest.Server
	mu        sync.Mutex
	connected int
	accept    bool
	policy    *protocol.Policy
}

func newFakeServer(t *testing.T, accept bool) *fakeServer {
	f := &fakeServer{accept: accept}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/agent/ws", func(w http.ResponseWriter, r *http.Request) {
		if !f.accept || r.Header.Get("Authorization") != "Device dev1:secret1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		f.mu.Lock()
		f.connected++
		p := f.policy
		f.mu.Unlock()
		if p != nil {
			frame, _ := protocol.Encode(protocol.TypePolicy, p)
			_ = c.Write(r.Context(), websocket.MessageText, frame)
		}
		for {
			if _, _, err := c.Read(r.Context()); err != nil {
				return
			}
		}
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) connections() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected
}

func runAgent(t *testing.T, server string, opts ...func(*Agent)) (*Agent, chan string) {
	a := New(config.Config{ServerURL: server, DeviceID: "dev1", DeviceSecret: "secret1"}, "test",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, o := range opts {
		o(a)
	}
	saved := make(chan string, 1)
	a.SaveServerURL = func(u string) error { saved <- u; return nil }
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)
	return a, saved
}

func TestAgentMovesToAnnouncedServer(t *testing.T) {
	newSrv := newFakeServer(t, true)
	oldSrv := newFakeServer(t, true)
	oldSrv.policy = &protocol.Policy{ServerURL: newSrv.URL + "/"}

	a, saved := runAgent(t, oldSrv.URL)
	select {
	case got := <-saved:
		if got != newSrv.URL {
			t.Fatalf("saved %q, want %q", got, newSrv.URL)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("agent never switched to the announced server")
	}
	if newSrv.connections() == 0 {
		t.Fatal("new server saw no connection")
	}
	if cur, trial := a.serverTarget(); cur != newSrv.URL || trial {
		t.Fatalf("serverTarget = %q, %v", cur, trial)
	}
}

func TestAgentKeepsServerWhenNewOneRejects(t *testing.T) {
	newSrv := newFakeServer(t, false) // healthy, but rejects this device
	oldSrv := newFakeServer(t, true)
	oldSrv.policy = &protocol.Policy{ServerURL: newSrv.URL}

	a, saved := runAgent(t, oldSrv.URL)
	deadline := time.Now().Add(10 * time.Second)
	for oldSrv.connections() < 2 && time.Now().Before(deadline) { // back on the old server
		time.Sleep(20 * time.Millisecond)
	}
	if oldSrv.connections() < 2 {
		t.Fatalf("agent didn't return to the old server (connections=%d)", oldSrv.connections())
	}
	select {
	case got := <-saved:
		t.Fatalf("saved %q although the new server rejected the device", got)
	default:
	}
	if cur, trial := a.serverTarget(); cur != oldSrv.URL || trial {
		t.Fatalf("serverTarget = %q, %v", cur, trial)
	}
	a.mu.Lock()
	_, remembered := a.failedMoves[newSrv.URL]
	a.mu.Unlock()
	if !remembered {
		t.Fatal("failed address not remembered; the agent would retry it on every policy")
	}
}

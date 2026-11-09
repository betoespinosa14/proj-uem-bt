package conn

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// A server that stops answering (here: never reads, so never sends pongs)
// looks like a connection that died when the device changed networks. The
// agent must drop it and reconnect instead of waiting for the OS to notice.
func TestAgentReconnectsWhenServerStopsAnswering(t *testing.T) {
	var mu sync.Mutex
	connects := 0
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agent/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		mu.Lock()
		connects++
		mu.Unlock()
		select { // hold the connection open without reading
		case <-release:
		case <-r.Context().Done():
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	runAgent(t, srv.URL, fastPings(100*time.Millisecond))
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := connects
		mu.Unlock()
		if n >= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent kept the unresponsive connection instead of reconnecting")
}

// A healthy server answers pings, so the agent must not drop the connection.
func TestAgentKeepsHealthyConnection(t *testing.T) {
	srv := newFakeServer(t, true)
	runAgent(t, srv.URL, fastPings(500*time.Millisecond))
	time.Sleep(time.Second) // about 20 pings
	if n := srv.connections(); n != 1 {
		t.Fatalf("agent connected %d times, want 1", n)
	}
}

func fastPings(timeout time.Duration) func(*Agent) {
	return func(a *Agent) { a.pingEvery, a.pingWait = 50*time.Millisecond, timeout }
}

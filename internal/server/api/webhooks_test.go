package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uem/uem/internal/server/notify"
)

func TestWebhookFormats(t *testing.T) {
	e := setup(t)
	a := e.signup(t, "Hooks")

	got := make(chan [2]string, 4) // {receiver, body}
	receiver := func(name string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			got <- [2]string{name, string(b)}
			w.WriteHeader(http.StatusAccepted) // what a Teams workflow answers
		}))
		t.Cleanup(srv.Close)
		return srv
	}
	siem, teams := receiver("siem"), receiver("teams")

	type hook struct {
		ID     string
		Format string
	}
	var g, tm hook
	a.do("POST", "/api/webhooks", map[string]any{"name": "SIEM", "url": siem.URL, "events": []string{"device.enrolled"}}, 201, &g)
	a.do("POST", "/api/webhooks", map[string]any{"name": "Teams", "url": teams.URL, "format": "teams", "events": []string{"device.enrolled"}}, 201, &tm)
	if g.Format != "generic" || tm.Format != "teams" {
		t.Fatalf("formats = %q, %q; want generic, teams", g.Format, tm.Format)
	}
	a.do("POST", "/api/webhooks", map[string]any{"name": "Bad", "url": siem.URL, "format": "slack", "events": []string{"device.enrolled"}}, 400, nil)
	a.do("PATCH", "/api/webhooks/"+g.ID, map[string]any{"format": "teams"}, 200, &g)
	if g.Format != "teams" {
		t.Fatalf("format after edit = %q", g.Format)
	}
	a.do("PATCH", "/api/webhooks/"+g.ID, map[string]any{"format": "generic"}, 200, &g)

	// Deliver one event through a dispatcher that may reach the local receivers.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hooks := notify.New(e.store, true)
	hooks.ConsoleURL = "https://beacon.example.com"
	hooks.Run(ctx, 2)
	hooks.Emit(ctx, a.orgID, "device.enrolled", map[string]any{"id": "d1", "name": "FIN-PC-02"})

	bodies := map[string]string{}
	for len(bodies) < 2 {
		select {
		case b := <-got:
			bodies[b[0]] = b[1]
		case <-time.After(5 * time.Second):
			t.Fatalf("deliveries received: %v", bodies)
		}
	}
	var generic struct{ Event string }
	if json.Unmarshal([]byte(bodies["siem"]), &generic) != nil || generic.Event != "device.enrolled" {
		t.Errorf("generic body = %s", bodies["siem"])
	}
	var card struct {
		Type        string
		Attachments []struct{ ContentType string }
	}
	if json.Unmarshal([]byte(bodies["teams"]), &card) != nil || card.Type != "message" || len(card.Attachments) != 1 ||
		card.Attachments[0].ContentType != "application/vnd.microsoft.card.adaptive" {
		t.Errorf("teams body = %s", bodies["teams"])
	}
}

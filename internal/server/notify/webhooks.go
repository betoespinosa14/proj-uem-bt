// Package notify delivers signed outbound webhooks, the generic integration
// point for SIEM (Splunk, Sentinel, Datadog) and ITSM (Jira, Freshservice) tools.
package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"

	"github.com/uem/uem/internal/server/store"
)

// Events a webhook can subscribe to. "audit.*" streams every audit entry.
var Events = []string{"alert.created", "compliance.violation", "device.enrolled", "device.offline", "command.completed", "audit.*"}

type Webhooks struct {
	store  *store.Store
	client *http.Client
	queue  chan delivery
}

type delivery struct {
	hook    store.Webhook
	event   string
	payload []byte
}

// New creates the dispatcher. Unless allowPrivate is set, deliveries to
// loopback, private, and link-local addresses (e.g. cloud metadata at
// 169.254.169.254) are refused: webhook URLs are tenant-supplied, so they
// must not become a way to probe the server's internal network.
func New(s *store.Store, allowPrivate bool) *Webhooks {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !allowPrivate {
		dialer.Control = func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return err
			}
			if !isPublic(ip) {
				return fmt.Errorf("webhook destination %s is not a public address", ip)
			}
			return nil
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.Proxy = nil // a proxy would bypass the address check
	return &Webhooks{
		store: s,
		client: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		queue: make(chan delivery, 1024),
	}
}

func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() &&
		!netip.MustParsePrefix("100.64.0.0/10").Contains(ip) // carrier-grade NAT
}

// Run delivers queued webhooks until ctx is cancelled.
func (w *Webhooks) Run(ctx context.Context, workers int) {
	for range workers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case d := <-w.queue:
					w.deliver(ctx, d)
				}
			}
		}()
	}
}

// Matches reports whether a hook subscribes to an event.
func Matches(subscribed []string, event string) bool {
	for _, s := range subscribed {
		if s == event || (strings.HasSuffix(s, ".*") && strings.HasPrefix(event, strings.TrimSuffix(s, "*"))) {
			return true
		}
	}
	return false
}

// Emit enqueues an event for every enabled hook in the org subscribed to it.
func (w *Webhooks) Emit(ctx context.Context, orgID, event string, data any) {
	hooks, err := w.store.ListWebhooks(ctx, orgID)
	if err != nil {
		slog.Error("webhooks: list", "err", err)
		return
	}
	var payload []byte
	for _, h := range hooks {
		if !h.Enabled || !Matches(h.Events, event) {
			continue
		}
		if payload == nil {
			payload, err = json.Marshal(map[string]any{
				"event": event, "org_id": orgID, "ts": time.Now().UTC(), "data": data,
			})
			if err != nil {
				return
			}
		}
		select {
		case w.queue <- delivery{h, event, payload}:
		default:
			slog.Warn("webhooks: queue full, dropping", "hook", h.ID, "event", event)
		}
	}
}

// SendTest delivers a test event synchronously and returns the HTTP status.
func (w *Webhooks) SendTest(ctx context.Context, h store.Webhook) (int, error) {
	payload, _ := json.Marshal(map[string]any{
		"event": "webhook.test", "org_id": h.OrgID, "ts": time.Now().UTC(),
		"data": map[string]string{"message": "Test delivery from UEM"},
	})
	status, err := w.post(ctx, h, "webhook.test", payload)
	_ = w.store.RecordWebhookDelivery(ctx, h.ID, status)
	return status, err
}

func (w *Webhooks) deliver(ctx context.Context, d delivery) {
	var status int
	var err error
	for attempt := range 3 {
		status, err = w.post(ctx, d.hook, d.event, d.payload)
		if err == nil && status < 500 {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(1<<attempt) * time.Second):
		}
	}
	if err != nil {
		slog.Warn("webhooks: delivery failed", "hook", d.hook.ID, "event", d.event, "err", err)
	}
	_ = w.store.RecordWebhookDelivery(ctx, d.hook.ID, status)
}

func (w *Webhooks) post(ctx context.Context, h store.Webhook, event string, payload []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "uem-webhooks/1")
	req.Header.Set("X-UEM-Event", event)
	req.Header.Set("X-UEM-Signature", "sha256="+Sign(h.Secret, payload))
	resp, err := w.client.Do(req)
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

// Sign returns the hex HMAC-SHA256 of body; receivers recompute it to verify.
func Sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

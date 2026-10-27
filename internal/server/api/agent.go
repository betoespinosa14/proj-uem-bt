package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/uem/uem/internal/protocol"
	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/auth"
	"github.com/uem/uem/internal/server/store"
)

const (
	agentReadTimeout = 90 * time.Second // agents heartbeat every 30s
	agentMaxMessage  = 16 << 20         // inventories with large software lists
)

func (s *Server) agentEnroll(w http.ResponseWriter, r *http.Request) {
	if !s.enrollLimiter.allow("enroll:" + clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "too many enrollment attempts")
		return
	}
	var req protocol.EnrollRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid enrollment request")
		return
	}
	secret := auth.RandomToken(32)
	dev, err := s.store.EnrollDevice(r.Context(), strings.TrimSpace(req.Token), req.Facts, auth.HashToken(secret))
	if err != nil {
		if errors.Is(err, store.ErrTokenInvalid) {
			writeErr(w, http.StatusUnauthorized, err.Error())
			return
		}
		fail(w, err)
		return
	}
	s.audit.Record(r.Context(), audit.Event{OrgID: dev.OrgID, Actor: audit.Actor{Type: "device", ID: dev.ID, IP: clientIP(r)},
		Action: "device.enroll", TargetType: "device", TargetID: dev.ID,
		Details: map[string]any{"hostname": dev.Hostname, "os": dev.OSName + " " + dev.OSVersion, "serial": dev.Serial, "group_id": dev.GroupID}})
	s.core.Hub.Publish(dev.OrgID, "device.updated", dev)
	s.core.Hooks.Emit(r.Context(), dev.OrgID, "device.enrolled", dev)
	writeJSON(w, http.StatusOK, protocol.EnrollResponse{DeviceID: dev.ID, DeviceSecret: secret})
}

// authenticateDevice checks "Authorization: Device <id>:<secret>".
func (s *Server) authenticateDevice(r *http.Request) (store.DeviceAuth, bool) {
	h := r.Header.Get("Authorization")
	cred, found := strings.CutPrefix(h, "Device ")
	if !found {
		return store.DeviceAuth{}, false
	}
	id, secret, found := strings.Cut(cred, ":")
	if !found || id == "" || secret == "" {
		return store.DeviceAuth{}, false
	}
	dev, err := s.store.GetDeviceAuth(r.Context(), id)
	if err != nil || dev.SecretHash == "" || !auth.ConstantTimeEqual(dev.SecretHash, auth.HashToken(secret)) {
		return store.DeviceAuth{}, false
	}
	return dev, true
}

// wsConn adapts a websocket to hub.Conn with a buffered writer goroutine so
// a slow peer never blocks the publisher.
type wsConn struct {
	c      *websocket.Conn
	out    chan []byte
	done   chan struct{}
	once   sync.Once
}

func newWSConn(c *websocket.Conn, buffer int) *wsConn {
	return &wsConn{c: c, out: make(chan []byte, buffer), done: make(chan struct{})}
}

func (w *wsConn) Send(frame []byte) bool {
	select {
	case w.out <- frame:
		return true
	case <-w.done:
		return false
	default:
		// Buffer full: the peer is too slow; drop it so it reconnects and resyncs.
		w.Close()
		return false
	}
}

func (w *wsConn) Close() {
	w.once.Do(func() {
		close(w.done)
		// The close handshake can wait on the peer; never block the caller.
		go w.c.Close(websocket.StatusGoingAway, "closing")
	})
}

func (w *wsConn) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case frame := <-w.out:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := w.c.Write(wctx, websocket.MessageText, frame)
			cancel()
			if err != nil {
				w.Close()
				return
			}
		}
	}
}

func (s *Server) agentWS(w http.ResponseWriter, r *http.Request) {
	dev, ok := s.authenticateDevice(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid device credentials")
		return
	}
	// Agents are not browsers; Origin checks don't apply.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	c.SetReadLimit(agentMaxMessage)
	// The request context is cancelled once the handler returns; detach so
	// work triggered by the final messages isn't aborted mid-write.
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()

	conn := newWSConn(c, 256)
	go conn.writeLoop(ctx)
	s.core.Hub.RegisterAgent(dev.ID, conn)
	log := slog.With("device", dev.ID)
	log.Info("agent connected", "ip", clientIP(r))

	defer func() {
		conn.Close()
		if s.core.Hub.UnregisterAgent(dev.ID, conn) {
			_ = s.store.SetDeviceOffline(ctx, dev.ID)
			_ = s.store.RequeueSent(ctx, dev.ID)
			s.core.PublishDevice(ctx, dev.OrgID, dev.ID)
		}
		log.Info("agent disconnected")
	}()

	for {
		rctx, rcancel := context.WithTimeout(ctx, agentReadTimeout)
		_, data, err := c.Read(rctx)
		rcancel()
		if err != nil {
			return
		}
		env, err := protocol.Decode(data)
		if err != nil {
			log.Warn("bad agent frame", "err", err)
			continue
		}
		s.handleAgentMessage(ctx, dev, env, log)
	}
}

func (s *Server) handleAgentMessage(ctx context.Context, dev store.DeviceAuth, env protocol.Envelope, log *slog.Logger) {
	switch env.Type {
	case protocol.TypeHello:
		var h protocol.Hello
		if json.Unmarshal(env.Data, &h) != nil {
			return
		}
		if err := s.store.SetDeviceOnline(ctx, dev.ID, h); err != nil {
			log.Error("set online", "err", err)
		}
		s.core.PushPolicy(ctx, dev.ID)
		s.core.FlushPending(ctx, dev.ID)
		s.core.PublishDevice(ctx, dev.OrgID, dev.ID)

	case protocol.TypeHeartbeat:
		_ = s.store.TouchDevice(ctx, dev.ID)

	case protocol.TypeTelemetry:
		var t protocol.Telemetry
		if json.Unmarshal(env.Data, &t) != nil {
			return
		}
		if t.TS.IsZero() || t.TS.After(time.Now().Add(5*time.Minute)) {
			t.TS = time.Now()
		}
		if err := s.store.RecordTelemetry(ctx, dev.ID, t); err != nil {
			log.Error("record telemetry", "err", err)
			return
		}
		s.core.PublishDevice(ctx, dev.OrgID, dev.ID)

	case protocol.TypeInventory:
		var inv protocol.Inventory
		if err := json.Unmarshal(env.Data, &inv); err != nil {
			log.Warn("bad inventory", "err", err)
			return
		}
		if inv.CollectedAt.IsZero() {
			inv.CollectedAt = time.Now()
		}
		if err := s.store.SaveInventory(ctx, dev.OrgID, dev.ID, inv); err != nil {
			log.Error("save inventory", "err", err)
			return
		}
		if err := s.core.EvaluateDevice(ctx, dev.OrgID, dev.ID); err != nil {
			log.Error("evaluate compliance", "err", err)
		}
		s.core.PublishDevice(ctx, dev.OrgID, dev.ID)

	case protocol.TypeCommandResult:
		var res protocol.CommandResult
		if json.Unmarshal(env.Data, &res) != nil {
			return
		}
		s.core.HandleCommandResult(ctx, dev, res)

	case protocol.TypeEvent:
		var ev protocol.Event
		if json.Unmarshal(env.Data, &ev) != nil {
			return
		}
		s.audit.Record(ctx, audit.Event{OrgID: dev.OrgID, Actor: audit.Actor{Type: "device", ID: dev.ID},
			Action: "device.event." + sanitizeKind(ev.Kind), TargetType: "device", TargetID: dev.ID,
			Details: map[string]any{"message": ev.Message, "details": ev.Details}})
	}
}

func sanitizeKind(k string) string {
	k = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return -1
	}, strings.ToLower(k))
	if k == "" || len(k) > 40 {
		return "other"
	}
	return k
}

// live streams org events to the browser console.
func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.cfg.OriginPatterns})
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	orgID := orgOf(r)
	conn := newWSConn(c, 512)
	go conn.writeLoop(ctx)
	s.core.Hub.Subscribe(orgID, conn)
	defer func() {
		s.core.Hub.Unsubscribe(orgID, conn)
		conn.Close()
	}()

	// Ping periodically so proxies keep the connection open and dead
	// clients are detected; the read loop just drains (browsers send nothing).
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-conn.done:
				return
			case <-t.C:
				pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
				err := c.Ping(pctx)
				pcancel()
				if err != nil {
					conn.Close()
					return
				}
			}
		}
	}()
	// Re-validate the session periodically so logout/revocation ends the stream.
	sessID := who(r).Session.ID
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-conn.done:
				return
			case <-t.C:
				if _, _, err := s.store.GetSession(ctx, sessID); err != nil {
					conn.Close()
					return
				}
			}
		}
	}()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return
		}
	}
}

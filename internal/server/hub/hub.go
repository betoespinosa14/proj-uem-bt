// Package hub tracks live WebSocket connections: one per connected agent,
// and any number of browser subscribers per organization.
package hub

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// Conn is a connection the hub can push frames to. Send must not block;
// implementations buffer and drop (or disconnect) slow consumers.
type Conn interface {
	Send(frame []byte) bool
	Close()
}

type Hub struct {
	mu     sync.RWMutex
	agents map[string]Conn               // deviceID → conn
	live   map[string]map[Conn]struct{} // orgID → browser conns
}

func New() *Hub {
	return &Hub{agents: map[string]Conn{}, live: map[string]map[Conn]struct{}{}}
}

// RegisterAgent records the device's connection, closing any previous one
// (an agent that reconnects before the old socket timed out).
func (h *Hub) RegisterAgent(deviceID string, c Conn) {
	h.mu.Lock()
	old := h.agents[deviceID]
	h.agents[deviceID] = c
	h.mu.Unlock()
	if old != nil && old != c {
		old.Close()
	}
}

// UnregisterAgent removes c if it is still the device's current connection,
// and reports whether it was.
func (h *Hub) UnregisterAgent(deviceID string, c Conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.agents[deviceID] == c {
		delete(h.agents, deviceID)
		return true
	}
	return false
}

func (h *Hub) AgentConnected(deviceID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.agents[deviceID]
	return ok
}

// SendToAgent pushes a frame to a connected agent; false if not connected
// or its buffer is full.
func (h *Hub) SendToAgent(deviceID string, frame []byte) bool {
	h.mu.RLock()
	c := h.agents[deviceID]
	h.mu.RUnlock()
	if c == nil {
		return false
	}
	return c.Send(frame)
}

// DisconnectAgent drops a device's connection, e.g. after it is retired.
func (h *Hub) DisconnectAgent(deviceID string) {
	h.mu.Lock()
	c := h.agents[deviceID]
	delete(h.agents, deviceID)
	h.mu.Unlock()
	if c != nil {
		c.Close()
	}
}

func (h *Hub) Subscribe(orgID string, c Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.live[orgID] == nil {
		h.live[orgID] = map[Conn]struct{}{}
	}
	h.live[orgID][c] = struct{}{}
}

func (h *Hub) Unsubscribe(orgID string, c Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.live[orgID], c)
	if len(h.live[orgID]) == 0 {
		delete(h.live, orgID)
	}
}

// Publish sends a {type, data} event to every browser subscribed to the org.
func (h *Hub) Publish(orgID, typ string, data any) {
	frame, err := json.Marshal(map[string]any{"type": typ, "data": data})
	if err != nil {
		slog.Error("hub: marshal event", "type", typ, "err", err)
		return
	}
	h.mu.RLock()
	conns := make([]Conn, 0, len(h.live[orgID]))
	for c := range h.live[orgID] {
		conns = append(conns, c)
	}
	h.mu.RUnlock()
	for _, c := range conns {
		c.Send(frame)
	}
}

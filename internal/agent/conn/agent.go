// Package conn runs the agent's connection to the server: it keeps a
// WebSocket open with reconnect/backoff, reports heartbeats, telemetry and
// inventory, executes commands, and enforces the app blocklist.
package conn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/uem/uem/internal/agent/collect"
	"github.com/uem/uem/internal/agent/config"
	agentexec "github.com/uem/uem/internal/agent/exec"
	"github.com/uem/uem/internal/agent/session"
	"github.com/uem/uem/internal/agent/usage"
	"github.com/uem/uem/internal/protocol"
)

const (
	heartbeatInterval = 30 * time.Second
	blocklistInterval = 15 * time.Second
	usageSampleEvery  = time.Minute
	usageReportEvery  = 5 // samples between usage reports
	maxConcurrentCmds = 4
	maxPendingResults = 200
)

// Agent is the long-running agent process.
type Agent struct {
	cfg     config.Config
	version string
	log     *slog.Logger

	// OnUninstall is invoked after an uninstall_agent command is acknowledged.
	OnUninstall func()

	mu      sync.Mutex
	policy  protocol.Policy
	out     chan []byte // frames for the current connection, nil when offline
	pending [][]byte    // command results produced while offline
	seen    map[string]*protocol.CommandResult // command ID → result (nil while running)

	cmdSlots chan struct{}
	refresh  chan struct{}
	usage    *usage.Tracker
}

func New(cfg config.Config, version string, log *slog.Logger) *Agent {
	return &Agent{
		cfg: cfg, version: version, log: log,
		policy:   protocol.Policy{TelemetryIntervalSec: 60, InventoryIntervalSec: 6 * 3600},
		seen:     map[string]*protocol.CommandResult{},
		cmdSlots: make(chan struct{}, maxConcurrentCmds),
		refresh:  make(chan struct{}, 1),
		usage:    usage.NewTracker(runtime.GOOS),
	}
}

// Run connects and reconnects until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	go a.enforceBlocklist(ctx)
	go a.trackUsage(ctx)
	go a.superviseSessions(ctx)
	backoff := time.Second
	for {
		start := time.Now()
		err := a.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > time.Minute {
			backoff = time.Second // the session was healthy; reconnect quickly
		}
		wait := backoff + time.Duration(rand.Int64N(int64(backoff/2)+1))
		if errors.Is(err, errUnauthorized) {
			// Retired or credential revoked: retry rarely in case it's transient.
			wait = 5 * time.Minute
		}
		a.log.Warn("disconnected from server", "err", err, "retry_in", wait.Round(time.Second))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

var errUnauthorized = errors.New("server rejected device credentials (device may have been retired)")

func wsURL(server string) string {
	u := strings.TrimRight(server, "/")
	switch {
	case strings.HasPrefix(u, "https://"):
		u = "wss://" + strings.TrimPrefix(u, "https://")
	case strings.HasPrefix(u, "http://"):
		u = "ws://" + strings.TrimPrefix(u, "http://")
	}
	return u + "/api/agent/ws"
}

func (a *Agent) session(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(dialCtx, wsURL(a.cfg.ServerURL), &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Device " + a.cfg.DeviceID + ":" + a.cfg.DeviceSecret}},
	})
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			return errUnauthorized
		}
		return err
	}
	defer c.CloseNow()
	c.SetReadLimit(4 << 20)
	a.log.Info("connected to server", "server", a.cfg.ServerURL)

	sctx, scancel := context.WithCancel(ctx)
	defer scancel()
	out := make(chan []byte, 64)
	a.mu.Lock()
	a.out = out
	pending := a.pending
	a.pending = nil
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.out = nil
		a.mu.Unlock()
	}()

	errc := make(chan error, 2)
	go func() { errc <- a.writeLoop(sctx, c, out) }()
	go func() { errc <- a.readLoop(sctx, c) }()

	a.send(protocol.TypeHello, protocol.Hello{
		ProtocolVersion: protocol.Version, AgentVersion: a.version, Facts: collect.Facts(sctx, a.version),
	})
	for _, f := range pending {
		a.sendFrame(f)
	}
	go a.reportLoop(sctx)

	err = <-errc
	return err
}

func (a *Agent) writeLoop(ctx context.Context, c *websocket.Conn, out <-chan []byte) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case f := <-out:
			wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err := c.Write(wctx, websocket.MessageText, f)
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

func (a *Agent) readLoop(ctx context.Context, c *websocket.Conn) error {
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return err
		}
		env, err := protocol.Decode(data)
		if err != nil {
			continue
		}
		switch env.Type {
		case protocol.TypeCommand:
			var cmd protocol.Command
			if json.Unmarshal(env.Data, &cmd) == nil {
				a.handleCommand(ctx, cmd)
			}
		case protocol.TypePolicy:
			var p protocol.Policy
			if json.Unmarshal(env.Data, &p) == nil {
				a.mu.Lock()
				a.policy = p
				a.mu.Unlock()
				a.log.Info("policy updated", "blocked_apps", len(p.BlockedApps), "usage_tracking", p.UsageTracking)
			}
		}
	}
}

// reportLoop sends heartbeats, telemetry and inventory for one session.
func (a *Agent) reportLoop(ctx context.Context) {
	a.sendInventory(ctx)
	a.send(protocol.TypeTelemetry, collect.Telemetry(ctx))

	hb := time.NewTicker(heartbeatInterval)
	defer hb.Stop()
	lastTelemetry, lastInventory := time.Now(), time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.refresh:
			a.sendInventory(ctx)
			lastInventory = time.Now()
		case <-hb.C:
			a.send(protocol.TypeHeartbeat, struct{}{})
			p := a.currentPolicy()
			if time.Since(lastTelemetry) >= seconds(p.TelemetryIntervalSec, time.Minute)-time.Second {
				a.send(protocol.TypeTelemetry, collect.Telemetry(ctx))
				lastTelemetry = time.Now()
			}
			if time.Since(lastInventory) >= seconds(p.InventoryIntervalSec, 6*time.Hour) {
				a.sendInventory(ctx)
				lastInventory = time.Now()
			}
		}
	}
}

func seconds(n int, def time.Duration) time.Duration {
	if n <= 0 {
		return def
	}
	return time.Duration(n) * time.Second
}

func (a *Agent) sendInventory(ctx context.Context) {
	start := time.Now()
	inv := collect.Inventory(ctx)
	if ctx.Err() != nil {
		return
	}
	a.log.Info("inventory collected", "software", len(inv.Software), "took", time.Since(start).Round(time.Millisecond))
	a.usage.SetSoftware(inv.Software)
	a.send(protocol.TypeInventory, inv)
}

func (a *Agent) currentPolicy() protocol.Policy {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.policy
}

func (a *Agent) send(typ string, v any) {
	f, err := protocol.Encode(typ, v)
	if err != nil {
		a.log.Error("encode frame", "type", typ, "err", err)
		return
	}
	a.sendFrame(f)
}

func (a *Agent) sendFrame(f []byte) bool {
	a.mu.Lock()
	out := a.out
	a.mu.Unlock()
	if out == nil {
		return false
	}
	select {
	case out <- f:
		return true
	case <-time.After(10 * time.Second):
		return false
	}
}

// sendResult delivers a command result, queueing it if offline so it is
// reported after reconnecting.
func (a *Agent) sendResult(res protocol.CommandResult) {
	f, err := protocol.Encode(protocol.TypeCommandResult, res)
	if err != nil {
		return
	}
	if a.sendFrame(f) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if protocol.IsTerminal(res.Status) && len(a.pending) < maxPendingResults {
		a.pending = append(a.pending, f)
	}
}

func (a *Agent) handleCommand(ctx context.Context, cmd protocol.Command) {
	a.mu.Lock()
	prev, dup := a.seen[cmd.ID]
	if !dup {
		a.seen[cmd.ID] = nil
		if len(a.seen) > 1000 { // bounded memory; old IDs won't be redelivered
			for k, v := range a.seen {
				if v != nil {
					delete(a.seen, k)
				}
			}
		}
	}
	a.mu.Unlock()
	if dup {
		// Redelivered after a reconnect: never re-run, just re-report.
		if prev != nil {
			a.sendResult(*prev)
		}
		return
	}
	a.log.Info("command received", "id", cmd.ID, "type", cmd.Type)
	a.sendResult(protocol.CommandResult{ID: cmd.ID, Status: protocol.StatusRunning})

	// Commands outlive the connection that delivered them.
	go func() {
		a.cmdSlots <- struct{}{}
		defer func() { <-a.cmdSlots }()
		res := a.execute(context.WithoutCancel(ctx), cmd)
		res.ID = cmd.ID
		a.mu.Lock()
		a.seen[cmd.ID] = &res
		a.mu.Unlock()
		a.log.Info("command finished", "id", cmd.ID, "status", res.Status)
		a.sendResult(res)
		a.afterResult(cmd)
	}()
}

func succeeded(stdout string) protocol.CommandResult {
	code := 0
	return protocol.CommandResult{Status: protocol.StatusSucceeded, ExitCode: &code, Stdout: stdout}
}

func failed(err error) protocol.CommandResult {
	return protocol.CommandResult{Status: protocol.StatusFailed, Error: err.Error()}
}

func (a *Agent) execute(ctx context.Context, cmd protocol.Command) protocol.CommandResult {
	switch cmd.Type {
	case protocol.CmdRunScript:
		var p protocol.RunScriptPayload
		if err := json.Unmarshal(cmd.Payload, &p); err != nil {
			return failed(fmt.Errorf("invalid payload: %w", err))
		}
		return agentexec.RunScript(ctx, p)
	case protocol.CmdReboot, protocol.CmdShutdown:
		var p protocol.DelayPayload
		_ = json.Unmarshal(cmd.Payload, &p)
		// Report success before acting: once the machine goes down we can't.
		verb := map[string]string{protocol.CmdReboot: "Reboot", protocol.CmdShutdown: "Shutdown"}[cmd.Type]
		return succeeded(fmt.Sprintf("%s scheduled in %ds", verb, p.DelaySeconds))
	case protocol.CmdLock:
		if err := agentexec.Lock(ctx); err != nil {
			return failed(err)
		}
		return succeeded("Session locked")
	case protocol.CmdDisplayMessage:
		var p protocol.MessagePayload
		_ = json.Unmarshal(cmd.Payload, &p)
		if err := agentexec.DisplayMessage(ctx, p.Title, p.Message); err != nil {
			return failed(err)
		}
		return succeeded("Message displayed")
	case protocol.CmdRefreshInventory:
		select {
		case a.refresh <- struct{}{}:
		default:
		}
		return succeeded("Inventory refresh requested")
	case protocol.CmdUninstallAgent:
		if a.OnUninstall == nil {
			return failed(errors.New("uninstall is not supported when running in the foreground"))
		}
		return succeeded("Agent uninstalling")
	}
	return failed(fmt.Errorf("unsupported command type %q", cmd.Type))
}

// afterResult performs actions that must happen after the result is sent.
func (a *Agent) afterResult(cmd protocol.Command) {
	switch cmd.Type {
	case protocol.CmdReboot, protocol.CmdShutdown:
		var p protocol.DelayPayload
		_ = json.Unmarshal(cmd.Payload, &p)
		time.Sleep(3 * time.Second) // let the result flush
		if err := agentexec.Power(cmd.Type == protocol.CmdReboot, time.Duration(p.DelaySeconds)*time.Second); err != nil {
			a.log.Error("power action failed", "err", err)
		}
	case protocol.CmdUninstallAgent:
		if a.OnUninstall != nil {
			time.Sleep(3 * time.Second)
			a.OnUninstall()
		}
	}
}

// enforceBlocklist terminates processes named in the device's blocklist.
func (a *Agent) enforceBlocklist(ctx context.Context) {
	t := time.NewTicker(blocklistInterval)
	defer t.Stop()
	self := int32(os.Getpid())
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		blocked := map[string]bool{}
		for _, b := range a.currentPolicy().BlockedApps {
			blocked[normalizeProc(b)] = true
		}
		if len(blocked) == 0 {
			continue
		}
		procs, err := process.ProcessesWithContext(ctx)
		if err != nil {
			continue
		}
		for _, p := range procs {
			if p.Pid <= 1 || p.Pid == self {
				continue
			}
			name, err := p.NameWithContext(ctx)
			if err != nil || !blocked[normalizeProc(name)] {
				continue
			}
			if err := p.KillWithContext(ctx); err != nil {
				a.log.Warn("failed to stop blocked app", "name", name, "pid", p.Pid, "err", err)
				continue
			}
			a.log.Info("stopped blocked app", "name", name, "pid", p.Pid)
			user, _ := p.UsernameWithContext(ctx)
			a.send(protocol.TypeEvent, protocol.Event{
				Kind: "app_blocked", Message: fmt.Sprintf("Blocked application %q was terminated", name),
				Details: map[string]any{"process": name, "pid": p.Pid, "user": user},
			})
		}
	}
}

// trackUsage samples running applications once a minute while the policy
// enables usage tracking, and reports the accumulated minutes periodically.
// Undelivered minutes are kept and sent after reconnecting.
func (a *Agent) trackUsage(ctx context.Context) {
	t := time.NewTicker(usageSampleEvery)
	defer t.Stop()
	samples := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if !a.currentPolicy().UsageTracking {
			a.usage.Reset()
			samples = 0
			continue
		}
		procs, err := process.ProcessesWithContext(ctx)
		if err != nil {
			continue
		}
		sample := make([]usage.Proc, 0, len(procs))
		for _, p := range procs {
			exe, err := p.ExeWithContext(ctx)
			if err != nil || exe == "" {
				continue
			}
			user, _ := p.UsernameWithContext(ctx)
			uid := int64(-1)
			if uids, err := p.UidsWithContext(ctx); err == nil && len(uids) > 0 {
				uid = int64(uids[0])
			}
			sample = append(sample, usage.Proc{Exe: exe, User: user, UID: uid})
		}
		a.usage.Sample(time.Now(), sample)
		if samples++; samples < usageReportEvery {
			continue
		}
		samples = 0
		entries := a.usage.Flush()
		if len(entries) == 0 {
			continue
		}
		f, err := protocol.Encode(protocol.TypeUsage, protocol.Usage{Entries: entries})
		if err != nil || !a.sendFrame(f) {
			a.usage.Restore(entries)
		}
	}
}

// superviseSessions runs a probe in each interactive user session while
// usage tracking is on, crediting active time to the front app.
func (a *Agent) superviseSessions(ctx context.Context) {
	exe, err := os.Executable()
	if err != nil {
		a.log.Warn("active usage disabled: can't locate agent binary", "err", err)
		return
	}
	sup := &session.Supervisor{Exe: exe, Log: a.log, OnSample: func(user string, s session.Sample, at time.Time) {
		p := a.currentPolicy()
		a.usage.Active(user, s.AppPath, time.Duration(s.IdleMS)*time.Millisecond, seconds(p.IdleThresholdSec, 2*time.Minute), at)
	}}
	sup.Run(ctx, func() bool { return a.currentPolicy().UsageTracking })
}

// normalizeProc makes "Chrome.exe", "chrome" and "/usr/bin/chrome" compare equal.
func normalizeProc(name string) string {
	name = strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	return strings.TrimSuffix(name, ".exe")
}

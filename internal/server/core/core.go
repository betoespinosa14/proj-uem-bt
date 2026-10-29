// Package core wires the store, live hub, audit log and webhooks together
// for the operations shared by the REST API, the agent gateway, and the
// background scheduler: issuing commands, ingesting results, evaluating
// compliance, and broadcasting changes.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/uem/uem/internal/protocol"
	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/compliance"
	"github.com/uem/uem/internal/server/hub"
	"github.com/uem/uem/internal/server/notify"
	"github.com/uem/uem/internal/server/store"
)

type Core struct {
	Store *store.Store
	Audit *audit.Log
	Hub   *hub.Hub
	Hooks *notify.Webhooks

	TelemetryInterval time.Duration
	InventoryInterval time.Duration
}

// ValidationError is returned for bad client input and maps to HTTP 400.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// ---- Broadcasting ----

func (c *Core) PublishDevice(ctx context.Context, orgID, deviceID string) {
	d, err := c.Store.GetDevice(ctx, orgID, deviceID)
	if err != nil {
		return
	}
	c.Hub.Publish(orgID, "device.updated", d)
}

func (c *Core) publishCommand(cmd store.Command) {
	c.Hub.Publish(cmd.OrgID, "command.updated", cmd)
}

// ---- Commands ----

var validShells = []string{"bash", "sh", "zsh", "powershell", "cmd"}

// ResolvePayload validates a command and normalizes its payload. Library
// scripts are inlined so the command row records exactly what ran, even if
// the script is edited later.
func (c *Core) ResolvePayload(ctx context.Context, orgID, typ string, payload json.RawMessage) (json.RawMessage, error) {
	if !slices.Contains(protocol.CommandTypes, typ) {
		return nil, invalid("unknown command type %q", typ)
	}
	if len(payload) == 0 || string(payload) == "null" {
		payload = json.RawMessage(`{}`)
	}
	switch typ {
	case protocol.CmdRunScript:
		var p struct {
			ScriptID       string `json:"script_id"`
			Shell          string `json:"shell"`
			Script         string `json:"script"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, invalid("invalid run_script payload")
		}
		out := map[string]any{}
		if p.ScriptID != "" {
			sc, err := c.Store.GetScript(ctx, orgID, p.ScriptID)
			if err != nil {
				return nil, invalid("script not found")
			}
			p.Shell, p.Script = sc.Shell, sc.Body
			if p.TimeoutSeconds == 0 {
				p.TimeoutSeconds = sc.TimeoutSeconds
			}
			out["script_id"], out["script_name"] = sc.ID, sc.Name
		}
		if !slices.Contains(validShells, p.Shell) {
			return nil, invalid("shell must be one of %s", strings.Join(validShells, ", "))
		}
		if strings.TrimSpace(p.Script) == "" {
			return nil, invalid("script body is required")
		}
		if p.TimeoutSeconds <= 0 {
			p.TimeoutSeconds = 300
		}
		if p.TimeoutSeconds > 3600 {
			return nil, invalid("timeout_seconds must be at most 3600")
		}
		out["shell"], out["script"], out["timeout_seconds"] = p.Shell, p.Script, p.TimeoutSeconds
		return json.Marshal(out)
	case protocol.CmdReboot, protocol.CmdShutdown:
		var p protocol.DelayPayload
		if err := json.Unmarshal(payload, &p); err != nil || p.DelaySeconds < 0 || p.DelaySeconds > 86400 {
			return nil, invalid("delay_seconds must be between 0 and 86400")
		}
		return json.Marshal(p)
	case protocol.CmdDisplayMessage:
		var p protocol.MessagePayload
		if err := json.Unmarshal(payload, &p); err != nil || strings.TrimSpace(p.Message) == "" {
			return nil, invalid("message is required")
		}
		if p.Title == "" {
			p.Title = "Message from IT"
		}
		return json.Marshal(p)
	default:
		return json.RawMessage(`{}`), nil
	}
}

// IssueCommand validates, queues, audits and (if the device is connected)
// immediately dispatches a command.
func (c *Core) IssueCommand(ctx context.Context, orgID, deviceID, typ string, payload json.RawMessage, actor audit.Actor, source string) (store.Command, error) {
	resolved, err := c.ResolvePayload(ctx, orgID, typ, payload)
	if err != nil {
		return store.Command{}, err
	}
	userID := ""
	if actor.Type == "user" {
		userID = actor.ID
	}
	cmd, err := c.Store.CreateCommand(ctx, orgID, deviceID, typ, resolved, userID, source)
	if err != nil {
		return store.Command{}, err
	}
	details := map[string]any{"type": typ, "source": source, "device_name": cmd.DeviceName, "command_id": cmd.ID}
	if typ == protocol.CmdRunScript {
		var p map[string]any
		_ = json.Unmarshal(resolved, &p)
		details["shell"], details["script"] = p["shell"], p["script"]
		if name, ok := p["script_name"]; ok {
			details["script_name"] = name
		}
	}
	c.Audit.Record(ctx, audit.Event{OrgID: orgID, Actor: actor, Action: "device.command.create",
		TargetType: "device", TargetID: deviceID, Details: details})
	c.publishCommand(cmd)
	c.trySend(ctx, cmd)
	return cmd, nil
}

func (c *Core) trySend(ctx context.Context, cmd store.Command) {
	frame, err := protocol.Encode(protocol.TypeCommand, protocol.Command{ID: cmd.ID, Type: cmd.Type, Payload: cmd.Payload})
	if err != nil {
		return
	}
	if !c.Hub.SendToAgent(cmd.DeviceID, frame) {
		return
	}
	if sent, err := c.Store.MarkCommandSent(ctx, cmd.ID); err == nil {
		c.publishCommand(sent)
	}
}

// FlushPending delivers commands queued while the device was offline.
func (c *Core) FlushPending(ctx context.Context, deviceID string) {
	cmds, err := c.Store.PendingCommands(ctx, deviceID)
	if err != nil {
		slog.Error("flush pending commands", "device", deviceID, "err", err)
		return
	}
	for _, cmd := range cmds {
		c.trySend(ctx, cmd)
	}
}

// HandleCommandResult records a result reported by the device that owns the command.
func (c *Core) HandleCommandResult(ctx context.Context, dev store.DeviceAuth, res protocol.CommandResult) {
	if res.Status != protocol.StatusRunning && !protocol.IsTerminal(res.Status) {
		return
	}
	cmd, err := c.Store.ApplyCommandResult(ctx, dev.ID, res)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Error("apply command result", "command", res.ID, "err", err)
		}
		return
	}
	c.publishCommand(cmd)
	if protocol.IsTerminal(cmd.Status) {
		c.Audit.Record(ctx, audit.Event{OrgID: dev.OrgID, Actor: audit.Actor{Type: "device", ID: dev.ID},
			Action: "device.command.result", TargetType: "command", TargetID: cmd.ID,
			Details: map[string]any{"type": cmd.Type, "status": cmd.Status, "exit_code": cmd.ExitCode, "device_name": cmd.DeviceName}})
		c.Hooks.Emit(ctx, dev.OrgID, "command.completed", cmd)
	}
}

// ---- Policy ----

func (c *Core) PolicyFor(ctx context.Context, deviceID string) protocol.Policy {
	apps, err := c.Store.BlockedAppsForDevice(ctx, deviceID)
	if err != nil {
		apps = []string{}
	}
	usage, _ := c.Store.UsageTrackingForDevice(ctx, deviceID) // off on error
	return protocol.Policy{
		BlockedApps:          apps,
		UsageTracking:        usage,
		TelemetryIntervalSec: int(c.TelemetryInterval.Seconds()),
		InventoryIntervalSec: int(c.InventoryInterval.Seconds()),
	}
}

func (c *Core) PushPolicy(ctx context.Context, deviceID string) {
	frame, err := protocol.Encode(protocol.TypePolicy, c.PolicyFor(ctx, deviceID))
	if err == nil {
		c.Hub.SendToAgent(deviceID, frame)
	}
}

// PushOrgPolicy re-sends policy to every connected device in an org.
func (c *Core) PushOrgPolicy(ctx context.Context, orgID string) {
	ids, err := c.Store.OnlineDeviceIDs(ctx, orgID)
	if err != nil {
		return
	}
	for _, id := range ids {
		c.PushPolicy(ctx, id)
	}
}

// PushGroupPolicy re-sends policy to every connected device in a group.
func (c *Core) PushGroupPolicy(ctx context.Context, orgID, groupID string) {
	devices, err := c.Store.ListDevices(ctx, orgID, store.DeviceFilter{GroupID: groupID, Status: "online"})
	if err != nil {
		return
	}
	for _, d := range devices {
		c.PushPolicy(ctx, d.ID)
	}
}

// ---- Compliance ----

// EvaluateDevice runs every applicable rule against a device, applies
// violation actions for newly failing rules, and updates the rolled-up state.
func (c *Core) EvaluateDevice(ctx context.Context, orgID, deviceID string) error {
	dev, err := c.Store.GetDevice(ctx, orgID, deviceID)
	if err != nil {
		return err
	}
	inv, err := c.Store.GetInventory(ctx, orgID, deviceID)
	if err != nil {
		return err
	}
	rules, err := c.Store.ListRules(ctx, orgID)
	if err != nil {
		return err
	}
	var software []store.Software
	if inv != nil {
		if software, err = c.Store.ListSoftware(ctx, orgID, deviceID); err != nil {
			return err
		}
	}
	facts := compliance.Facts{Device: dev, Inventory: inv, Software: software, Now: time.Now()}

	applied := []string{}
	state := "compliant"
	for _, r := range rules {
		if !compliance.Applies(r, dev) {
			continue
		}
		applied = append(applied, r.ID)
		passed, details := compliance.Evaluate(r, facts)
		newlyFailing, err := c.Store.SaveResult(ctx, orgID, deviceID, r.ID, passed, details)
		if err != nil {
			return err
		}
		if passed {
			_ = c.Store.AutoResolveAlert(ctx, deviceID, r.ID)
			continue
		}
		state = "non_compliant"
		if newlyFailing {
			c.onViolation(ctx, dev, r, details)
		}
	}
	if err := c.Store.DeleteResultsExcept(ctx, deviceID, applied); err != nil {
		return err
	}
	// Without inventory nothing meaningful has been verified yet.
	if inv == nil {
		state = "unknown"
	}
	changed, err := c.Store.SetComplianceState(ctx, deviceID, state)
	if err != nil {
		return err
	}
	if changed {
		c.Audit.Record(ctx, audit.Event{OrgID: orgID, Actor: audit.System(), Action: "device.compliance.change",
			TargetType: "device", TargetID: deviceID,
			Details: map[string]any{"device_name": dev.Name, "from": dev.ComplianceState, "to": state}})
		c.PublishDevice(ctx, orgID, deviceID)
	}
	return nil
}

func (c *Core) onViolation(ctx context.Context, dev store.Device, r store.Rule, details string) {
	c.Audit.Record(ctx, audit.Event{OrgID: dev.OrgID, Actor: audit.System(), Action: "compliance.violation",
		TargetType: "device", TargetID: dev.ID,
		Details: map[string]any{"device_name": dev.Name, "rule_id": r.ID, "rule_name": r.Name, "severity": r.Severity, "details": details}})
	if r.Actions.Alert {
		alert, created, err := c.Store.CreateAlert(ctx, dev.OrgID, &dev.ID, &r.ID, r.Severity,
			fmt.Sprintf("%s: %s", dev.Name, r.Name), details)
		if err != nil {
			slog.Error("create alert", "err", err)
		} else if created {
			c.Hub.Publish(dev.OrgID, "alert.created", alert)
			c.Hooks.Emit(ctx, dev.OrgID, "alert.created", alert)
		}
	}
	if r.Actions.Webhook {
		c.Hooks.Emit(ctx, dev.OrgID, "compliance.violation", map[string]any{
			"device": dev, "rule": map[string]any{"id": r.ID, "name": r.Name, "severity": r.Severity}, "details": details,
		})
	}
	if r.Actions.Tag != nil && strings.TrimSpace(*r.Actions.Tag) != "" {
		if err := c.Store.AddDeviceTag(ctx, dev.OrgID, dev.ID, strings.TrimSpace(*r.Actions.Tag)); err == nil {
			c.PublishDevice(ctx, dev.OrgID, dev.ID)
		}
	}
	if r.Actions.Command != nil && r.Actions.Command.Type != "" {
		if _, err := c.IssueCommand(ctx, dev.OrgID, dev.ID, r.Actions.Command.Type, r.Actions.Command.Payload,
			audit.System(), "compliance"); err != nil {
			slog.Warn("compliance remediation command", "rule", r.ID, "err", err)
		}
	}
}

// EvaluateOrg re-evaluates every device in an org.
func (c *Core) EvaluateOrg(ctx context.Context, orgID string) (int, error) {
	devices, err := c.Store.ListDevices(ctx, orgID, store.DeviceFilter{})
	if err != nil {
		return 0, err
	}
	for _, d := range devices {
		if err := c.EvaluateDevice(ctx, orgID, d.ID); err != nil {
			slog.Error("evaluate device", "device", d.ID, "err", err)
		}
	}
	return len(devices), nil
}

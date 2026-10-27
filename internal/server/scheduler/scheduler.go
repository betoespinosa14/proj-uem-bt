// Package scheduler runs admin-defined cron tasks and the server's own
// housekeeping loops (offline detection, command timeouts, retention).
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/uem/uem/internal/server/audit"
	"github.com/uem/uem/internal/server/core"
	"github.com/uem/uem/internal/server/store"
)

const ActionComplianceCheck = "compliance_check"

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ValidateCron checks a standard 5-field cron expression.
func ValidateCron(expr string) error {
	if _, err := parser.Parse(expr); err != nil {
		return fmt.Errorf("invalid cron expression: %v", err)
	}
	return nil
}

type Scheduler struct {
	core *core.Core
	cron *cron.Cron

	mu      sync.Mutex
	entries map[string]cron.EntryID // taskID → cron entry
	ctx     context.Context

	OfflineAfter time.Duration
}

func New(c *core.Core) *Scheduler {
	return &Scheduler{
		core:         c,
		cron:         cron.New(cron.WithParser(parser), cron.WithLocation(time.UTC)),
		entries:      map[string]cron.EntryID{},
		OfflineAfter: 90 * time.Second,
	}
}

// Start loads tasks and begins the cron and housekeeping loops.
func (s *Scheduler) Start(ctx context.Context) error {
	s.ctx = ctx
	if err := s.Reload(ctx); err != nil {
		return err
	}
	s.cron.Start()
	go func() {
		<-ctx.Done()
		s.cron.Stop()
	}()
	go s.every(ctx, 15*time.Second, s.markOffline)
	go s.every(ctx, time.Minute, s.expireCommands)
	go s.every(ctx, time.Hour, s.retention)
	return nil
}

// Reload replaces all cron entries with the current enabled tasks.
func (s *Scheduler) Reload(ctx context.Context) error {
	tasks, err := s.core.Store.ListAllEnabledTasks(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.entries {
		s.cron.Remove(id)
	}
	s.entries = map[string]cron.EntryID{}
	for _, t := range tasks {
		taskID, orgID := t.ID, t.OrgID
		id, err := s.cron.AddFunc(t.Cron, func() {
			// Re-read so edits made since the last reload are honored.
			task, err := s.core.Store.GetTask(s.ctx, orgID, taskID)
			if err != nil || !task.Enabled {
				return
			}
			if _, err := s.Run(s.ctx, task, audit.System()); err != nil {
				slog.Error("scheduled task failed", "task", taskID, "err", err)
			}
		})
		if err != nil {
			slog.Warn("skipping task with invalid cron", "task", t.ID, "cron", t.Cron, "err", err)
			continue
		}
		s.entries[t.ID] = id
	}
	return nil
}

// NextRun returns the next scheduled time of a task, if it is scheduled.
func (s *Scheduler) NextRun(taskID string) *time.Time {
	s.mu.Lock()
	id, ok := s.entries[taskID]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	next := s.cron.Entry(id).Next
	if next.IsZero() {
		return nil
	}
	return &next
}

// Run executes a task now against its targets and returns how many devices it touched.
func (s *Scheduler) Run(ctx context.Context, t store.Task, actor audit.Actor) (int, error) {
	filter := store.DeviceFilter{}
	switch {
	case t.Target.All:
	case t.Target.GroupID != nil:
		filter.GroupID = *t.Target.GroupID
	default:
		filter.IDs = t.Target.DeviceIDs
	}
	devices, err := s.core.Store.ListDevices(ctx, t.OrgID, filter)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, d := range devices {
		if t.Action.Type == ActionComplianceCheck {
			err = s.core.EvaluateDevice(ctx, t.OrgID, d.ID)
		} else {
			_, err = s.core.IssueCommand(ctx, t.OrgID, d.ID, t.Action.Type, t.Action.Payload, actor, "scheduler")
		}
		if err != nil {
			slog.Warn("task target failed", "task", t.ID, "device", d.ID, "err", err)
			continue
		}
		n++
	}
	now := time.Now()
	_ = s.core.Store.TouchTaskRun(ctx, t.ID, now)
	s.core.Audit.Record(ctx, audit.Event{OrgID: t.OrgID, Actor: actor, Action: "task.run", TargetType: "task", TargetID: t.ID,
		Details: map[string]any{"name": t.Name, "action": t.Action.Type, "devices": n}})
	return n, nil
}

func (s *Scheduler) every(ctx context.Context, d time.Duration, fn func(context.Context)) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}

func (s *Scheduler) markOffline(ctx context.Context) {
	refs, err := s.core.Store.MarkStaleOffline(ctx, time.Now().Add(-s.OfflineAfter))
	if err != nil {
		slog.Error("mark offline", "err", err)
		return
	}
	for _, r := range refs {
		s.core.Hub.DisconnectAgent(r.ID)
		s.core.PublishDevice(ctx, r.OrgID, r.ID)
		if d, err := s.core.Store.GetDevice(ctx, r.OrgID, r.ID); err == nil {
			s.core.Hooks.Emit(ctx, r.OrgID, "device.offline", d)
		}
	}
}

func (s *Scheduler) expireCommands(ctx context.Context) {
	// Scripts may run up to an hour; allow margin before declaring them lost.
	cmds, err := s.core.Store.ExpireStaleCommands(ctx, 75*time.Minute)
	if err != nil {
		slog.Error("expire commands", "err", err)
		return
	}
	for _, c := range cmds {
		s.core.Hub.Publish(c.OrgID, "command.updated", c)
	}
}

func (s *Scheduler) retention(ctx context.Context) {
	now := time.Now()
	if err := s.core.Store.PurgeTelemetry(ctx, now.Add(-30*24*time.Hour)); err != nil {
		slog.Error("purge telemetry", "err", err)
	}
	if err := s.core.Store.PurgeExpiredSessions(ctx, now); err != nil {
		slog.Error("purge sessions", "err", err)
	}
}

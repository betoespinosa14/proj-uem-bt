package core

import (
	"context"
	"time"

	"github.com/uem/uem/internal/agentdist"
	"github.com/uem/uem/internal/server/store"
)

// TrendDays is how many days of history the Dashboard shows.
const TrendDays = 30

// SnapshotRetention is how long daily snapshots are kept.
const SnapshotRetention = 400 * 24 * time.Hour

// SnapshotFleet records today's Dashboard figures for every organization, both
// for all departments and for each one, replacing today's earlier snapshot.
// "Today" is the server's local date.
func (c *Core) SnapshotFleet(ctx context.Context, now time.Time) error {
	orgs, err := c.Store.ListOrgIDs(ctx)
	if err != nil {
		return err
	}
	latest := ""
	if m, err := agentdist.Load(c.AgentDist); err == nil {
		latest = m.Version
	}
	for _, org := range orgs {
		groups, err := c.Store.ListGroups(ctx, org)
		if err != nil {
			return err
		}
		keys := []string{""}
		for _, g := range groups {
			keys = append(keys, g.ID)
		}
		for _, key := range keys {
			if err := c.snapshot(ctx, org, key, latest, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Core) snapshot(ctx context.Context, org, groupKey, latest string, now time.Time) error {
	d, err := c.Store.Dashboard(ctx, org, groupKey)
	if err != nil {
		return err
	}
	current := 0
	if latest != "" {
		for _, v := range d.AgentVersions {
			if v.Version != "" && !agentdist.Outdated(v.Version, latest) {
				current += v.Count
			}
		}
	}
	return c.Store.SaveSnapshot(ctx, org, groupKey, now, store.SnapshotOf(d, current))
}

// Trend returns the last TrendDays of snapshots for an organization or one department.
func (c *Core) Trend(ctx context.Context, org, groupKey string, now time.Time) ([]store.Snapshot, error) {
	return c.Store.ListSnapshots(ctx, org, groupKey, now.AddDate(0, 0, -(TrendDays-1)))
}

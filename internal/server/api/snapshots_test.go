package api_test

import (
	"context"
	"testing"
	"time"
)

func TestDashboardTrend(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	a := e.signup(t, "Trend")
	var tok, grp idResp
	a.do("POST", "/api/enrollment-tokens", map[string]any{"description": "t"}, 201, &tok)
	dev := e.enroll(t, tok.Token)
	e.enroll(t, tok.Token)
	a.do("POST", "/api/groups", map[string]any{"name": "Finance"}, 201, &grp)
	a.do("PATCH", "/api/devices/"+dev.DeviceID, map[string]any{"group_id": grp.ID}, 200, nil)

	type trend struct {
		Trend []struct {
			Day     string
			Devices int
		}
	}
	var none trend
	a.do("GET", "/api/dashboard", nil, 200, &none)
	if none.Trend == nil || len(none.Trend) != 0 {
		t.Fatalf("trend before any snapshot = %+v, want an empty list", none.Trend)
	}

	// Two days ago, yesterday, and today twice: today keeps one row with the latest figures.
	now := time.Now()
	for _, day := range []time.Time{now.AddDate(0, 0, -2), now.AddDate(0, 0, -1), now, now} {
		if err := e.core.SnapshotFleet(ctx, day); err != nil {
			t.Fatal(err)
		}
	}
	// Older than the 30 days shown, and older than retention.
	if err := e.core.SnapshotFleet(ctx, now.AddDate(0, 0, -45)); err != nil {
		t.Fatal(err)
	}
	if err := e.core.SnapshotFleet(ctx, now.AddDate(0, 0, -500)); err != nil {
		t.Fatal(err)
	}

	var all trend
	a.do("GET", "/api/dashboard", nil, 200, &all)
	if len(all.Trend) != 3 {
		t.Fatalf("trend has %d days, want 3 (30-day window, one row per day): %+v", len(all.Trend), all.Trend)
	}
	if all.Trend[0].Day != now.AddDate(0, 0, -2).Format(time.DateOnly) || all.Trend[2].Day != now.Format(time.DateOnly) {
		t.Fatalf("trend days = %+v, want oldest first ending today", all.Trend)
	}
	if all.Trend[2].Devices != 2 {
		t.Fatalf("today's devices = %d, want 2", all.Trend[2].Devices)
	}

	var finance trend
	a.do("GET", "/api/dashboard?group_id="+grp.ID, nil, 200, &finance)
	if len(finance.Trend) != 3 || finance.Trend[2].Devices != 1 {
		t.Fatalf("Finance trend = %+v, want 3 days with 1 device", finance.Trend)
	}

	// Retention removes snapshots older than 400 days and keeps the rest.
	if err := e.store.PurgeSnapshots(ctx, now.AddDate(0, 0, -400)); err != nil {
		t.Fatal(err)
	}
	old, err := e.store.ListSnapshots(ctx, a.orgID, "", now.AddDate(0, 0, -600))
	if err != nil {
		t.Fatal(err)
	}
	if len(old) != 4 { // -45, -2, -1, today
		t.Fatalf("after purge %d snapshots, want 4: %+v", len(old), old)
	}
}

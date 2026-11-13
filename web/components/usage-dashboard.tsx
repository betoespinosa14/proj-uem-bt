"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { UsageAppStat, UsageSummary } from "@/lib/api";
import { cn, formatHours } from "@/lib/utils";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { ChevronRight, ServerCog } from "lucide-react";

// One series per chart, in the brand color (--primary: dark teal on light,
// bright teal on dark). Idle time is a neutral remainder, never a hue, and
// every bar carries a text label, so color is never the only cue.

type InfraAction = (app: string, mark: boolean) => void;

/** "device" when the dashboard covers one device (the device page's Usage tab). */
export type UsageScope = "fleet" | "device";

/**
 * The App usage dashboard: trend, top apps and users, active vs open time, and
 * background-only processes. onInfrastructure is set when the viewer may mark
 * apps as infrastructure.
 */
export function UsageDashboard({
  data,
  days,
  scope = "fleet",
  onPickApp,
  onInfrastructure,
}: {
  data: UsageSummary;
  days: number;
  scope?: UsageScope;
  onPickApp: (app: string) => void;
  onInfrastructure?: InfraAction;
}) {
  const device = scope === "device";
  // Usage is stored per day: a one-day period has no trend, and a weekday
  // average needs several weeks.
  const showTrend = days > 1;
  const showWeekdays = device && days >= 30;
  return (
    <div className="space-y-5">
      {showTrend && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle>Active hours per day</CardTitle>
            <CardDescription>Time an app was in front while someone was typing or using the mouse, {device ? "on this device" : "across all devices"}.</CardDescription>
          </CardHeader>
          <CardContent>
            <Trend daily={data.daily} days={days} device={device} />
          </CardContent>
        </Card>
      )}

      <div className={cn("grid grid-cols-1 gap-5", (!device || showWeekdays) && "lg:grid-cols-2")}>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle>Top apps</CardTitle>
            <CardDescription>By active time. Click an app to see who used it.</CardDescription>
          </CardHeader>
          <CardContent>
            <Ranked
              rows={data.top_apps.map((a) => ({
                key: a.app,
                label: a.app,
                value: a.active_seconds,
                note: `${a.users} ${a.users === 1 ? "user" : "users"}`,
                onClick: () => onPickApp(a.app),
              }))}
            />
          </CardContent>
        </Card>
        {device ? (
          showWeekdays && (
            <Card>
              <CardHeader className="pb-2">
                <CardTitle>Active hours by weekday</CardTitle>
                <CardDescription>Average active time on each day of the week, from the first report in this period.</CardDescription>
              </CardHeader>
              <CardContent>
                <WeekdayPattern daily={data.daily} days={days} />
              </CardContent>
            </Card>
          )
        ) : (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle>Top users</CardTitle>
              <CardDescription>By active time: how long they were actually typing or using the mouse.</CardDescription>
            </CardHeader>
            <CardContent>
              <Ranked
                rows={data.top_users.map((u) => ({
                  key: u.user,
                  label: userLabel(u, data.top_users),
                  value: u.active_seconds,
                  note: device
                    ? `${u.apps} ${u.apps === 1 ? "app" : "apps"}`
                    : `${u.apps} ${u.apps === 1 ? "app" : "apps"} · ${u.devices > 2 ? `${u.devices} devices` : u.device_names.join(", ")}`,
                  hint: `Signed in as ${u.user} on ${u.device_names.join(", ")}`,
                }))}
              />
            </CardContent>
          </Card>
        )}
      </div>

      <Card>
        <CardHeader className="flex-row items-start justify-between gap-4 pb-2">
          <div className="space-y-1">
            <CardTitle>Active vs open</CardTitle>
            <CardDescription>
              Apps people used, sorted by idle hours (time open minus time in use), so apps left running the longest come first. Processes nobody ever used are
              listed separately below.
            </CardDescription>
          </div>
          <span className="flex shrink-0 items-center gap-3 pt-0.5 text-xs text-muted-foreground">
            <Swatch className="bg-primary" label="Active" />
            <Swatch className="bg-muted-foreground/35" label="Idle" />
          </span>
        </CardHeader>
        <CardContent>
          <ActiveVsOpen apps={data.idle_apps} onPickApp={onPickApp} onInfrastructure={onInfrastructure} device={device} />
          <InfrastructureFooter apps={data.infrastructure} onInfrastructure={onInfrastructure} />
        </CardContent>
      </Card>

      {data.background.length > 0 && (
        <Background
          rows={data.background}
          apps={data.totals.background_apps}
          minutes={data.totals.background_minutes}
          onInfrastructure={onInfrastructure}
          device={device}
        />
      )}
    </div>
  );
}

function Swatch({ className, label }: { className: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={`h-2 w-3 rounded-sm ${className}`} />
      {label}
    </span>
  );
}

/** Active share of open time; small shares keep a decimal so they don't read as 0%. */
function share(activeSeconds: number, openMinutes: number) {
  if (openMinutes <= 0) return "—";
  const pct = Math.min(100, (activeSeconds / (openMinutes * 60)) * 100);
  return pct > 0 && pct < 1 ? `${pct.toFixed(1)}%` : `${Math.round(pct)}%`;
}

const BACKGROUND_COLLAPSED = 5;

/** Processes that ran but nobody ever used in the period: helpers, daemons, remote-access hosts, service accounts. */
function Background({
  rows: allRows,
  apps,
  minutes,
  onInfrastructure,
  device,
}: {
  rows: UsageSummary["background"];
  apps: number;
  minutes: number;
  onInfrastructure?: InfraAction;
  device: boolean;
}) {
  const [all, setAll] = useState(false);
  // Third-party only by default: vendor software (remote access, helpers, paid
  // apps) is what's worth reviewing; OS processes like bash or systemd aren't.
  const [thirdOnly, setThirdOnly] = useState(true);
  const system = allRows.filter((b) => !b.third_party);
  const rows = thirdOnly ? allRows.filter((b) => b.third_party) : allRows;
  const shown = all ? rows : rows.slice(0, BACKGROUND_COLLAPSED);
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-4 pb-2">
        <div className="space-y-1">
          <CardTitle>Running in the background, never used</CardTitle>
          <CardDescription>
            {apps} {apps === 1 ? "app" : "apps"} ran for {formatHours(minutes * 60)} with no one using them: system processes, helpers, and apps left
            running under another account. They aren&apos;t counted as use above. Worth a look when it&apos;s remote-access software or a paid app.
          </CardDescription>
        </div>
        <label className="flex shrink-0 items-center gap-2 pt-0.5 text-xs font-medium">
          <Switch checked={thirdOnly} onCheckedChange={setThirdOnly} aria-label="Show third-party software only" />
          Third-party only
        </label>
      </CardHeader>
      <CardContent className="px-0 pb-3">
        <ul className="divide-y divide-border/60">
          {shown.map((b) => (
            <li
              key={`${b.device_id}-${b.app}-${b.user}`}
              className={`group grid ${device ? "grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_5.5rem_2rem]" : "grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)_5.5rem_2rem]"} items-center gap-3 px-5 py-2 text-sm`}
            >
              <span className="truncate font-medium" title={b.app}>
                {b.app}
              </span>
              {!device && (
                <Link href={`/devices/${b.device_id}`} className="truncate text-muted-foreground hover:underline" title={b.device_name}>
                  {b.device_name}
                </Link>
              )}
              <span className="truncate text-muted-foreground" title={b.user}>
                {b.user}
              </span>
              <span className="text-right text-xs text-muted-foreground tabular">{formatHours(b.minutes * 60)} open</span>
              <span>{onInfrastructure && <MarkInfra app={b.app} onInfrastructure={onInfrastructure} device={device} />}</span>
            </li>
          ))}
        </ul>
        {rows.length === 0 && <p className="px-5 py-4 text-sm text-muted-foreground">Only system processes ran in the background.</p>}
        {thirdOnly && system.length > 0 && (
          <p className="px-5 pt-2 text-xs text-muted-foreground">
            Hiding {system.length} system {system.length === 1 ? "process" : "processes"} ({system.slice(0, 3).map((b) => b.app).join(", ")}
            {system.length > 3 ? "…" : ""}).
          </p>
        )}
        {rows.length > BACKGROUND_COLLAPSED && (
          <button type="button" onClick={() => setAll(!all)} className="mx-5 mt-2 text-xs font-semibold text-primary hover:underline">
            {all ? "Show fewer" : `Show all ${rows.length}`}
          </button>
        )}
      </CardContent>
    </Card>
  );
}

function localDay(d: Date) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

// Plot-area insets of the daily chart (Y-axis width, right margin), shared with the strip under it.
const TREND_LEFT = 44;
const TREND_RIGHT = 8;

// Axis steps in minutes: round durations, then whole days for large fleet totals.
const AXIS_STEPS = [1, 2, 5, 10, 15, 30, 60, 120, 180, 240, 360, 480, 720, 1440, 2880, 7200, 14400, 28800, 72000];

/** Round tick values (in hours) for a duration axis whose tallest value is maxHours: at most 5 steps above 0. */
function durationAxis(maxHours: number): { ticks: number[]; top: number } {
  const maxMin = maxHours * 60;
  if (!(maxMin > 0)) return { ticks: [0, 0.25, 0.5, 0.75, 1], top: 1 };
  const step = AXIS_STEPS.find((s) => maxMin / s <= 5) ?? Math.ceil(maxMin / 5);
  const n = Math.max(1, Math.ceil(maxMin / step));
  const ticks = Array.from({ length: n + 1 }, (_, i) => (i * step) / 60);
  return { ticks, top: ticks[ticks.length - 1] ?? 1 };
}

/** "0", "5m", "30m", "1h", "1.5h", "12h". */
function axisLabel(hours: number) {
  const m = Math.round(hours * 60);
  if (m === 0) return "0";
  if (m < 60) return `${m}m`;
  const h = m / 60;
  return `${Number.isInteger(h) ? h : h.toFixed(1)}h`;
}

/**
 * Every day of the period. Days no device reported anything for (agents not
 * installed yet, devices off, tracking off) are shaded "No data" and break the
 * line, so they don't read as days nobody worked.
 */
export function Trend({
  daily,
  days,
  device,
  compact = false,
}: {
  daily: UsageSummary["daily"];
  days: number;
  device: boolean;
  /** A shorter chart, for the summary card on a device's Overview. */
  compact?: boolean;
}) {
  const byDay = new Map(daily.map((d) => [d.day, d]));
  const today = new Date();
  // One bar per day (daily totals, not a continuous signal); the last one is today, still in progress.
  const points = Array.from({ length: days }, (_, i) => {
    const d = new Date(today.getFullYear(), today.getMonth(), today.getDate() - (days - 1 - i));
    const row = byDay.get(localDay(d));
    return {
      t: d.getTime(),
      active: row ? row.active_seconds / 3600 : null,
      // Reported but nobody used anything: a stub on the baseline, so it doesn't look like a no-data day.
      stub: row && row.active_seconds === 0 ? 1 : 0,
      apps: row?.apps ?? 0,
      devices: row?.devices ?? 0,
      today: i === days - 1,
    };
  });
  const byT = new Map(points.map((p) => [p.t, p]));
  const hasGaps = points.some((p) => p.active === null);
  const hasReported = points.some((p) => p.active !== null);
  const hasIdleDays = points.some((p) => p.stub);
  // The strip's segments start and end at bar edges, not band edges; that needs the plot width in px.
  const gapPct = days > 31 ? 0.12 : 0.28;
  const MAX_BAR = 44;
  const stripRef = useRef<HTMLDivElement>(null);
  const [stripWidth, setStripWidth] = useState(0);
  useEffect(() => {
    const el = stripRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setStripWidth(e!.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, [hasReported]); // the strip mounts once any day has reported
  const band = stripWidth / days;
  const barW = Math.min(band * (1 - 2 * gapPct), MAX_BAR);
  const barLeft = (i: number) => i * band + (band - barW) / 2;
  const fmt = (t: number) => new Date(t).toLocaleDateString(undefined, { month: "short", day: "numeric" });
  const axis = durationAxis(Math.max(0, ...points.map((p) => p.active ?? 0)));
  // The stub is a sliver (1.5% of the axis), stacked under the (zero) active bar.
  const data = points.map((p) => ({ ...p, stubV: p.stub ? axis.top * 0.015 : 0 }));
  // Runs of reported / not reported days for the strip under the chart.
  const runs: { from: number; to: number; on: boolean }[] = [];
  points.forEach((p, i) => {
    const on = p.active !== null;
    const last = runs[runs.length - 1];
    if (last && last.on === on) last.to = i;
    else runs.push({ from: i, to: i, on });
  });
  // Each run starts at its first bar's left edge; the one before it runs up to a 4px seam
  // short of that, so the strip is continuous. The outer ends sit at the first and last bars.
  const SEAM = 4;
  const runBox = (k: number) => {
    const r = runs[k]!;
    const left = barLeft(r.from);
    const next = runs[k + 1];
    const right = next ? barLeft(next.from) - SEAM : barLeft(r.to) + barW;
    return { left, width: Math.max(0, right - left) };
  };
  return (
    <div>
      <div className={compact ? "h-40" : "h-56"}>
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={data} margin={{ left: 0, right: TREND_RIGHT, top: 8 }} barCategoryGap={`${gapPct * 100}%`}>
            <defs>
              <pattern id="usage-today" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
                <rect width="6" height="6" fill="hsl(var(--primary))" fillOpacity={0.25} />
                <line x1="0" y1="0" x2="0" y2="6" stroke="hsl(var(--primary))" strokeOpacity={0.5} strokeWidth="2" />
              </pattern>
            </defs>
            <CartesianGrid stroke="hsl(var(--border))" strokeOpacity={0.6} vertical={false} />
            <XAxis dataKey="t" tickFormatter={fmt} fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} interval="preserveStartEnd" minTickGap={24} />
            <YAxis fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} ticks={axis.ticks} domain={[0, axis.top]} tickFormatter={axisLabel} width={TREND_LEFT} />
            <Tooltip
              cursor={{ fill: "hsl(var(--muted))", fillOpacity: 0.5 }}
              content={({ active, label }) => {
                const p = active ? byT.get(Number(label)) : undefined;
                if (!p) return null;
                return (
                  <div className="rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lift">
                    <p className="mb-1 font-medium">
                      {new Date(p.t).toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" })}
                      {p.today && <span className="font-normal text-muted-foreground"> · Today, so far</span>}
                    </p>
                    {p.active === null ? (
                      <p className="text-muted-foreground">No data reported</p>
                    ) : p.stub ? (
                      <>
                        <p className="font-medium">No active use</p>
                        <p className="text-muted-foreground">
                          {device ? "Apps were open, but no one used them." : `${p.devices} ${p.devices === 1 ? "device" : "devices"} reported, but no one used them.`}
                        </p>
                      </>
                    ) : (
                      <>
                        <p>
                          <span className="text-muted-foreground">Active</span> <span className="font-semibold tabular">{formatHours(p.active * 3600)}</span>
                        </p>
                        <p className="text-muted-foreground">
                          {device ? `${p.apps} ${p.apps === 1 ? "app" : "apps"} used` : `${p.devices} ${p.devices === 1 ? "device" : "devices"} reported`}
                        </p>
                      </>
                    )}
                  </div>
                );
              }}
            />
            <Bar dataKey="stubV" stackId="day" fill="hsl(var(--muted-foreground))" fillOpacity={0.4} isAnimationActive={false} maxBarSize={MAX_BAR} minPointSize={0} legendType="none" />
            <Bar dataKey="active" stackId="day" radius={[3, 3, 0, 0]} isAnimationActive={false} maxBarSize={MAX_BAR}>
              {points.map((p) => (
                <Cell key={p.t} fill={p.today ? "url(#usage-today)" : "hsl(var(--primary))"} fillOpacity={p.today ? 1 : 0.85} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
      {hasReported && (
        <>
          {/* Same idea as Telemetry's strip: when the device (or fleet) reported. Each run starts at its
              first bar's left edge and ends at its last bar's right edge, so it sits right under the bars. */}
          <div ref={stripRef} className="relative mt-1 h-2" style={{ marginLeft: TREND_LEFT, marginRight: TREND_RIGHT }} role="img" aria-label="Days with usage data">
            <div className="absolute inset-0">
              {runs.map((r, k) => (
                <div
                  key={r.from}
                  className={cn("absolute inset-y-0 rounded-full", r.on ? "bg-success/60" : "bg-muted-foreground/20")}
                  style={
                    stripWidth
                      ? runBox(k)
                      : { left: `${(r.from / days) * 100}%`, width: `${((r.to - r.from + 1) / days) * 100}%` }
                  }
                  title={`${r.on ? "Reported" : "No data"}: ${fmt(points[r.from]!.t)}${r.to > r.from ? ` – ${fmt(points[r.to]!.t)}` : ""}`}
                />
              ))}
            </div>
          </div>
          <p className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground" style={{ marginLeft: TREND_LEFT }}>
            <span className="inline-flex items-center gap-1.5">
              <span className="h-2 w-3 rounded-full bg-success/60" /> Reported
            </span>
            {hasGaps && (
              <>
                <span className="inline-flex items-center gap-1.5">
                  <span className="h-2 w-3 rounded-full bg-muted-foreground/20" />
                  No data ({device ? "agent not installed yet, device off, or tracking off" : "agent not installed yet, devices off, or tracking off"})
                </span>
              </>
            )}
            {hasIdleDays && (
              <span className="inline-flex items-center gap-1.5">
                <span className="h-0.5 w-3 bg-muted-foreground/40" /> Reported, no one used it
              </span>
            )}
          </p>
        </>
      )}
    </div>
  );
}

const WEEKDAYS = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"];

/**
 * Average active hours per weekday, Mon to Sun. Each weekday's total is divided
 * by how many times it occurred from the first report in the period to today:
 * days before the first report (agent not installed yet) are left out, and days
 * after it with no report count as 0 (the device was off).
 */
function WeekdayPattern({ daily, days }: { daily: UsageSummary["daily"]; days: number }) {
  const byDay = new Map(daily.map((d) => [d.day, d.active_seconds]));
  const today = new Date();
  const totals = Array(7).fill(0) as number[];
  const counts = Array(7).fill(0) as number[];
  let started = false;
  let span = 0;
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(today.getFullYear(), today.getMonth(), today.getDate() - i);
    const secs = byDay.get(localDay(d));
    if (!started && secs === undefined) continue;
    started = true;
    span++;
    const w = (d.getDay() + 6) % 7; // Monday = 0
    totals[w] = (totals[w] ?? 0) + (secs ?? 0);
    counts[w] = (counts[w] ?? 0) + 1;
  }
  if (span < 7) return <p className="py-6 text-center text-sm text-muted-foreground">Not enough days yet. Check back after a week of use.</p>;
  const bars = WEEKDAYS.map((name, i) => {
    const n = counts[i] ?? 0;
    return { name, short: name.slice(0, 3), hours: n ? (totals[i] ?? 0) / n / 3600 : 0, count: n };
  });
  const busiest = bars.reduce((a, b) => (b.hours > a.hours ? b : a));
  const axis = durationAxis(busiest.hours);
  return (
    <div>
      <div className="h-48">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={bars} margin={{ left: -12, right: 8, top: 8 }}>
            <CartesianGrid stroke="hsl(var(--border))" strokeOpacity={0.6} vertical={false} />
            <XAxis dataKey="short" fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} />
            <YAxis fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} ticks={axis.ticks} domain={[0, axis.top]} tickFormatter={axisLabel} width={44} />
            <Tooltip
              cursor={{ fill: "hsl(var(--muted))", fillOpacity: 0.5 }}
              content={({ active, payload }) => {
                const b = active ? (payload?.[0]?.payload as (typeof bars)[number] | undefined) : undefined;
                if (!b) return null;
                return (
                  <div className="rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lift">
                    <p className="mb-1 font-medium">{b.name}</p>
                    <p>
                      <span className="font-semibold tabular">{formatHours(b.hours * 3600)}</span> <span className="text-muted-foreground">average</span>
                    </p>
                    <p className="text-muted-foreground">
                      over {b.count} {b.name}
                      {b.count === 1 ? "" : "s"}
                    </p>
                  </div>
                );
              }}
            />
            <Bar dataKey="hours" fill="hsl(var(--primary))" radius={[4, 4, 0, 0]} maxBarSize={36} isAnimationActive={false} />
          </BarChart>
        </ResponsiveContainer>
      </div>
      {busiest.hours > 0 && (
        <p className="mt-2 text-xs text-muted-foreground">
          Busiest: <span className="font-medium text-foreground">{busiest.name}</span> · {formatHours(busiest.hours * 3600)} average
        </p>
      )}
    </div>
  );
}

/**
 * A top user's display name: the employee name of their main device, with the
 * account's own name in brackets when two accounts map to the same employee.
 */
function userLabel(u: UsageSummary["top_users"][number], all: UsageSummary["top_users"]) {
  if (!u.employee) return u.user;
  const shared = all.filter((x) => x.employee === u.employee).length > 1;
  return shared ? `${u.employee} (${u.user.split("\\").pop()})` : u.employee;
}

type RankedRow = { key: string; label: string; value: number; note: string; hint?: string; onClick?: () => void };

/** Horizontal bars scaled to the top row, with the value as text. */
export function Ranked({ rows }: { rows: RankedRow[] }) {
  if (!rows.length) return <p className="py-6 text-center text-sm text-muted-foreground">No use in this period</p>;
  const max = Math.max(...rows.map((r) => r.value), 1);
  return (
    <ul className="space-y-0.5">
      {rows.map((r) => {
        const body = (
          <>
            <span className="truncate font-medium" title={r.label}>
              {r.label}
            </span>
            <span className="flex h-2 overflow-hidden rounded-full bg-muted">
              <span className="h-full rounded-full bg-primary" style={{ width: `${Math.max((r.value / max) * 100, 1.5)}%` }} />
            </span>
            <span className="text-right text-xs text-muted-foreground tabular" title={r.hint}>
              <span className="font-semibold text-foreground">{formatHours(r.value)}</span> · {r.note}
            </span>
          </>
        );
        const cls = "grid items-center gap-3 rounded-lg px-2 py-1.5 text-sm";
        return (
          <li key={r.key}>
            {r.onClick ? (
              <button
                type="button"
                onClick={r.onClick}
                className={`${cls} group/row w-full grid-cols-[minmax(0,9rem)_1fr_10.5rem_1rem] text-left transition-colors hover:bg-muted/60`}
              >
                {body}
                <RowChevron />
              </button>
            ) : (
              <div className={`${cls} grid-cols-[minmax(0,9rem)_1fr_10.5rem]`}>{body}</div>
            )}
          </li>
        );
      })}
    </ul>
  );
}

/** Each app's open time as a bar, split into the part it was actively used and the idle rest. */
function ActiveVsOpen({
  apps,
  onPickApp,
  onInfrastructure,
  device,
}: {
  apps: UsageAppStat[];
  onPickApp: (app: string) => void;
  onInfrastructure?: InfraAction;
  device: boolean;
}) {
  const rows = apps.filter((a) => a.minutes > 0);
  if (!rows.length) return <p className="py-6 text-center text-sm text-muted-foreground">No open-time data in this period</p>;
  const maxOpen = Math.max(...rows.map((a) => a.minutes * 60));
  return (
    <ul className="space-y-0.5">
      {rows.map((a) => {
        const open = a.minutes * 60;
        const active = Math.min(a.active_seconds, open); // active-only samples can exceed sampled open time
        const pct = (active / open) * 100;
        const width = (open / maxOpen) * 100;
        return (
          <li key={a.app} className="group flex items-center gap-1">
            <button
              type="button"
              onClick={() => onPickApp(a.app)}
              title={`${a.app}: ${formatHours(open - active)} idle — ${formatHours(active)} active of ${formatHours(open)} open`}
              className="group/row grid min-w-0 flex-1 grid-cols-[minmax(0,9rem)_1fr_12rem_1rem] items-center gap-3 rounded-lg px-2 py-1.5 text-left text-sm transition-colors hover:bg-muted/60"
            >
              <span className="truncate font-medium">{a.app}</span>
              <span className="flex h-2.5" style={{ width: `${Math.max(width, 2)}%` }}>
                {active > 0 && <span className={`h-full rounded-l-full bg-primary ${pct >= 100 ? "rounded-r-full" : ""}`} style={{ width: `${pct}%` }} />}
                {pct < 100 && (
                  <span
                    className={`h-full rounded-r-full bg-muted-foreground/35 ${active > 0 ? "ml-[2px]" : "rounded-l-full"}`}
                    style={{ width: `${100 - pct}%` }}
                  />
                )}
              </span>
              <span className="text-right text-xs text-muted-foreground tabular">
                <span className="font-semibold text-foreground">{formatHours(open - active)} idle</span> · {share(active, a.minutes)} active
              </span>
              <RowChevron />
            </button>
            {onInfrastructure && <MarkInfra app={a.app} onInfrastructure={onInfrastructure} device={device} />}
          </li>
        );
      })}
    </ul>
  );
}

/** A faint "›" on rows that open the detailed table; stronger on hover. */
function RowChevron() {
  return (
    <ChevronRight
      className="h-4 w-4 justify-self-end text-muted-foreground opacity-40 transition-opacity group-hover/row:opacity-100 group-focus-visible/row:opacity-100"
      aria-hidden
    />
  );
}

/** Icon button, shown on row hover or keyboard focus. */
function MarkInfra({ app, onInfrastructure, device }: { app: string; onInfrastructure: InfraAction; device: boolean }) {
  return (
    <button
      type="button"
      onClick={() => onInfrastructure(app, true)}
      title={`Mark ${app} as infrastructure: open by design (e.g. remote access), so its open time isn't counted as idle${device ? " (applies to all devices)" : ""}`}
      aria-label={`Mark ${app} as infrastructure`}
      className="shrink-0 rounded-md p-1.5 text-muted-foreground opacity-0 transition hover:bg-muted hover:text-foreground focus-visible:opacity-100 group-hover:opacity-100"
    >
      <ServerCog className="h-4 w-4" />
    </button>
  );
}

/** Apps marked as infrastructure, each with a way back into the ranking. */
function InfrastructureFooter({ apps, onInfrastructure }: { apps: UsageAppStat[]; onInfrastructure?: InfraAction }) {
  if (!apps.length) return null;
  return (
    <p className="mt-3 flex flex-wrap items-center gap-x-1.5 gap-y-1 border-t border-border/60 pt-3 text-xs text-muted-foreground">
      <ServerCog className="h-3.5 w-3.5" />
      <span>Ignored as infrastructure:</span>
      {apps.map((a, i) => (
        <span key={a.app}>
          <span className="font-medium text-foreground">{a.app}</span> ({formatHours(a.minutes * 60)} open)
          {onInfrastructure && (
            <>
              {" "}
              <button type="button" onClick={() => onInfrastructure(a.app, false)} className="font-semibold text-primary hover:underline">
                Restore
              </button>
            </>
          )}
          {i < apps.length - 1 ? " ·" : ""}
        </span>
      ))}
    </p>
  );
}

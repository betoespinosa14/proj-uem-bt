"use client";

import { useState } from "react";
import Link from "next/link";
import { Area, AreaChart, CartesianGrid, ReferenceArea, ReferenceLine, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import type { UsageAppStat, UsageSummary } from "@/lib/api";
import { formatHours } from "@/lib/utils";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { ServerCog } from "lucide-react";

// One series per chart, in the brand color (--primary: dark teal on light,
// bright teal on dark). Idle time is a neutral remainder, never a hue, and
// every bar carries a text label, so color is never the only cue.

type InfraAction = (app: string, mark: boolean) => void;

/**
 * The App usage dashboard: trend, top apps and users, active vs open time, and
 * background-only processes. onInfrastructure is set when the viewer may mark
 * apps as infrastructure.
 */
export function UsageDashboard({
  data,
  days,
  onPickApp,
  onInfrastructure,
}: {
  data: UsageSummary;
  days: number;
  onPickApp: (app: string) => void;
  onInfrastructure?: InfraAction;
}) {
  return (
    <div className="space-y-5">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle>Active hours per day</CardTitle>
          <CardDescription>Time an app was in front while someone was typing or using the mouse, across all devices.</CardDescription>
        </CardHeader>
        <CardContent>
          <Trend daily={data.daily} days={days} />
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
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
        <Card>
          <CardHeader className="pb-2">
            <CardTitle>Top users</CardTitle>
            <CardDescription>By active time: how long they were actually typing or using the mouse.</CardDescription>
          </CardHeader>
          <CardContent>
            <Ranked
              rows={data.top_users.map((u) => ({
                key: u.user,
                label: u.user,
                value: u.active_seconds,
                note: `${u.apps} ${u.apps === 1 ? "app" : "apps"} · ${u.devices > 2 ? `${u.devices} devices` : u.device_names.join(", ")}`,
                hint: `On ${u.device_names.join(", ")}`,
              }))}
            />
          </CardContent>
        </Card>
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
          <ActiveVsOpen apps={data.idle_apps} onPickApp={onPickApp} onInfrastructure={onInfrastructure} />
          <InfrastructureFooter apps={data.infrastructure} onInfrastructure={onInfrastructure} />
        </CardContent>
      </Card>

      {data.background.length > 0 && (
        <Background rows={data.background} apps={data.totals.background_apps} minutes={data.totals.background_minutes} onInfrastructure={onInfrastructure} />
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
}: {
  rows: UsageSummary["background"];
  apps: number;
  minutes: number;
  onInfrastructure?: InfraAction;
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
              className="group grid grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1fr)_5.5rem_2rem] items-center gap-3 px-5 py-2 text-sm"
            >
              <span className="truncate font-medium" title={b.app}>
                {b.app}
              </span>
              <Link href={`/devices/${b.device_id}`} className="truncate text-muted-foreground hover:underline" title={b.device_name}>
                {b.device_name}
              </Link>
              <span className="truncate text-muted-foreground" title={b.user}>
                {b.user}
              </span>
              <span className="text-right text-xs text-muted-foreground tabular">{formatHours(b.minutes * 60)} open</span>
              <span>{onInfrastructure && <MarkInfra app={b.app} onInfrastructure={onInfrastructure} />}</span>
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

const DAY = 86_400_000;

/**
 * Every day of the period. Days no device reported anything for (agents not
 * installed yet, devices off, tracking off) are shaded "No data" and break the
 * line, so they don't read as days nobody worked.
 */
function Trend({ daily, days }: { daily: UsageSummary["daily"]; days: number }) {
  const byDay = new Map(daily.map((d) => [d.day, d]));
  const today = new Date();
  const points = Array.from({ length: days }, (_, i) => {
    const d = new Date(today.getFullYear(), today.getMonth(), today.getDate() - (days - 1 - i));
    const row = byDay.get(localDay(d));
    return { t: d.getTime(), active: row ? row.active_seconds / 3600 : null, open: row ? row.minutes / 60 : null, devices: row?.devices ?? 0 };
  });
  const byT = new Map(points.map((p) => [p.t, p]));
  // Runs of consecutive no-data days, as shaded bands.
  const gaps: { from: number; to: number; len: number }[] = [];
  for (const p of points) {
    const last = gaps[gaps.length - 1];
    if (p.active !== null) continue;
    if (last && Math.round((p.t - last.to) / DAY) === 1) {
      last.to = p.t;
      last.len++;
    } else gaps.push({ from: p.t, to: p.t, len: 1 });
  }
  const first = points[0]?.t ?? 0;
  // Mark where reporting began when the period starts with no data.
  const firstReport = points[0]?.active === null ? points.find((p) => p.active !== null)?.t : undefined;
  const lastT = points[points.length - 1]?.t ?? 0;
  const fmt = (t: number) => new Date(t).toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return (
    <div>
      <div className="h-56">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={points} margin={{ left: -12, right: 8, top: 8 }}>
            <defs>
              <linearGradient id="usage-fill" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="hsl(var(--primary))" stopOpacity={0.22} />
                <stop offset="100%" stopColor="hsl(var(--primary))" stopOpacity={0.02} />
              </linearGradient>
            </defs>
            <CartesianGrid stroke="hsl(var(--border))" strokeOpacity={0.6} vertical={false} />
            {gaps.map((g) => (
              <ReferenceArea
                key={g.from}
                x1={Math.max(first, g.from - DAY / 2)}
                x2={Math.min(lastT, g.to + DAY / 2)}
                fill="hsl(var(--muted-foreground))"
                fillOpacity={0.08}
                strokeOpacity={0}
                ifOverflow="hidden"
                label={g.len >= 4 ? { value: "No data", position: "insideTop", fontSize: 11, fill: "hsl(var(--muted-foreground))" } : undefined}
              />
            ))}
            {firstReport !== undefined && (
              <ReferenceLine
                x={firstReport}
                stroke="hsl(var(--muted-foreground))"
                strokeDasharray="3 3"
                label={{ value: "First report", position: "insideTopLeft", fontSize: 11, fill: "hsl(var(--muted-foreground))" }}
              />
            )}
            <XAxis dataKey="t" type="number" scale="time" domain={["dataMin", "dataMax"]} tickFormatter={fmt} fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} minTickGap={40} />
            <YAxis fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} tickFormatter={(v: number) => `${v}h`} allowDecimals width={44} />
            <Tooltip
              cursor={{ stroke: "hsl(var(--muted-foreground))", strokeDasharray: "3 3" }}
              content={({ active, label }) => {
                const p = active ? byT.get(Number(label)) : undefined;
                if (!p) return null;
                return (
                  <div className="rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lift">
                    <p className="mb-1 font-medium">{new Date(p.t).toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" })}</p>
                    {p.active === null ? (
                      <p className="text-muted-foreground">No data reported</p>
                    ) : (
                      <>
                        <p>
                          <span className="text-muted-foreground">Active</span> <span className="font-semibold tabular">{formatHours(p.active * 3600)}</span>
                        </p>
                        <p>
                          <span className="text-muted-foreground">Open</span> <span className="tabular">{formatHours((p.open ?? 0) * 3600)}</span>
                        </p>
                        <p className="text-muted-foreground">
                          {p.devices} {p.devices === 1 ? "device" : "devices"} reported
                        </p>
                      </>
                    )}
                  </div>
                );
              }}
            />
            <Area type="monotone" dataKey="active" stroke="hsl(var(--primary))" strokeWidth={2} fill="url(#usage-fill)" isAnimationActive={false} connectNulls={false} activeDot={{ r: 4, strokeWidth: 2, stroke: "hsl(var(--card))" }} />
          </AreaChart>
        </ResponsiveContainer>
      </div>
      {gaps.length > 0 && (
        <p className="mt-2 flex items-center gap-2 text-xs text-muted-foreground">
          <span className="h-2.5 w-4 rounded-sm bg-muted-foreground/15" />
          Shaded: no device reported usage (agent not installed yet, devices off, or tracking off).
        </p>
      )}
    </div>
  );
}

type RankedRow = { key: string; label: string; value: number; note: string; hint?: string; onClick?: () => void };

/** Horizontal bars scaled to the top row, with the value as text. */
function Ranked({ rows }: { rows: RankedRow[] }) {
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
        const cls = "grid grid-cols-[minmax(0,9rem)_1fr_10.5rem] items-center gap-3 rounded-lg px-2 py-1.5 text-sm";
        return (
          <li key={r.key}>
            {r.onClick ? (
              <button type="button" onClick={r.onClick} className={`${cls} w-full text-left transition-colors hover:bg-muted/60`}>
                {body}
              </button>
            ) : (
              <div className={cls}>{body}</div>
            )}
          </li>
        );
      })}
    </ul>
  );
}

/** Each app's open time as a bar, split into the part it was actively used and the idle rest. */
function ActiveVsOpen({ apps, onPickApp, onInfrastructure }: { apps: UsageAppStat[]; onPickApp: (app: string) => void; onInfrastructure?: InfraAction }) {
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
              className="grid min-w-0 flex-1 grid-cols-[minmax(0,9rem)_1fr_12rem] items-center gap-3 rounded-lg px-2 py-1.5 text-left text-sm transition-colors hover:bg-muted/60"
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
            </button>
            {onInfrastructure && <MarkInfra app={a.app} onInfrastructure={onInfrastructure} />}
          </li>
        );
      })}
    </ul>
  );
}

/** Icon button, shown on row hover or keyboard focus. */
function MarkInfra({ app, onInfrastructure }: { app: string; onInfrastructure: InfraAction }) {
  return (
    <button
      type="button"
      onClick={() => onInfrastructure(app, true)}
      title={`Mark ${app} as infrastructure: open by design (e.g. remote access), so its open time isn't counted as idle`}
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

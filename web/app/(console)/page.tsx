"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpCircle,
  Bell,
  BrickWall,
  Bug,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Cpu,
  Download,
  HardDrive,
  Lock,
  MemoryStick,
  Monitor,
  RefreshCw,
  ShieldCheck,
  Terminal,
  Wifi,
  WifiOff,
  type LucideIcon,
} from "lucide-react";
import { agentOutdated, api, qk, type Command, type Dashboard, type FleetSnapshot } from "@/lib/api";
import { useCan, useMe } from "@/lib/session";
import { cn, formatDuration, osLabel, relativeTime, titleCase } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Segmented } from "@/components/ui/tabs";
import { commandLabel } from "@/components/commands";
import { CommandStatusBadge, DeviceSearch, EmptyState, ErrorState, OsIcon, PageHeader, SeverityBadge } from "@/components/common";
import { toast } from "@/components/toast";
import { Cell, Pie, PieChart, ResponsiveContainer } from "recharts";

function greeting(d = new Date()) {
  const h = d.getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
}

type Tone = "default" | "brand" | "success" | "warning" | "danger";

export default function DashboardPage() {
  const { data: me } = useMe();
  const [groupId, setGroupId] = useState("");
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: [...qk.dashboard, groupId || "all"],
    queryFn: () => api.dashboard(groupId),
    placeholderData: (p) => p,
  });
  const firstName = (me?.user.name || "").split(" ")[0];
  // Links into other pages keep the chosen department.
  const inGroup = (path: string) => (groupId ? `${path}${path.includes("?") ? "&" : "?"}group=${encodeURIComponent(groupId)}` : path);

  if (error) return <ErrorState error={error} onRetry={refetch} />;

  return (
    <>
      <PageHeader
        title={`${greeting()}${firstName ? `, ${firstName}` : ""}`}
        description="Fleet health at a glance. Updates live as devices check in."
        actions={
          <>
            <Select
              aria-label="Department"
              className="w-44"
              value={groupId}
              onValueChange={setGroupId}
              placeholder="All departments"
              options={(groups.data?.items ?? []).map((g) => ({ value: g.id, label: g.name }))}
            />
            <DeviceSearch />
          </>
        }
      />

      {data ? <StatTiles data={data} inGroup={inGroup} /> : isLoading && <TilesSkeleton />}

      {data && data.devices_total === 0 && (
        <Card className="mb-5">
          <EmptyState
            icon={Monitor}
            title={groupId ? "No devices in this department" : "No devices enrolled yet"}
            description={groupId ? "Assign devices to it from the Devices page." : "Create an enrollment token and run the one-line installer on a Windows, macOS or Linux device."}
            action={
              !groupId && (
                <Link href="/enrollment" className="text-sm font-medium text-primary hover:underline">
                  Go to Enrollment →
                </Link>
              )
            }
          />
        </Card>
      )}

      <div className="grid grid-cols-1 gap-5 xl:grid-cols-5">
        <NeedsAttention data={data} className="xl:col-span-3" inGroup={inGroup} />
        <DeviceHealth data={data} className="xl:col-span-2" />
      </div>

      <SecurityPosture data={data} inGroup={inGroup} />

      <div className="mt-5 grid grid-cols-1 gap-5 xl:grid-cols-5">
        <OsMix data={data} className="xl:col-span-2" inGroup={inGroup} />
        <RecentActivity data={data} className="xl:col-span-3" />
      </div>
    </>
  );
}

// ---------------------------------------------------------------------------
// Headline tiles
// ---------------------------------------------------------------------------

const tileTone: Record<Tone, { tile: string; value: string; icon: string }> = {
  default: { tile: "", value: "text-foreground", icon: "text-muted-foreground" },
  brand: { tile: "", value: "text-primary", icon: "text-primary" },
  success: { tile: "", value: "text-success dark:text-neon", icon: "text-success dark:text-neon" },
  // Problem tiles share one treatment: a clear border and a soft tint of their colour.
  warning: { tile: "border-warning/60 bg-warning/10", value: "text-warning", icon: "text-warning" },
  danger: { tile: "border-destructive/60 bg-destructive/10", value: "text-destructive", icon: "text-destructive" },
};

function StatTiles({ data, inGroup }: { data: Dashboard; inGroup: (p: string) => string }) {
  const pct = data.devices_total > 0 ? Math.round((data.compliant / data.devices_total) * 100) : null;
  const tiles: {
    label: string;
    value: React.ReactNode;
    sub?: string;
    icon: LucideIcon;
    tone: Tone;
    href: string;
    trend: TrendSpec;
  }[] = [
    {
      label: "Devices",
      value: data.devices_total,
      icon: Monitor,
      tone: "brand",
      href: inGroup("/devices"),
      trend: { pick: (n) => n.devices, better: "none" },
    },
    {
      label: "Online",
      value: data.online,
      icon: Wifi,
      tone: "success",
      href: inGroup("/devices?status=online"),
      trend: { pick: (n) => n.online, better: "up" },
    },
    {
      label: "Offline",
      value: data.offline,
      icon: WifiOff,
      tone: data.offline > 0 ? "warning" : "default",
      href: inGroup("/devices?status=offline"),
      trend: { pick: (n) => n.offline, better: "down" },
    },
    {
      label: "Compliant",
      value: pct == null ? "—" : `${pct}%`,
      sub: data.non_compliant > 0 ? `${data.non_compliant} non-compliant` : "All passing",
      icon: CheckCircle2,
      tone: data.non_compliant > 0 ? "danger" : "success",
      href: inGroup("/devices?compliance=non_compliant"),
      trend: { pick: (n) => (n.devices ? Math.round((n.compliant / n.devices) * 100) : 0), better: "up", unit: "pts", suffix: "%" },
    },
    {
      label: "Open alerts",
      value: data.open_alerts,
      icon: Bell,
      tone: data.open_alerts > 0 ? "danger" : "default",
      href: "/alerts",
      trend: { pick: (n) => n.open_alerts, better: "down" },
    },
  ];
  return (
    <div className="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      {tiles.map((t) => {
        const tone = tileTone[t.tone];
        return (
          <Link
            key={t.label}
            href={t.href}
            className={cn(
              "group flex flex-col gap-1 rounded-xl border border-border bg-card p-4 shadow-sm transition-colors hover:border-primary/40",
              tone.tile,
            )}
          >
            <span className="flex items-center justify-between text-xs font-medium text-muted-foreground">
              {t.label}
              <t.icon className={cn("h-4 w-4", tone.icon)} />
            </span>
            <span className={cn("text-3xl font-semibold tracking-tight tabular", tone.value)}>{t.value}</span>
            {t.sub && <span className={cn("text-xs", t.tone === "danger" ? "font-medium text-destructive" : "text-muted-foreground")}>{t.sub}</span>}
            <Trend history={data.trend} spec={t.trend} className="mt-auto pt-2" />
          </Link>
        );
      })}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Trends: a 30-day sparkline and the change against a week ago
// ---------------------------------------------------------------------------

type TrendSpec = {
  pick: (n: FleetSnapshot) => number;
  /** Which direction is good news; "none" shows the change in grey. */
  better: "up" | "down" | "none";
  /** Unit of the change, e.g. "pts" for a percentage. */
  unit?: string;
  /** Appended to values in the hover titles, e.g. "%". */
  suffix?: string;
};

/** A local date as YYYY-MM-DD (toISOString would shift it to UTC, a day early in Manila). */
const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

const shortDay = (day: string) => new Date(`${day}T00:00:00`).toLocaleDateString(undefined, { day: "numeric", month: "short" });

function Trend({ history, spec, className }: { history: FleetSnapshot[]; spec: TrendSpec; className?: string }) {
  if (history.length < 2) {
    return (
      <span className={cn("text-xs text-muted-foreground", className)}>
        Trend builds daily from {shortDay(history[0]?.day ?? ymd(new Date()))}
      </span>
    );
  }
  const values = history.map(spec.pick);
  const last = values[values.length - 1]!;
  // Compare with 7 days ago, or the oldest day we have when the history is shorter.
  const weekAgo = new Date(`${history[history.length - 1]!.day}T00:00:00`);
  weekAgo.setDate(weekAgo.getDate() - 7);
  const target = ymd(weekAgo);
  const baseIndex = Math.max(0, history.findIndex((n) => n.day >= target));
  const base = values[baseIndex]!;
  const diff = last - base;
  const since = history[baseIndex]!.day === target ? "last week" : shortDay(history[baseIndex]!.day);
  const good = spec.better === "none" || diff === 0 ? null : (diff > 0) === (spec.better === "up");
  return (
    <span className={cn("flex items-center gap-2", className)}>
      <Sparkline values={values} days={history.map((n) => n.day)} suffix={spec.suffix ?? ""} />
      <span
        className={cn(
          "whitespace-nowrap text-xs tabular",
          good === null ? "text-muted-foreground" : good ? "font-medium text-success dark:text-neon" : "font-medium text-destructive",
        )}
      >
        {diff === 0 ? "– same" : `${diff > 0 ? "▲" : "▼"} ${Math.abs(diff)}${spec.unit ? ` ${spec.unit}` : ""}`}
        <span className="font-normal text-muted-foreground"> vs {since}</span>
      </span>
    </span>
  );
}

/** A small line of daily values; hover a point for its day and value. */
function Sparkline({ values, days, suffix }: { values: number[]; days: string[]; suffix: string }) {
  const w = 96;
  const h = 24;
  const pad = 3;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const x = (i: number) => pad + (i * (w - 2 * pad)) / Math.max(1, values.length - 1);
  const y = (v: number) => (max === min ? h / 2 : h - pad - ((v - min) * (h - 2 * pad)) / (max - min));
  const path = values.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(" ");
  const lastI = values.length - 1;
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} className="shrink-0 overflow-visible" aria-hidden>
      <path d={path} fill="none" stroke="hsl(var(--muted-foreground))" strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
      {values.map((v, i) => (
        <circle key={days[i]} cx={x(i)} cy={y(v)} r={i === lastI ? 2.5 : 6} fill={i === lastI ? "hsl(var(--foreground))" : "transparent"}>
          <title>{`${shortDay(days[i]!)}: ${v}${suffix}`}</title>
        </circle>
      ))}
    </svg>
  );
}

function TilesSkeleton() {
  return (
    <div className="mb-5 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
      {[0, 1, 2, 3, 4].map((i) => (
        <Skeleton key={i} className="h-24 rounded-xl" />
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Needs attention: failing rules and open alerts in one place
// ---------------------------------------------------------------------------

type AlertGroup = {
  key: string;
  name: string;
  severity?: Dashboard["failing_rules"][number]["severity"];
  devices?: number;
  alerts: Dashboard["recent_alerts"];
};

/** Open alerts grouped under their compliance rule (in failing-rule order); the rest under "Other alerts". */
function alertGroups(data: Dashboard): AlertGroup[] {
  const groups = new Map<string, AlertGroup>();
  for (const r of data.failing_rules) {
    groups.set(r.rule_id, { key: r.rule_id, name: r.name, severity: r.severity, devices: r.devices, alerts: [] });
  }
  for (const a of data.recent_alerts) {
    const key = a.rule_id ?? "other";
    let g = groups.get(key);
    if (!g) {
      g = key === "other" ? { key, name: "Other alerts", alerts: [] } : { key, name: a.title, severity: a.severity, alerts: [] };
      groups.set(key, g);
    }
    g.alerts.push(a);
  }
  const list = [...groups.values()];
  // "Other alerts" last.
  return [...list.filter((g) => g.key !== "other"), ...list.filter((g) => g.key === "other")];
}

/** The quoted names in a finding such as `Games: found "Candy Crush Saga", "Riot Vanguard"`, without repeats. */
function findings(message: string): { kind: string; items: string[] } | null {
  const items = [...new Set([...message.matchAll(/"([^"]+)"/g)].map((m) => m[1]!))];
  if (!items.length) return null;
  const kind = message.split(":")[0]!.trim();
  return { kind: kind.length < 30 && kind !== message ? kind : "", items };
}

/** App badges shown per alert row before a "+N more" button. */
const BADGE_LIMIT = 4;

function NeedsAttention({ data, className, inGroup }: { data?: Dashboard; className?: string; inGroup: (p: string) => string }) {
  const qc = useQueryClient();
  const can = useCan();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const toggleExpanded = (id: string) =>
    setExpanded((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: qk.dashboard });
    void qc.invalidateQueries({ queryKey: qk.alerts() });
  };
  const resolve = useMutation({
    mutationFn: (id: string) => api.alerts.resolve(id),
    onSuccess: () => {
      toast.success("Alert resolved");
      refresh();
    },
    onError: (e) => toast.error("Couldn't resolve the alert", e),
  });
  const recheck = useMutation({
    mutationFn: api.compliance.evaluate,
    onSuccess: (r) => {
      toast.success("Compliance re-checked", `${r.evaluated} device${r.evaluated === 1 ? "" : "s"} evaluated.`);
      refresh();
    },
    onError: (e) => toast.error("Couldn't re-check compliance", e),
  });

  return (
    <Card className={className}>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2 pb-3">
        <CardTitle>Needs attention</CardTitle>
        <div className="flex items-center gap-2">
          {can("compliance.write") && (
            <Button
              variant="ghost"
              size="sm"
              className="text-muted-foreground"
              loading={recheck.isPending}
              onClick={() => recheck.mutate()}
              title="Run every compliance rule on every device now"
            >
              <RefreshCw /> Re-check
            </Button>
          )}
          <Link href="/alerts" className="text-xs font-semibold text-primary hover:underline">
            All alerts →
          </Link>
        </div>
      </CardHeader>
      <CardContent className="px-0 pb-3">
        {!data ? (
          <div className="px-5">
            <ListSkeleton rows={4} />
          </div>
        ) : (
          <>
            {data.recent_alerts.length || data.failing_rules.length ? (
              <div className="divide-y divide-border/60 border-t border-border/60">
                {alertGroups(data).map((g) => (
                  <section key={g.key}>
                    <Link
                      href={inGroup("/devices?compliance=non_compliant")}
                      className="flex flex-wrap items-center gap-2 bg-muted/40 px-5 py-2 text-sm transition-colors hover:bg-muted/70"
                    >
                      {g.severity && <SeverityBadge severity={g.severity} />}
                      <span className="font-semibold">{g.name}</span>
                      {g.devices != null && (
                        <span className="text-xs text-muted-foreground tabular">
                          {g.devices} of {data.devices_total} device{data.devices_total === 1 ? "" : "s"}
                        </span>
                      )}
                    </Link>
                    <ul className="divide-y divide-border/40">
                      {g.alerts.map((a) => {
                        const found = findings(a.message);
                        return (
                          <li key={a.id} className="flex items-start gap-3 px-5 py-2.5">
                            <div className="min-w-0 flex-1">
                              {/* As tall as the Resolve button, so the two sit level however many badges wrap below. */}
                              <p className="flex min-h-8 flex-wrap items-center gap-x-1.5 text-sm">
                                {a.device_id ? (
                                  <Link href={`/devices/${a.device_id}`} className="font-semibold hover:underline">
                                    {a.device_name ?? "Device"}
                                  </Link>
                                ) : (
                                  <span className="font-semibold">{a.title}</span>
                                )}
                                <span className="text-xs text-muted-foreground">· {relativeTime(a.created_at)}</span>
                              </p>
                              {found ? (
                                <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                                  {found.kind && <span className="text-xs text-muted-foreground">{found.kind}:</span>}
                                  {(expanded.has(a.id) ? found.items : found.items.slice(0, BADGE_LIMIT)).map((name) => (
                                    // One line each; a very long name ends in "…" with the full name on hover.
                                    <span
                                      key={name}
                                      title={name}
                                      className="max-w-[220px] truncate rounded-md border border-border bg-muted/60 px-2 py-0.5 text-xs font-medium"
                                    >
                                      {name}
                                    </span>
                                  ))}
                                  {found.items.length > BADGE_LIMIT && (
                                    <button
                                      type="button"
                                      onClick={() => toggleExpanded(a.id)}
                                      className="rounded-md px-1.5 py-0.5 text-xs font-semibold text-primary transition-colors hover:bg-primary/10 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                                    >
                                      {expanded.has(a.id) ? "Show less" : `+${found.items.length - BADGE_LIMIT} more`}
                                    </button>
                                  )}
                                </div>
                              ) : (
                                <p className="mt-0.5 break-words text-sm text-muted-foreground">{a.message}</p>
                              )}
                            </div>
                            {can("alerts.write") && (
                              <Button
                                variant="outline"
                                size="sm"
                                className="shrink-0"
                                loading={resolve.isPending && resolve.variables === a.id}
                                onClick={() => resolve.mutate(a.id)}
                                title="Mark as handled. It opens again if the device still fails the rule at the next check."
                              >
                                Resolve
                              </Button>
                            )}
                          </li>
                        );
                      })}
                    </ul>
                  </section>
                ))}
              </div>
            ) : (
              <EmptyState icon={ShieldCheck} title="Nothing needs attention" description="No open alerts and no failing compliance checks." className="py-8" />
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Device health
// ---------------------------------------------------------------------------

const healthInfo: Record<Dashboard["health"][number]["kind"], { icon: LucideIcon; label: string }> = {
  disk: { icon: HardDrive, label: "Disk" },
  cpu: { icon: Cpu, label: "CPU" },
  memory: { icon: MemoryStick, label: "Memory" },
  offline: { icon: WifiOff, label: "Offline" },
};

function DeviceHealth({ data, className }: { data?: Dashboard; className?: string }) {
  return (
    <Card className={className}>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2 pb-3">
        <CardTitle>Device health</CardTitle>
        {data && data.devices_total > 0 && (
          <span className="text-xs text-muted-foreground tabular">
            {data.devices_total - new Set(data.health.map((h) => h.device_id)).size} of {data.devices_total} healthy
          </span>
        )}
      </CardHeader>
      <CardContent className="px-0 pb-3">
        {!data ? (
          <div className="px-5">
            <ListSkeleton rows={3} />
          </div>
        ) : data.health.length ? (
          <ul className="divide-y divide-border/60">
            {data.health.map((h) => {
              const info = healthInfo[h.kind];
              const pct = h.kind !== "offline";
              const critical = pct ? h.value >= 95 : h.value >= 7 * 86400;
              return (
                <li key={`${h.device_id}-${h.kind}`}>
                  <Link href={`/devices/${h.device_id}`} className="flex items-center gap-3 px-5 py-2.5 transition-colors hover:bg-muted/60">
                    <info.icon className={cn("h-4 w-4 shrink-0", critical ? "text-destructive" : "text-warning")} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{h.name}</p>
                      {pct ? (
                        <span className="mt-1 flex h-1.5 overflow-hidden rounded-full bg-muted">
                          <span className={cn("h-full", critical ? "bg-destructive" : "bg-warning")} style={{ width: `${Math.min(h.value, 100)}%` }} />
                        </span>
                      ) : (
                        <p className="text-xs text-muted-foreground">Last seen {relativeTime(h.last_seen_at)}</p>
                      )}
                    </div>
                    <span className={cn("shrink-0 text-right text-sm font-semibold tabular", critical ? "text-destructive" : "text-warning")}>
                      {pct ? `${Math.round(h.value)}%` : formatDuration(h.value)}
                      <span className="block text-xs font-normal text-muted-foreground">{pct ? `${info.label} used` : "offline"}</span>
                    </span>
                  </Link>
                </li>
              );
            })}
          </ul>
        ) : (
          <EmptyState
            icon={CheckCircle2}
            title="All devices healthy"
            description="No disk 85% full or more, no CPU or memory at 90% or more, and nothing offline for over a day."
            className="py-8"
          />
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Security posture
// ---------------------------------------------------------------------------

function SecurityPosture({ data, inGroup }: { data?: Dashboard; inGroup: (p: string) => string }) {
  if (!data) return <Skeleton className="mt-5 h-28 rounded-xl" />;
  if (data.devices_total === 0) return null;
  const total = data.devices_total;
  // A staged in-between build (e.g. 2026.11.14-8-gce97569) counts as its release, like Update agents.
  const current = data.latest_agent
    ? data.agent_versions.filter((v) => v.version && !agentOutdated(v.version, data.latest_agent)).reduce((n, v) => n + v.count, 0)
    : null;
  const release = data.latest_agent.split("-")[0];
  const meters: { label: string; good: number | null; icon: LucideIcon; href: string; note?: string; trend: TrendSpec }[] = [
    {
      label: "Disk encryption",
      good: data.posture.encrypted,
      icon: Lock,
      href: inGroup("/devices?posture=unencrypted"),
      trend: { pick: (n) => n.encrypted, better: "up" },
    },
    {
      label: "Firewall on",
      good: data.posture.firewall,
      icon: BrickWall,
      href: inGroup("/devices?posture=firewall_off"),
      trend: { pick: (n) => n.firewall, better: "up" },
    },
    {
      label: "Antivirus",
      good: data.posture.antivirus,
      icon: Bug,
      href: inGroup("/devices?posture=no_antivirus"),
      trend: { pick: (n) => n.antivirus, better: "up" },
    },
    {
      label: "OS up to date",
      good: total - data.posture.updates_pending - (total - data.posture.reported),
      icon: Download,
      href: inGroup("/devices?posture=updates_pending"),
      note: data.posture.updates_pending ? `${data.posture.updates_pending} with pending updates` : undefined,
      trend: { pick: (n) => n.reported - n.updates_pending, better: "up" },
    },
    {
      label: "Latest agent",
      good: current,
      icon: ArrowUpCircle,
      href: inGroup("/devices"),
      note: data.latest_agent ? `Version ${release}` : "No agent build published",
      trend: { pick: (n) => n.agent_current, better: "up" },
    },
  ];
  return (
    <Card className="mt-5">
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2 pb-3">
        <CardTitle>Security posture</CardTitle>
        {data.posture.reported < total && (
          <span className="text-xs text-muted-foreground">
            {total - data.posture.reported} device{total - data.posture.reported === 1 ? " hasn't" : "s haven't"} reported yet and count as missing
          </span>
        )}
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
          {meters.map((m) => {
            const ok = m.good == null ? null : m.good >= total;
            const ratio = m.good == null ? 0 : Math.max(0, m.good) / total;
            return (
              <Link
                key={m.label}
                href={m.href}
                className="flex flex-col gap-2 rounded-lg border border-border p-3 transition-colors hover:border-primary/40 hover:bg-muted/40"
              >
                <span className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
                  <m.icon className="h-4 w-4" />
                  {m.label}
                </span>
                <span className="text-xl font-semibold tabular">
                  {m.good == null ? "—" : Math.max(0, m.good)}
                  <span className="text-sm font-normal text-muted-foreground"> / {total}</span>
                </span>
                {/* Always one line, so every card's bar sits at the same height. */}
                <span className="min-h-4 text-xs leading-4 text-muted-foreground">{m.note ?? " "}</span>
                <span
                  className={cn(
                    "mt-auto flex h-1.5 overflow-hidden rounded-full",
                    ok || m.good == null ? "bg-muted" : ratio >= 0.5 ? "bg-warning/20" : "bg-destructive/20",
                  )}
                >
                  <span
                    className={cn("h-full", ok ? "bg-success dark:bg-neon" : ratio >= 0.5 ? "bg-warning" : "bg-destructive")}
                    style={{ width: `${ratio * 100}%` }}
                  />
                </span>
                <Trend history={data.trend} spec={m.trend} />
              </Link>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// OS mix
// ---------------------------------------------------------------------------

const osColor = (os: string) => `var(--os-${["windows", "macos", "linux"].includes(os) ? os : "other"})`;

function OsMix({ data, className, inGroup }: { data?: Dashboard; className?: string; inGroup: (p: string) => string }) {
  // The slice or legend row under the cursor (or keyboard focus); its details fill the middle of the ring.
  const [hovered, setHovered] = useState<string | null>(null);
  const total = data?.devices_total ?? 0;
  const pct = (n: number) => (total ? Math.round((n / total) * 100) : 0);
  const active = data?.by_os.find((o) => o.os_family === hovered);
  const activeOffline = active ? active.count - active.online : 0;
  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <CardTitle>Operating systems</CardTitle>
      </CardHeader>
      <CardContent>
        {!data ? (
          <ListSkeleton rows={3} />
        ) : !data.by_os.length ? (
          <p className="py-6 text-center text-sm text-muted-foreground">No devices yet</p>
        ) : (
          <div className="flex flex-col items-center gap-5">
            <div className="relative h-60 w-60 shrink-0" role="img" aria-label={data.by_os.map((o) => `${osLabel(o.os_family)} ${o.count}`).join(", ")}>
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={data.by_os}
                    dataKey="count"
                    nameKey="os_family"
                    innerRadius="60%"
                    outerRadius="94%"
                    paddingAngle={data.by_os.length > 1 ? 2 : 0}
                    stroke="hsl(var(--card))"
                    strokeWidth={2}
                    startAngle={90}
                    endAngle={-270}
                    isAnimationActive={false}
                    onMouseEnter={(_, i) => setHovered(data.by_os[i]?.os_family ?? null)}
                    onMouseLeave={() => setHovered(null)}
                  >
                    {data.by_os.map((o) => (
                      <Cell
                        key={o.os_family}
                        fill={osColor(o.os_family)}
                        fillOpacity={hovered && hovered !== o.os_family ? 0.35 : 1}
                        className="cursor-pointer transition-[fill-opacity] duration-150"
                      />
                    ))}
                  </Pie>
                  {/* The hovered slice again, slightly larger, so it stands out from the ring. */}
                  {hovered && (
                    <Pie
                      data={data.by_os}
                      dataKey="count"
                      innerRadius="58%"
                      outerRadius="100%"
                      paddingAngle={data.by_os.length > 1 ? 2 : 0}
                      stroke="none"
                      startAngle={90}
                      endAngle={-270}
                      isAnimationActive={false}
                      pointerEvents="none"
                    >
                      {data.by_os.map((o) => (
                        <Cell key={o.os_family} fill={o.os_family === hovered ? osColor(o.os_family) : "transparent"} />
                      ))}
                    </Pie>
                  )}
                </PieChart>
              </ResponsiveContainer>
              <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center" aria-live="polite">
                {active ? (
                  <>
                    <span className="text-sm font-medium">{osLabel(active.os_family)}</span>
                    <span className="text-4xl font-semibold leading-tight tabular">{active.count}</span>
                    <span className="text-xs text-muted-foreground tabular">{pct(active.count)}% of devices</span>
                    {activeOffline > 0 && <span className="text-xs font-medium text-warning">{activeOffline} offline</span>}
                  </>
                ) : (
                  <>
                    <span className="text-4xl font-semibold leading-tight tabular">{total}</span>
                    <span className="text-sm text-muted-foreground">devices</span>
                  </>
                )}
              </div>
            </div>
            <ul className="w-full max-w-sm space-y-1">
              {data.by_os.map((o) => {
                const offline = o.count - o.online;
                return (
                  <li key={o.os_family}>
                    <Link
                      href={inGroup(`/devices?os=${encodeURIComponent(o.os_family)}`)}
                      onMouseEnter={() => setHovered(o.os_family)}
                      onMouseLeave={() => setHovered(null)}
                      onFocus={() => setHovered(o.os_family)}
                      onBlur={() => setHovered(null)}
                      className={cn(
                        "flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm transition-colors hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
                        hovered === o.os_family && "bg-muted/60",
                      )}
                    >
                      <span className="h-3 w-3 shrink-0 rounded-full" style={{ background: osColor(o.os_family) }} />
                      <OsIcon os={o.os_family} />
                      <span className="min-w-0 flex-1 whitespace-nowrap font-medium leading-tight">
                        {osLabel(o.os_family)}
                        {offline > 0 && <span className="block text-xs font-medium text-warning">{offline} offline</span>}
                      </span>
                      <span className="w-8 text-right font-semibold tabular">{o.count}</span>
                      <span className="w-10 text-right text-xs text-muted-foreground tabular">{pct(o.count)}%</span>
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Recent activity: commands grouped when the same action ran on several devices
// ---------------------------------------------------------------------------

type ActivityFilter = "all" | "failed" | "pending";
const PENDING = new Set(["queued", "sent", "running"]);
const FAILED = new Set(["failed", "timeout", "cancelled"]);

type Batch = { key: string; label: string; by: string; at: string; items: Command[] };

/** Groups commands with the same action and issuer created within 10 minutes of each other. */
function batches(cmds: Command[]): Batch[] {
  const out: Batch[] = [];
  for (const c of cmds) {
    const label = commandLabel(c.type, c.payload);
    const by = c.issued_by_email ?? titleCase(c.source);
    const last = out[out.length - 1];
    const prev = last?.items[last.items.length - 1];
    if (last && last.label === label && last.by === by && prev && Math.abs(Date.parse(prev.created_at) - Date.parse(c.created_at)) < 10 * 60_000) {
      last.items.push(c);
    } else {
      out.push({ key: c.id, label, by, at: c.created_at, items: [c] });
    }
  }
  return out;
}

function RecentActivity({ data, className }: { data?: Dashboard; className?: string }) {
  const [filter, setFilter] = useState<ActivityFilter>("all");
  const [open, setOpen] = useState<Set<string>>(new Set());
  const list = useMemo(() => {
    const cmds = (data?.recent_commands ?? []).filter((c) =>
      filter === "all" ? true : filter === "failed" ? FAILED.has(c.status) : PENDING.has(c.status),
    );
    return batches(cmds).slice(0, 8);
  }, [data, filter]);

  return (
    <Card className={className}>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-2 pb-3">
        <CardTitle>Recent activity</CardTitle>
        <Segmented
          value={filter}
          onChange={setFilter}
          options={[
            { value: "all", label: "All" },
            { value: "failed", label: "Failed" },
            { value: "pending", label: "Pending" },
          ]}
        />
      </CardHeader>
      <CardContent className="px-0 pb-2">
        {!data ? (
          <div className="px-5">
            <ListSkeleton rows={4} />
          </div>
        ) : list.length ? (
          <ul className="divide-y divide-border/60">
            {list.map((b) => {
              const single = b.items.length === 1 ? b.items[0]! : null;
              const expanded = open.has(b.key);
              const counts = b.items.reduce<Record<string, number>>((m, c) => ({ ...m, [c.status]: (m[c.status] ?? 0) + 1 }), {});
              return (
                <li key={b.key}>
                  <div className="flex items-center gap-3 px-5 py-2.5">
                    {single ? (
                      <Terminal className="h-4 w-4 shrink-0 text-muted-foreground" />
                    ) : (
                      <button
                        type="button"
                        aria-expanded={expanded}
                        aria-label={expanded ? "Hide devices" : "Show devices"}
                        className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
                        onClick={() =>
                          setOpen((s) => {
                            const n = new Set(s);
                            if (n.has(b.key)) n.delete(b.key);
                            else n.add(b.key);
                            return n;
                          })
                        }
                      >
                        {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                      </button>
                    )}
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm">
                        <span className="font-medium">{b.label}</span> <span className="text-muted-foreground">on</span>{" "}
                        {single ? (
                          <Link href={`/devices/${single.device_id}`} className="hover:underline">
                            {single.device_name}
                          </Link>
                        ) : (
                          <span>{b.items.length} devices</span>
                        )}
                      </p>
                      <p className="truncate text-xs text-muted-foreground">
                        {b.by} · {relativeTime(b.at)}
                      </p>
                    </div>
                    {single ? (
                      <CommandStatusBadge status={single.status} />
                    ) : (
                      <span className="flex shrink-0 flex-wrap justify-end gap-1">
                        {Object.entries(counts).map(([status, n]) => (
                          <span key={status} className="inline-flex items-center gap-1">
                            <CommandStatusBadge status={status as Command["status"]} />
                            <span className="text-xs font-semibold tabular">×{n}</span>
                          </span>
                        ))}
                      </span>
                    )}
                  </div>
                  {expanded && (
                    <ul className="mb-2 ml-12 mr-5 space-y-1 border-l border-border pl-3">
                      {b.items.map((c) => (
                        <li key={c.id} className="flex items-center gap-2 text-sm">
                          <Link href={`/devices/${c.device_id}`} className="min-w-0 flex-1 truncate hover:underline">
                            {c.device_name}
                          </Link>
                          <CommandStatusBadge status={c.status} />
                        </li>
                      ))}
                    </ul>
                  )}
                </li>
              );
            })}
          </ul>
        ) : (
          <EmptyState
            icon={Terminal}
            title={filter === "all" ? "No commands yet" : filter === "failed" ? "No failed commands" : "Nothing pending"}
            className="py-8"
          />
        )}
      </CardContent>
    </Card>
  );
}

function ListSkeleton({ rows }: { rows: number }) {
  return (
    <div className="space-y-2.5 py-1">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-6" />
      ))}
    </div>
  );
}

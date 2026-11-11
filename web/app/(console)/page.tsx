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
import { api, qk, type Command, type Dashboard } from "@/lib/api";
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
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from "recharts";

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
  warning: { tile: "border-warning/50 bg-warning/5", value: "text-warning", icon: "text-warning" },
  danger: { tile: "border-destructive/50 bg-destructive/5", value: "text-destructive", icon: "text-destructive" },
};

function StatTiles({ data, inGroup }: { data: Dashboard; inGroup: (p: string) => string }) {
  const pct = data.devices_total > 0 ? Math.round((data.compliant / data.devices_total) * 100) : null;
  const tiles: { label: string; value: React.ReactNode; sub?: string; icon: LucideIcon; tone: Tone; href: string }[] = [
    { label: "Devices", value: data.devices_total, icon: Monitor, tone: "brand", href: inGroup("/devices") },
    { label: "Online", value: data.online, icon: Wifi, tone: "success", href: inGroup("/devices?status=online") },
    {
      label: "Offline",
      value: data.offline,
      icon: WifiOff,
      tone: data.offline > 0 ? "warning" : "default",
      href: inGroup("/devices?status=offline"),
    },
    {
      label: "Compliant",
      value: pct == null ? "—" : `${pct}%`,
      sub: data.non_compliant > 0 ? `${data.non_compliant} non-compliant` : "All passing",
      icon: CheckCircle2,
      tone: data.non_compliant > 0 ? "danger" : "success",
      href: inGroup("/devices?compliance=non_compliant"),
    },
    { label: "Open alerts", value: data.open_alerts, icon: Bell, tone: data.open_alerts > 0 ? "danger" : "default", href: "/alerts" },
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
          </Link>
        );
      })}
    </div>
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

function NeedsAttention({ data, className, inGroup }: { data?: Dashboard; className?: string; inGroup: (p: string) => string }) {
  const qc = useQueryClient();
  const can = useCan();
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
            <Button variant="outline" size="sm" loading={recheck.isPending} onClick={() => recheck.mutate()}>
              <RefreshCw /> Re-check compliance
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
            {data.failing_rules.length > 0 && (
              <div className="flex flex-wrap gap-2 px-5 pb-3">
                {data.failing_rules.map((r) => (
                  <Link
                    key={r.rule_id}
                    href={inGroup("/devices?compliance=non_compliant")}
                    className="inline-flex items-center gap-2 rounded-full border border-border bg-muted/50 py-1 pl-1 pr-3 text-xs transition-colors hover:bg-muted"
                  >
                    <SeverityBadge severity={r.severity} />
                    <span className="font-medium">{r.name}</span>
                    <span className="text-muted-foreground tabular">
                      {r.devices} of {data.devices_total}
                    </span>
                  </Link>
                ))}
              </div>
            )}
            {data.recent_alerts.length ? (
              <ul className="divide-y divide-border/60 border-t border-border/60">
                {data.recent_alerts.map((a) => (
                  <li key={a.id} className="flex items-start gap-3 px-5 py-3">
                    <SeverityBadge severity={a.severity} />
                    <div className="min-w-0 flex-1">
                      <p className="text-sm">
                        {a.device_id ? (
                          <Link href={`/devices/${a.device_id}`} className="font-semibold hover:underline">
                            {a.device_name ?? "Device"}
                          </Link>
                        ) : (
                          <span className="font-semibold">Organization</span>
                        )}
                        <span className="text-muted-foreground"> · {a.title}</span>
                      </p>
                      <p className="mt-0.5 break-words text-sm text-muted-foreground">{a.message}</p>
                      <p className="mt-1 text-xs text-muted-foreground">Opened {relativeTime(a.created_at)}</p>
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
                ))}
              </ul>
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
      <CardHeader className="pb-3">
        <CardTitle>Device health</CardTitle>
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
  const current = data.latest_agent ? (data.agent_versions.find((v) => v.version === data.latest_agent)?.count ?? 0) : null;
  const meters: { label: string; good: number | null; icon: LucideIcon; href: string; note?: string }[] = [
    { label: "Disk encryption", good: data.posture.encrypted, icon: Lock, href: inGroup("/devices?posture=unencrypted") },
    { label: "Firewall on", good: data.posture.firewall, icon: BrickWall, href: inGroup("/devices?posture=firewall_off") },
    { label: "Antivirus", good: data.posture.antivirus, icon: Bug, href: inGroup("/devices?posture=no_antivirus") },
    {
      label: "OS up to date",
      good: total - data.posture.updates_pending - (total - data.posture.reported),
      icon: Download,
      href: inGroup("/devices?posture=updates_pending"),
      note: data.posture.updates_pending ? `${data.posture.updates_pending} with pending updates` : undefined,
    },
    {
      label: "Latest agent",
      good: current,
      icon: ArrowUpCircle,
      href: inGroup("/devices"),
      note: data.latest_agent ? `Version ${data.latest_agent}` : "No agent build published",
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
                <span className="flex h-1.5 overflow-hidden rounded-full bg-muted">
                  <span
                    className={cn("h-full", ok ? "bg-success dark:bg-neon" : ratio >= 0.5 ? "bg-warning" : "bg-destructive")}
                    style={{ width: `${ratio * 100}%` }}
                  />
                </span>
                {m.note && <span className="text-xs text-muted-foreground">{m.note}</span>}
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
  const total = data?.devices_total ?? 0;
  const pct = (n: number) => (total ? Math.round((n / total) * 100) : 0);
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
          <div className="flex flex-col items-center gap-5 sm:flex-row">
            <div className="relative h-44 w-44 shrink-0" role="img" aria-label={data.by_os.map((o) => `${osLabel(o.os_family)} ${o.count}`).join(", ")}>
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={data.by_os}
                    dataKey="count"
                    nameKey="os_family"
                    innerRadius="58%"
                    outerRadius="100%"
                    paddingAngle={data.by_os.length > 1 ? 2 : 0}
                    stroke="hsl(var(--card))"
                    strokeWidth={2}
                    startAngle={90}
                    endAngle={-270}
                    isAnimationActive={false}
                  >
                    {data.by_os.map((o) => (
                      <Cell key={o.os_family} fill={osColor(o.os_family)} />
                    ))}
                  </Pie>
                  <Tooltip
                    content={({ active, payload }) => {
                      const o = active ? (payload?.[0]?.payload as Dashboard["by_os"][number] | undefined) : undefined;
                      if (!o) return null;
                      const offline = o.count - o.online;
                      return (
                        <div className="rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-lift">
                          <p className="font-medium">{osLabel(o.os_family)}</p>
                          <p className="tabular">
                            {o.count} device{o.count === 1 ? "" : "s"} ({pct(o.count)}%)
                            {offline > 0 && <span className="text-warning"> · {offline} offline</span>}
                          </p>
                        </div>
                      );
                    }}
                  />
                </PieChart>
              </ResponsiveContainer>
              <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                <span className="text-2xl font-semibold tabular">{total}</span>
                <span className="text-xs text-muted-foreground">devices</span>
              </div>
            </div>
            <ul className="w-full min-w-0 flex-1 space-y-1">
              {data.by_os.map((o) => {
                const offline = o.count - o.online;
                return (
                  <li key={o.os_family}>
                    <Link
                      href={inGroup(`/devices?os=${encodeURIComponent(o.os_family)}`)}
                      className="flex items-center gap-2.5 rounded-lg px-2 py-1.5 text-sm transition-colors hover:bg-muted/60"
                    >
                      <span className="h-3 w-3 shrink-0 rounded-sm" style={{ background: osColor(o.os_family) }} />
                      <OsIcon os={o.os_family} />
                      <span className="min-w-0 flex-1 truncate font-medium">{osLabel(o.os_family)}</span>
                      {offline > 0 && <span className="text-xs font-medium text-warning">{offline} offline</span>}
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

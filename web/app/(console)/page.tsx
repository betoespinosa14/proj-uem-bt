"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Bell, CheckCircle2, Monitor, ShieldCheck, Terminal, Wifi, WifiOff } from "lucide-react";
import { api, qk, type Dashboard } from "@/lib/api";
import { osLabel, relativeTime, titleCase } from "@/lib/utils";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { commandLabel } from "@/components/commands";
import { CommandStatusBadge, DeviceSearch, EmptyState, ErrorState, OsIcon, PageHeader, SeverityBadge } from "@/components/common";
import { useMe } from "@/lib/session";

function greeting(d = new Date()) {
  const h = d.getHours();
  return h < 12 ? "Good morning" : h < 18 ? "Good afternoon" : "Good evening";
}

export default function DashboardPage() {
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.dashboard, queryFn: api.dashboard });
  const { data: me } = useMe();

  if (error) return <ErrorState error={error} onRetry={refetch} />;

  const compliancePct =
    data && data.devices_total > 0 ? Math.round((data.compliant / data.devices_total) * 100) : null;
  const firstName = (me?.user.name || "").split(" ")[0];

  return (
    <>
      <PageHeader
        title={`${greeting()}${firstName ? `, ${firstName}` : ""}`}
        description="Fleet health at a glance. Updates live as devices check in."
        statsAside={<DeviceSearch />}
        stats={
          data
            ? [
                { label: "Devices", value: data.devices_total, icon: Monitor, tone: "brand" },
                { label: "Online", value: data.online, icon: Wifi, tone: "success" },
                { label: "Offline", value: data.offline, icon: WifiOff, tone: data.offline > 0 ? "warning" : "default" },
                {
                  label: `Compliant · ${data.non_compliant} non-compliant`,
                  value: compliancePct == null ? "—" : `${compliancePct}%`,
                  icon: CheckCircle2,
                  tone: data.non_compliant > 0 ? "danger" : "success",
                },
                { label: "Open alerts", value: data.open_alerts, icon: Bell, tone: data.open_alerts > 0 ? "danger" : "default" },
              ]
            : undefined
        }
      />
      {isLoading && (
        <div className="-mt-2 mb-7 flex flex-wrap gap-8">
          {[0, 1, 2, 3, 4].map((i) => (
            <Skeleton key={i} className="h-10 w-28" />
          ))}
        </div>
      )}

      {data && data.devices_total === 0 && (
        <Card className="mb-6">
          <EmptyState
            icon={Monitor}
            title="No devices enrolled yet"
            description="Create an enrollment token and run the one-line installer on a Windows, macOS or Linux device."
            action={
              <Link href="/enrollment" className="text-sm font-medium text-primary hover:underline">
                Go to Enrollment →
              </Link>
            }
          />
        </Card>
      )}

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-5">
        <Card className="lg:col-span-3">
          <CardHeader className="flex-row items-center justify-between pb-2">
            <CardTitle>Devices by operating system</CardTitle>
            {data && (
              <span className="flex items-center gap-3 text-xs text-muted-foreground">
                <Legend className="bg-success dark:bg-neon" label="Online" />
                <Legend className="bg-warning" label="Offline" />
              </span>
            )}
          </CardHeader>
          <CardContent>{data ? <OsBreakdown data={data} /> : <ListSkeleton rows={3} />}</CardContent>
        </Card>
        <Card className="lg:col-span-2">
          <CardHeader className="flex-row items-center justify-between pb-2">
            <CardTitle>Failing compliance checks</CardTitle>
            <Link href="/compliance" className="text-xs font-semibold text-primary hover:underline">
              View compliance →
            </Link>
          </CardHeader>
          <CardContent className="px-0 pb-3">
            {!data ? (
              <div className="px-5">
                <ListSkeleton rows={3} />
              </div>
            ) : data.failing_rules.length ? (
              <ul className="divide-y divide-border/60">
                {data.failing_rules.map((r) => (
                  <li key={r.rule_id}>
                    <Link
                      href="/devices?compliance=non_compliant"
                      className="flex items-center gap-3 px-5 py-2 text-sm transition-colors hover:bg-muted/60"
                    >
                      <SeverityBadge severity={r.severity} />
                      <span className="min-w-0 flex-1 truncate font-medium">{r.name}</span>
                      <span className="shrink-0 text-xs text-muted-foreground tabular">
                        {r.devices} of {data.devices_total} {data.devices_total === 1 ? "device" : "devices"}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            ) : (
              <EmptyState icon={ShieldCheck} title="All checks passing" description="No device fails an enabled compliance rule." className="py-8" />
            )}
          </CardContent>
        </Card>
      </div>

      <div className="mt-5 grid grid-cols-1 gap-5 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Open alerts</CardTitle>
            <Link href="/alerts" className="text-xs font-semibold text-primary hover:underline">
              View all
            </Link>
          </CardHeader>
          <CardContent className="px-0 pb-2">
            {data?.recent_alerts.length ? (
              <ul className="divide-y divide-border/60">
                {data.recent_alerts.map((a) => (
                  <li key={a.id} className="flex items-start gap-3 px-5 py-2 transition-colors hover:bg-muted/60">
                    <SeverityBadge severity={a.severity} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">
                        {a.device_id ? (
                          <Link href={`/devices/${a.device_id}`} className="hover:underline">
                            {a.title}
                          </Link>
                        ) : (
                          a.title
                        )}
                      </p>
                      <p className="truncate text-xs text-muted-foreground">{a.message}</p>
                    </div>
                    <span className="shrink-0 text-xs text-muted-foreground">{relativeTime(a.created_at)}</span>
                  </li>
                ))}
              </ul>
            ) : (
              <EmptyState icon={Bell} title="No open alerts" className="py-8" />
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Recent commands</CardTitle>
          </CardHeader>
          <CardContent className="px-0 pb-2">
            {data?.recent_commands.length ? (
              <ul className="divide-y divide-border/60">
                {data.recent_commands.slice(0, 8).map((c) => (
                  <li key={c.id} className="flex items-center gap-3 px-5 py-2 transition-colors hover:bg-muted/60">
                    <Terminal className="h-4 w-4 shrink-0 text-muted-foreground" />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm">
                        <span className="font-medium">{commandLabel(c.type, c.payload)}</span>{" "}
                        <span className="text-muted-foreground">on</span>{" "}
                        <Link href={`/devices/${c.device_id}`} className="hover:underline">
                          {c.device_name}
                        </Link>
                      </p>
                      <p className="truncate text-xs text-muted-foreground">
                        {c.issued_by_email ?? titleCase(c.source)} · {relativeTime(c.created_at)}
                      </p>
                    </div>
                    <CommandStatusBadge status={c.status} />
                  </li>
                ))}
              </ul>
            ) : (
              <EmptyState icon={Terminal} title="No commands yet" className="py-8" />
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}

function Legend({ className, label }: { className: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={`h-2 w-2 rounded-full ${className}`} />
      {label}
    </span>
  );
}

/** One compact row per OS: a bar split into online and offline devices. */
function OsBreakdown({ data }: { data: Dashboard }) {
  if (!data.by_os.length) return <p className="py-6 text-center text-sm text-muted-foreground">No devices yet</p>;
  const max = Math.max(...data.by_os.map((o) => o.count));
  return (
    <ul className="space-y-1">
      {data.by_os.map((o) => {
        const offline = o.count - o.online;
        return (
          <li key={o.os_family}>
            <Link
              href={`/devices?os=${encodeURIComponent(o.os_family)}`}
              className="grid grid-cols-[7.5rem_1fr_auto] items-center gap-3 rounded-lg px-2 py-1.5 text-sm transition-colors hover:bg-muted/60"
            >
              <span className="flex items-center gap-2 truncate font-medium">
                <OsIcon os={o.os_family} />
                {osLabel(o.os_family)}
              </span>
              <span className="flex h-2.5 overflow-hidden rounded-full bg-muted" title={`${o.online} online, ${offline} offline`}>
                <span className="h-full bg-success dark:bg-neon" style={{ width: `${(o.online / max) * 100}%` }} />
                <span className="h-full bg-warning" style={{ width: `${(offline / max) * 100}%` }} />
              </span>
              <span className="w-32 text-right text-xs text-muted-foreground tabular">
                <span className="font-semibold text-foreground">{o.online}</span> online ·{" "}
                <span className={offline > 0 ? "font-semibold text-foreground" : undefined}>{offline} offline</span>
              </span>
            </Link>
          </li>
        );
      })}
    </ul>
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

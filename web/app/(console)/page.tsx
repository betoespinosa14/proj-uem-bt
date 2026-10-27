"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Bar, BarChart, Cell, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Bell, CheckCircle2, Monitor, Terminal, Wifi, WifiOff } from "lucide-react";
import { api, qk } from "@/lib/api";
import { osLabel, relativeTime, titleCase } from "@/lib/utils";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { commandLabel } from "@/components/commands";
import { CommandStatusBadge, EmptyState, ErrorState, PageHeader, SeverityBadge, StatCard } from "@/components/common";

const tooltipStyle = {
  contentStyle: {
    background: "hsl(var(--popover))",
    border: "1px solid hsl(var(--border))",
    borderRadius: 8,
    fontSize: 12,
    color: "hsl(var(--popover-foreground))",
  },
  cursor: { fill: "hsl(var(--muted))" },
};

export default function DashboardPage() {
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.dashboard, queryFn: api.dashboard });

  if (error) return <ErrorState error={error} onRetry={refetch} />;

  const compliancePct =
    data && data.devices_total > 0 ? Math.round((data.compliant / data.devices_total) * 100) : null;
  const complianceData = data
    ? [
        { name: "Compliant", value: data.compliant, color: "hsl(var(--success))" },
        { name: "Non-compliant", value: data.non_compliant, color: "hsl(var(--destructive))" },
        { name: "Unknown", value: data.unknown, color: "hsl(var(--warning))" },
      ].filter((d) => d.value > 0)
    : [];

  return (
    <>
      <PageHeader title="Dashboard" description="Fleet health at a glance. Updates live as devices check in." />

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
        {isLoading || !data ? (
          [0, 1, 2, 3, 4].map((i) => <Skeleton key={i} className="h-[104px]" />)
        ) : (
          <>
            <StatCard label="Devices" value={data.devices_total} icon={Monitor} />
            <StatCard label="Online" value={data.online} icon={Wifi} tone="success" />
            <StatCard label="Offline" value={data.offline} icon={WifiOff} tone="warning" />
            <StatCard
              label="Compliance"
              value={compliancePct == null ? "—" : `${compliancePct}%`}
              sub={`${data.non_compliant} non-compliant`}
              icon={CheckCircle2}
              tone={data.non_compliant > 0 ? "danger" : "success"}
            />
            <StatCard label="Open alerts" value={data.open_alerts} icon={Bell} tone={data.open_alerts > 0 ? "danger" : "default"} />
          </>
        )}
      </div>

      {data && data.devices_total === 0 && (
        <Card className="mt-6">
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

      <div className="mt-6 grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle>Devices by operating system</CardTitle>
          </CardHeader>
          <CardContent className="h-64">
            {data && data.by_os.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={data.by_os.map((o) => ({ name: osLabel(o.os_family), count: o.count }))} margin={{ left: -20 }}>
                  <XAxis dataKey="name" tickLine={false} axisLine={false} fontSize={12} stroke="hsl(var(--muted-foreground))" />
                  <YAxis allowDecimals={false} tickLine={false} axisLine={false} fontSize={12} stroke="hsl(var(--muted-foreground))" />
                  <Tooltip {...tooltipStyle} />
                  <Bar dataKey="count" name="Devices" fill="hsl(var(--primary))" radius={[4, 4, 0, 0]} maxBarSize={56} isAnimationActive={false} />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <ChartPlaceholder loading={isLoading} />
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Compliance</CardTitle>
          </CardHeader>
          <CardContent className="h-64">
            {complianceData.length > 0 ? (
              <div className="relative h-full">
                <ResponsiveContainer width="100%" height="100%">
                  <PieChart>
                    <Pie data={complianceData} dataKey="value" nameKey="name" innerRadius="62%" outerRadius="85%" paddingAngle={complianceData.length > 1 ? 2 : 0} stroke="none" isAnimationActive={false}>
                      {complianceData.map((d) => (
                        <Cell key={d.name} fill={d.color} />
                      ))}
                    </Pie>
                    <Tooltip {...tooltipStyle} />
                  </PieChart>
                </ResponsiveContainer>
                <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                  <span className="text-2xl font-semibold tabular">{compliancePct}%</span>
                  <span className="text-xs text-muted-foreground">compliant</span>
                </div>
              </div>
            ) : (
              <ChartPlaceholder loading={isLoading} />
            )}
          </CardContent>
        </Card>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="flex-row items-center justify-between">
            <CardTitle>Open alerts</CardTitle>
            <Link href="/alerts" className="text-xs font-medium text-primary hover:underline">
              View all
            </Link>
          </CardHeader>
          <CardContent className="px-0 pb-2">
            {data?.recent_alerts.length ? (
              <ul className="divide-y divide-border">
                {data.recent_alerts.map((a) => (
                  <li key={a.id} className="flex items-start gap-3 px-5 py-2.5">
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
              <ul className="divide-y divide-border">
                {data.recent_commands.slice(0, 8).map((c) => (
                  <li key={c.id} className="flex items-center gap-3 px-5 py-2.5">
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

function ChartPlaceholder({ loading }: { loading: boolean }) {
  return loading ? (
    <Skeleton className="h-full" />
  ) : (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">No data yet</div>
  );
}


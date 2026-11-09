"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, AppWindow, Clock, LayoutDashboard, PackageX, Search, Table2, Users } from "lucide-react";
import { api, qk, type FleetAppRow, type FleetUsageFilters, type FleetUsageRow, type UsageGroup, type UsagePeriod } from "@/lib/api";
import { useDebounced } from "@/lib/hooks";
import { useCan, useMe } from "@/lib/session";
import { toast } from "@/components/toast";
import { formatDateTime, formatHours, relativeTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Select } from "@/components/ui/select";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Segmented } from "@/components/ui/tabs";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, ExportLink, PageHeader, type HeadlineStat } from "@/components/common";
import { Skeleton } from "@/components/ui/skeleton";
import { UsageDashboard } from "@/components/usage-dashboard";

const PERIODS = { "30": "30 days", "90": "90 days", "365": "1 year" } as const;
const GROUPS = { app: "By app", app_user: "By app and user" } as const;

type View = "dashboard" | "table";
const VIEW_KEY = "usage-view";

const ACTIVE_HINT = "In front while the user was typing or using the mouse";
const OPEN_HINT = "Running, whether or not it was being used";

export default function UsagePage() {
  const { data: me } = useMe();
  const [q, setQ] = useState("");
  const [period, setPeriod] = useState<keyof typeof PERIODS>("30");
  const [group, setGroup] = useState<UsageGroup>("app");
  const [unused, setUnused] = useState(false);
  const [groupId, setGroupId] = useState("");
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  // Dashboard first; remember the last choice in this browser (read after
  // mount so the server render and the first client render match).
  const [view, setViewState] = useState<View>("dashboard");
  useEffect(() => {
    try {
      if (localStorage.getItem(VIEW_KEY) === "table") setViewState("table");
    } catch {}
  }, []);
  const setView = (v: View) => {
    setViewState(v);
    try {
      localStorage.setItem(VIEW_KEY, v);
    } catch {}
  };
  const days = Number(period) as UsagePeriod;
  const filters: FleetUsageFilters = { app: useDebounced(q.trim()), days, unused: unused || undefined, group_id: groupId || undefined };
  const byApp = useQuery({
    queryKey: qk.usage(filters, "app"),
    queryFn: () => api.usage.byApp(filters),
    placeholderData: (p) => p,
    enabled: view === "table" && group === "app",
  });
  const byUser = useQuery({
    queryKey: qk.usage(filters, "app_user"),
    queryFn: () => api.usage.list(filters),
    placeholderData: (p) => p,
    enabled: view === "table" && group === "app_user",
  });
  const summaryFilters = { app: filters.app, days, group_id: filters.group_id };
  const summary = useQuery({
    queryKey: qk.usageSummary(summaryFilters),
    queryFn: () => api.usage.summary(summaryFilters),
    placeholderData: (p) => p,
    enabled: view === "dashboard",
  });
  // From the dashboard: the license-reclamation list.
  const showUnused = () => {
    setGroup("app");
    setUnused(true);
    setView("table");
  };
  // From the dashboard: show who used one app, in the detailed table.
  const pickApp = (app: string) => {
    setQ(app);
    setGroup("app_user");
    setUnused(false);
    setView("table");
  };
  // Infrastructure apps (open by design, e.g. remote-access hosts).
  const can = useCan();
  const qc = useQueryClient();
  const infra = useMutation({
    mutationFn: ({ app, mark }: { app: string; mark: boolean }) => (mark ? api.usage.markInfrastructure(app) : api.usage.unmarkInfrastructure(app)),
    onSuccess: (_, { app, mark }) => {
      void qc.invalidateQueries({ queryKey: ["usage", "summary"] });
      void qc.invalidateQueries({ queryKey: qk.usageInfrastructure });
      toast.success(mark ? `${app} marked as infrastructure` : `${app} is ranked again`, mark ? "Its open time no longer counts as idle." : undefined);
    },
    onError: (e) => toast.error("Couldn't update infrastructure apps", e),
  });
  const totals = summary.data?.totals;
  const stats: HeadlineStat[] | undefined =
    view === "dashboard" && totals
      ? [
          { label: "Active", value: formatHours(totals.active_seconds), icon: Activity, tone: "brand" },
          {
            label: "of open time active",
            value: totals.open_minutes > 0 ? formatShare(totals.active_seconds / (totals.open_minutes * 60)) : "—",
            icon: Clock,
            hint: `${formatHours(totals.active_seconds)} active of ${formatHours(totals.open_minutes * 60)} open, in apps people used. Background-only processes are not counted.`,
          },
          { label: "Apps used", value: totals.apps_used, icon: AppWindow },
          { label: "Active users", value: totals.active_users, icon: Users },
          {
            label: "Installed, not used",
            value: totals.unused_apps,
            icon: PackageX,
            tone: totals.unused_apps > 0 ? "warning" : "default",
            onClick: showUnused,
            cta: "Review list",
            hint: "Third-party apps installed but not used in this period. Click to see the list.",
          },
        ]
      : undefined;
  // Badge apps marked as infrastructure in the table, so they aren't re-audited.
  const infraList = useQuery({ queryKey: qk.usageInfrastructure, queryFn: api.usage.infrastructure, enabled: view === "table" });
  const infraSet = new Set((infraList.data?.items ?? []).map((i) => i.app.toLowerCase()));
  const current = group === "app" ? byApp : byUser;
  const count = current.data?.items.length ?? 0;
  const off = me?.org.usage_tracking === false;

  return (
    <>
      <PageHeader
        title="App usage"
        description="How much each installed app is actively used, for license and subscription audits. Search an app to see every device it's installed on, used or not."
        stats={stats}
        actions={
          <>
            <Segmented
              value={view}
              onChange={setView}
              options={[
                { value: "dashboard", label: <span className="inline-flex items-center gap-1.5"><LayoutDashboard className="h-3.5 w-3.5" />Dashboard</span> },
                { value: "table", label: <span className="inline-flex items-center gap-1.5"><Table2 className="h-3.5 w-3.5" />Table</span> },
              ]}
            />
            {view === "table" && unused && (
              <ExportLink
                href={api.usage.reclaimUrl(summaryFilters)}
                label="Export reclaim list"
                title="One row per unused third-party app per device, with publisher, version, device user and last use, for procurement"
              />
            )}
            <ExportLink href={api.usage.exportUrl(filters, group)} />
          </>
        }
      />
      {off && (
        <div className="mb-4 rounded-lg border border-warning/25 bg-warning/10 px-4 py-2.5 text-sm">
          Usage tracking is off, so no new usage is being recorded. Turn it on in{" "}
          <Link href="/settings" className="font-medium underline">
            Settings
          </Link>
          .
        </div>
      )}
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <div className="relative min-w-[14rem] flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search an app, e.g. Photoshop" className="pl-8" aria-label="Search apps" />
        </div>
        {view === "table" && (
          <Segmented value={group} onChange={setGroup} options={Object.entries(GROUPS).map(([v, l]) => ({ value: v as UsageGroup, label: l }))} />
        )}
        <Segmented value={period} onChange={setPeriod} options={Object.entries(PERIODS).map(([v, l]) => ({ value: v as keyof typeof PERIODS, label: l }))} />
        <Select
          aria-label="Group"
          className="w-40"
          value={groupId}
          onValueChange={setGroupId}
          placeholder="Any group"
          options={(groups.data?.items ?? []).map((g) => ({ value: g.id, label: g.name }))}
        />
        {view === "table" && (
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={unused} onCheckedChange={setUnused} aria-label="Only installed apps not used in this period" />
            Not used in {PERIODS[period]}
          </label>
        )}
      </div>
      {view === "dashboard" ? (
        summary.error ? (
          <ErrorState error={summary.error} onRetry={summary.refetch} />
        ) : !summary.data ? (
          <DashboardSkeleton />
        ) : summary.data.daily.length === 0 ? (
          <Card>
            <EmptyState
              icon={AppWindow}
              title={q ? "No use of matching apps in this period" : "No app usage recorded in this period"}
              description={summary.data.totals.unused_apps > 0 ? "Switch to the table and turn on “Not used” to see installed apps nobody opened." : undefined}
            />
          </Card>
        ) : (
          <UsageDashboard
            data={summary.data}
            days={days}
            onPickApp={pickApp}
            onInfrastructure={can("devices.write") ? (app, mark) => infra.mutate({ app, mark }) : undefined}
          />
        )
      ) : current.error ? (
        <ErrorState error={current.error} onRetry={current.refetch} />
      ) : (
        <Card className="overflow-hidden">
          {current.isLoading ? (
            <TableSkeleton rows={8} cols={6} />
          ) : count === 0 ? (
            <EmptyState
              icon={AppWindow}
              title={unused ? "Every installed third-party app was used in this period" : q ? "No matching apps" : "No app usage recorded in this period"}
            />
          ) : (
            <div className="max-h-[40rem] overflow-auto">
              {group === "app" ? (
                <ByAppTable items={byApp.data?.items ?? []} infra={infraSet} />
              ) : (
                <ByUserTable items={byUser.data?.items ?? []} days={days} infra={infraSet} />
              )}
              {count >= 10000 && <p className="p-3 text-center text-xs text-muted-foreground">Showing the first 10,000 rows. Search an app to narrow it down.</p>}
            </div>
          )}
        </Card>
      )}
    </>
  );
}

/** Active share of open time; small shares keep a decimal so they don't read as 0%. */
function formatShare(r: number) {
  const pct = r * 100;
  if (pct > 0 && pct < 1) return `${pct.toFixed(1)}%`;
  return `${Math.round(pct)}%`;
}

function DashboardSkeleton() {
  return (
    <div className="space-y-5">
      <Skeleton className="h-72 rounded-2xl" />
      <div className="grid gap-5 lg:grid-cols-2">
        <Skeleton className="h-64 rounded-2xl" />
        <Skeleton className="h-64 rounded-2xl" />
      </div>
    </div>
  );
}

function LastUsed({ ts }: { ts: string | null }) {
  return (
    <TD className="text-muted-foreground" title={ts ? formatDateTime(ts) : undefined}>
      {ts ? relativeTime(ts) : "Never"}
    </TD>
  );
}

function InfraBadge({ app, infra }: { app: string; infra: Set<string> }) {
  if (!infra.has(app.toLowerCase())) return null;
  return (
    <Badge variant="neutral" className="ml-2" title="Marked as infrastructure: open by design, so its open time isn't ranked as idle">
      Infrastructure
    </Badge>
  );
}

function ByAppTable({ items, infra }: { items: FleetAppRow[]; infra: Set<string> }) {
  return (
    <Table>
      <THead className="sticky top-0 bg-card">
        <TR>
          <TH>App</TH>
          <TH className="text-right" title={ACTIVE_HINT}>
            Active
          </TH>
          <TH className="text-right" title={OPEN_HINT}>
            Open
          </TH>
          <TH className="text-right">Users</TH>
          <TH className="text-right" title="Devices it was used on in this period, of the devices it is installed on">
            Used on / installed on
          </TH>
          <TH>Last used</TH>
        </TR>
      </THead>
      <TBody>
        {items.map((u) => (
          <TR key={u.app}>
            <TD className="font-medium">
              {u.app}
              <InfraBadge app={u.app} infra={infra} />
            </TD>
            <TD className="text-right font-medium tabular">{u.users ? formatHours(u.active_seconds) : <Badge variant="warning">Unused</Badge>}</TD>
            <TD className="text-right tabular text-muted-foreground">{u.users ? formatHours(u.minutes * 60) : "—"}</TD>
            <TD className="text-right tabular">{u.users}</TD>
            <TD className="text-right tabular">
              {u.devices} / {u.installed_on || "—"}
            </TD>
            <LastUsed ts={u.last_used} />
          </TR>
        ))}
      </TBody>
    </Table>
  );
}

function ByUserTable({ items, days, infra }: { items: FleetUsageRow[]; days: number; infra: Set<string> }) {
  return (
    <Table>
      <THead className="sticky top-0 bg-card">
        <TR>
          <TH>App</TH>
          <TH>Device</TH>
          <TH>User</TH>
          <TH className="text-right" title={ACTIVE_HINT}>
            Active
          </TH>
          <TH className="text-right" title={OPEN_HINT}>
            Open
          </TH>
          <TH className="text-right">Days used</TH>
          <TH>Last used</TH>
        </TR>
      </THead>
      <TBody>
        {items.map((u) => (
          <TR key={`${u.device_id}-${u.app}-${u.user}`}>
            <TD className="font-medium">
              {u.app}
              <InfraBadge app={u.app} infra={infra} />
              {!u.installed && (
                <Badge variant="neutral" className="ml-2" title="Used, but no longer in the device's installed software list">
                  Not installed
                </Badge>
              )}
            </TD>
            <TD>
              <Link href={`/devices/${u.device_id}`} className="hover:underline">
                {u.device_name}
              </Link>
            </TD>
            <TD className={u.user ? "" : "text-muted-foreground"}>{u.user || "—"}</TD>
            <TD className="text-right font-medium tabular">{u.user ? formatHours(u.active_seconds) : <Badge variant="warning">Unused</Badge>}</TD>
            <TD className="text-right tabular text-muted-foreground">{u.user ? formatHours(u.minutes * 60) : "—"}</TD>
            <TD className="text-right tabular">{u.user ? `${u.days_used} / ${days}` : "—"}</TD>
            <LastUsed ts={u.last_used} />
          </TR>
        ))}
      </TBody>
    </Table>
  );
}

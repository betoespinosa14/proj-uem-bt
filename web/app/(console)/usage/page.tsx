"use client";

import Link from "next/link";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AppWindow, Search } from "lucide-react";
import { api, qk, type FleetAppRow, type FleetUsageFilters, type FleetUsageRow, type UsageGroup, type UsagePeriod } from "@/lib/api";
import { useDebounced } from "@/lib/hooks";
import { useMe } from "@/lib/session";
import { formatDateTime, formatHours, relativeTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Segmented } from "@/components/ui/tabs";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, ExportLink, PageHeader } from "@/components/common";

const PERIODS = { "30": "30 days", "90": "90 days", "365": "1 year" } as const;
const GROUPS = { app: "By app", app_user: "By app and user" } as const;

const ACTIVE_HINT = "In front while the user was typing or using the mouse";
const OPEN_HINT = "Running, whether or not it was being used";

export default function UsagePage() {
  const { data: me } = useMe();
  const [q, setQ] = useState("");
  const [period, setPeriod] = useState<keyof typeof PERIODS>("30");
  const [group, setGroup] = useState<UsageGroup>("app");
  const [unused, setUnused] = useState(false);
  const days = Number(period) as UsagePeriod;
  const filters: FleetUsageFilters = { app: useDebounced(q.trim()), days, unused: unused || undefined };
  const byApp = useQuery({
    queryKey: qk.usage(filters, "app"),
    queryFn: () => api.usage.byApp(filters),
    placeholderData: (p) => p,
    enabled: group === "app",
  });
  const byUser = useQuery({
    queryKey: qk.usage(filters, "app_user"),
    queryFn: () => api.usage.list(filters),
    placeholderData: (p) => p,
    enabled: group === "app_user",
  });
  const current = group === "app" ? byApp : byUser;
  const count = current.data?.items.length ?? 0;
  const off = me?.org.usage_tracking === false;

  return (
    <>
      <PageHeader
        title="App usage"
        description="How much each installed app is actively used, for license and subscription audits. Search an app to see every device it's installed on, used or not."
        actions={<ExportLink href={api.usage.exportUrl(filters, group)} />}
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
        <Segmented value={group} onChange={setGroup} options={Object.entries(GROUPS).map(([v, l]) => ({ value: v as UsageGroup, label: l }))} />
        <Segmented value={period} onChange={setPeriod} options={Object.entries(PERIODS).map(([v, l]) => ({ value: v as keyof typeof PERIODS, label: l }))} />
        <label className="flex items-center gap-2 text-sm">
          <Switch checked={unused} onCheckedChange={setUnused} aria-label="Only installed apps not used in this period" />
          Not used in {PERIODS[period]}
        </label>
      </div>
      {current.error ? (
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
              {group === "app" ? <ByAppTable items={byApp.data?.items ?? []} /> : <ByUserTable items={byUser.data?.items ?? []} days={days} />}
              {count >= 10000 && <p className="p-3 text-center text-xs text-muted-foreground">Showing the first 10,000 rows. Search an app to narrow it down.</p>}
            </div>
          )}
        </Card>
      )}
    </>
  );
}

function LastUsed({ ts }: { ts: string | null }) {
  return (
    <TD className="text-muted-foreground" title={ts ? formatDateTime(ts) : undefined}>
      {ts ? relativeTime(ts) : "Never"}
    </TD>
  );
}

function ByAppTable({ items }: { items: FleetAppRow[] }) {
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
            <TD className="font-medium">{u.app}</TD>
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

function ByUserTable({ items, days }: { items: FleetUsageRow[]; days: number }) {
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

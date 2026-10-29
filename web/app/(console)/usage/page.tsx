"use client";

import Link from "next/link";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AppWindow, Search } from "lucide-react";
import { api, qk, type FleetUsageFilters, type UsagePeriod } from "@/lib/api";
import { useDebounced } from "@/lib/hooks";
import { useMe } from "@/lib/session";
import { formatDateTime, relativeTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Segmented } from "@/components/ui/tabs";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, ExportLink, PageHeader } from "@/components/common";

const PERIODS = { "30": "30 days", "90": "90 days", "365": "1 year" } as const;

export default function UsagePage() {
  const { data: me } = useMe();
  const [q, setQ] = useState("");
  const [period, setPeriod] = useState<keyof typeof PERIODS>("30");
  const [unused, setUnused] = useState(false);
  const days = Number(period) as UsagePeriod;
  const filters: FleetUsageFilters = { app: useDebounced(q.trim()), days, unused: unused || undefined };
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.usage(filters), queryFn: () => api.usage.list(filters), placeholderData: (p) => p });
  const items = data?.items ?? [];
  const off = me?.org.usage_tracking === false;

  return (
    <>
      <PageHeader
        title="App usage"
        description="Who uses each installed app, for license and subscription audits. Search an app to see every device it's installed on, used or not."
        actions={<ExportLink href={api.usage.exportUrl(filters)} />}
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
        <Segmented value={period} onChange={setPeriod} options={Object.entries(PERIODS).map(([v, l]) => ({ value: v as keyof typeof PERIODS, label: l }))} />
        <label className="flex items-center gap-2 text-sm">
          <Switch checked={unused} onCheckedChange={setUnused} aria-label="Only installed apps not used in this period" />
          Not used in {PERIODS[period]}
        </label>
      </div>
      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={8} cols={6} />
          ) : items.length === 0 ? (
            <EmptyState
              icon={AppWindow}
              title={unused ? "Every installed third-party app was used in this period" : q ? "No matching apps" : "No app usage recorded in this period"}
            />
          ) : (
            <div className="max-h-[40rem] overflow-auto">
              <Table>
                <THead className="sticky top-0 bg-card">
                  <TR>
                    <TH>App</TH>
                    <TH>Device</TH>
                    <TH>User</TH>
                    <TH>Last used</TH>
                    <TH className="text-right">Days used</TH>
                    <TH className="text-right">Hours</TH>
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
                      <TD className="text-muted-foreground" title={u.last_used ? formatDateTime(u.last_used) : undefined}>
                        {u.last_used ? relativeTime(u.last_used) : "Never"}
                      </TD>
                      <TD className="text-right tabular">
                        {u.user ? (
                          `${u.days_used} / ${days}`
                        ) : (
                          <Badge variant="warning">Unused</Badge>
                        )}
                      </TD>
                      <TD className="text-right tabular">{u.user ? (u.minutes / 60).toFixed(1) : "—"}</TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
              {items.length >= 10000 && <p className="p-3 text-center text-xs text-muted-foreground">Showing the first 10,000 rows. Search an app to narrow it down.</p>}
            </div>
          )}
        </Card>
      )}
    </>
  );
}

"use client";

import { useQuery } from "@tanstack/react-query";
import { ChevronRight } from "lucide-react";
import { api, qk } from "@/lib/api";
import { useMe } from "@/lib/session";
import { formatHours } from "@/lib/utils";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Ranked, Trend } from "@/components/usage-dashboard";

const DAYS = 30;
const TOP = 5;

/**
 * A compact App usage summary for a device's Overview: active hours per day
 * and the top apps over the last 30 days. Hidden when there's nothing to show
 * (tracking off, no use, or Linux, which doesn't measure active time).
 */
export function DeviceUsageCard({ deviceId, os, onOpen }: { deviceId: string; os: string; onOpen: () => void }) {
  const { data: me } = useMe();
  const off = me?.org.usage_tracking === false;
  // Same filters (and so the same cached request) as the Usage tab's default dashboard.
  const filters = { app: "", days: DAYS as 30, group_id: undefined, device_id: deviceId };
  const { data } = useQuery({
    queryKey: qk.usageSummary(filters),
    queryFn: () => api.usage.summary(filters),
    enabled: !off && os !== "linux",
  });
  if (off || os === "linux" || !data || data.daily.length === 0 || data.totals.active_seconds === 0) return null;
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-4 pb-2">
        <CardTitle>App usage · last {DAYS} days</CardTitle>
        <button type="button" onClick={onOpen} className="inline-flex shrink-0 items-center gap-0.5 text-xs font-semibold text-primary hover:underline">
          See all usage <ChevronRight className="h-3.5 w-3.5" />
        </button>
      </CardHeader>
      <CardContent className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="min-w-0">
          <p className="mb-1 text-sm text-muted-foreground">
            <span className="text-lg font-semibold tabular text-foreground">{formatHours(data.totals.active_seconds)}</span> active
          </p>
          <Trend daily={data.daily} days={DAYS} device compact />
        </div>
        <div className="min-w-0">
          <p className="mb-2 text-sm font-medium">Top apps</p>
          <Ranked
            rows={data.top_apps.slice(0, TOP).map((a) => ({
              key: a.app,
              label: a.app,
              value: a.active_seconds,
              note: `${a.users} ${a.users === 1 ? "user" : "users"}`,
              onClick: onOpen,
            }))}
          />
        </div>
      </CardContent>
    </Card>
  );
}

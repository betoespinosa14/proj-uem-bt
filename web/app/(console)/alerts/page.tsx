"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, CheckCheck } from "lucide-react";
import { api, qk } from "@/lib/api";
import { useCan } from "@/lib/session";
import { formatDateTime, relativeTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Segmented } from "@/components/ui/tabs";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader, SeverityBadge } from "@/components/common";
import { toast } from "@/components/toast";

export default function AlertsPage() {
  const can = useCan();
  const qc = useQueryClient();
  const [status, setStatus] = useState<"open" | "resolved">("open");
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.alerts(status), queryFn: () => api.alerts.list(status) });
  const resolve = useMutation({
    mutationFn: (id: string) => api.alerts.resolve(id),
    onSuccess: () => {
      toast.success("Alert resolved");
      void qc.invalidateQueries({ queryKey: qk.alerts() });
      void qc.invalidateQueries({ queryKey: qk.dashboard });
    },
    onError: (e) => toast.error("Couldn't resolve alert", e),
  });

  return (
    <>
      <PageHeader
        title="Alerts"
        description="Raised by compliance rules. Alerts close automatically when the device becomes compliant again."
        actions={
          <Segmented
            value={status}
            onChange={setStatus}
            options={[
              { value: "open", label: "Open" },
              { value: "resolved", label: "Resolved" },
            ]}
          />
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={4} cols={4} />
          ) : !data?.items.length ? (
            <EmptyState icon={status === "open" ? CheckCheck : Bell} title={status === "open" ? "No open alerts" : "No resolved alerts"} description={status === "open" ? "Everything's in order." : undefined} />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>Severity</TH>
                  <TH>Alert</TH>
                  <TH className="hidden md:table-cell">Device</TH>
                  <TH>{status === "open" ? "Raised" : "Resolved"}</TH>
                  {status === "open" && can("alerts.write") && <TH className="w-24" />}
                </TR>
              </THead>
              <TBody>
                {data.items.map((a) => (
                  <TR key={a.id}>
                    <TD>
                      <SeverityBadge severity={a.severity} />
                    </TD>
                    <TD className="max-w-lg">
                      <p className="font-medium">{a.title}</p>
                      <p className="text-xs text-muted-foreground">{a.message}</p>
                    </TD>
                    <TD className="hidden md:table-cell">
                      {a.device_id ? (
                        <Link href={`/devices/${a.device_id}`} className="hover:underline">
                          {a.device_name}
                        </Link>
                      ) : (
                        "—"
                      )}
                    </TD>
                    <TD className="whitespace-nowrap text-muted-foreground" title={formatDateTime(status === "open" ? a.created_at : a.resolved_at)}>
                      {relativeTime(status === "open" ? a.created_at : a.resolved_at)}
                    </TD>
                    {status === "open" && can("alerts.write") && (
                      <TD>
                        <Button size="sm" variant="outline" loading={resolve.isPending && resolve.variables === a.id} onClick={() => resolve.mutate(a.id)}>
                          Resolve
                        </Button>
                      </TD>
                    )}
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </Card>
      )}
    </>
  );
}

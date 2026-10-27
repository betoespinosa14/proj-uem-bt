"use client";

import { Fragment, useEffect, useState } from "react";
import { useInfiniteQuery, useMutation } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, Download, ScrollText, ShieldAlert, ShieldCheck } from "lucide-react";
import { api, qk, type AuditVerify } from "@/lib/api";
import { formatDateTime, titleCase } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

const ACTION_PREFIXES = [
  { value: "auth.", label: "Sign-ins" },
  { value: "device.", label: "Devices" },
  { value: "device.command", label: "Commands" },
  { value: "compliance.", label: "Compliance" },
  { value: "user.", label: "Users" },
  { value: "enrollment_token.", label: "Enrollment" },
  { value: "script.", label: "Scripts" },
  { value: "task.", label: "Tasks" },
  { value: "group.", label: "Groups" },
  { value: "webhook.", label: "Webhooks" },
];

export default function AuditPage() {
  const [action, setAction] = useState("");
  const [actorDraft, setActorDraft] = useState("");
  const [actor, setActor] = useState("");
  const [open, setOpen] = useState<number | null>(null);
  const [verify, setVerify] = useState<AuditVerify | null>(null);

  useEffect(() => {
    const t = setTimeout(() => setActor(actorDraft.trim()), 300);
    return () => clearTimeout(t);
  }, [actorDraft]);

  const filters = { action, actor };
  const q = useInfiniteQuery({
    queryKey: qk.audit(filters),
    queryFn: ({ pageParam }) => api.audit.list({ ...filters, limit: 100, before_id: pageParam }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.next_before_id ?? undefined,
  });
  const verifyM = useMutation({
    mutationFn: api.audit.verify,
    onSuccess: (r) => {
      setVerify(r);
      if (r.valid) toast.success("Audit log verified", `${r.checked} entries, hash chain intact.`);
      else toast.error("Audit log integrity check failed", `Chain broken at entry #${r.broken_at_id}.`);
    },
    onError: (e) => toast.error("Verification failed", e),
  });
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <>
      <PageHeader
        title="Audit log"
        description="Append-only, hash-chained record of every administrative action, device command and compliance change."
        actions={
          <>
            <Button variant="outline" loading={verifyM.isPending} onClick={() => verifyM.mutate()}>
              <ShieldCheck /> Verify integrity
            </Button>
            <a
              href={api.audit.exportUrl}
              download
              className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-card px-3.5 text-sm font-medium shadow-sm transition-colors hover:bg-muted [&_svg]:size-4"
            >
              <Download /> Export CSV
            </a>
          </>
        }
      />
      {verify && (
        <div
          className={
            "mb-4 flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm " +
            (verify.valid ? "border-success/30 bg-success/5 text-success" : "border-destructive/30 bg-destructive/5 text-destructive")
          }
        >
          {verify.valid ? <ShieldCheck className="h-4 w-4" /> : <ShieldAlert className="h-4 w-4" />}
          {verify.valid
            ? `Integrity verified: ${verify.checked} entries, hash chain intact.`
            : `Tampering detected: the hash chain breaks at entry #${verify.broken_at_id}.`}
        </div>
      )}
      <div className="mb-4 flex flex-wrap gap-2">
        <Select aria-label="Action type" className="w-44" value={action} onValueChange={setAction} placeholder="All actions" options={ACTION_PREFIXES} />
        <Input aria-label="Actor email" className="w-64" placeholder="Filter by actor email" value={actorDraft} onChange={(e) => setActorDraft(e.target.value)} />
      </div>
      {q.error ? (
        <ErrorState error={q.error} onRetry={q.refetch} />
      ) : (
        <Card className="overflow-hidden">
          {q.isLoading ? (
            <TableSkeleton rows={8} cols={4} />
          ) : items.length === 0 ? (
            <EmptyState icon={ScrollText} title="No matching entries" />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH className="w-8" />
                  <TH>Time</TH>
                  <TH>Actor</TH>
                  <TH>Action</TH>
                  <TH className="hidden md:table-cell">Target</TH>
                </TR>
              </THead>
              <TBody>
                {items.map((e) => {
                  const expanded = open === e.id;
                  const name = (e.details.device_name ?? e.details.name ?? e.details.email) as string | undefined;
                  return (
                    <Fragment key={e.id}>
                      <TR className="cursor-pointer" onClick={() => setOpen(expanded ? null : e.id)}>
                        <TD>{expanded ? <ChevronDown className="h-4 w-4 text-muted-foreground" /> : <ChevronRight className="h-4 w-4 text-muted-foreground" />}</TD>
                        <TD className="whitespace-nowrap text-muted-foreground tabular">{formatDateTime(e.ts)}</TD>
                        <TD>
                          <div className="flex items-center gap-2">
                            <Badge variant={e.actor_type === "user" ? "primary" : "neutral"}>{titleCase(e.actor_type)}</Badge>
                            <span className="truncate">{e.actor_email ?? (e.actor_type === "device" ? "agent" : "")}</span>
                          </div>
                        </TD>
                        <TD className="font-mono text-xs">{e.action}</TD>
                        <TD className="hidden max-w-xs truncate text-muted-foreground md:table-cell">
                          {name ?? (e.target_type ? `${e.target_type} ${e.target_id?.slice(0, 8) ?? ""}` : "—")}
                        </TD>
                      </TR>
                      {expanded && (
                        <TR className="hover:bg-transparent">
                          <TD />
                          <TD colSpan={4}>
                            <div className="space-y-2 py-1 text-xs">
                              <div className="flex flex-wrap gap-x-6 gap-y-1 text-muted-foreground">
                                <span>Entry #{e.id}</span>
                                {e.ip && <span>IP {e.ip}</span>}
                                {e.target_id && <span>Target {e.target_type}:{e.target_id}</span>}
                                <span className="font-mono">hash {e.hash.slice(0, 16)}…</span>
                              </div>
                              <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-muted/50 p-3 font-mono">
                                {JSON.stringify(e.details, null, 2)}
                              </pre>
                            </div>
                          </TD>
                        </TR>
                      )}
                    </Fragment>
                  );
                })}
              </TBody>
            </Table>
          )}
          {q.hasNextPage && (
            <div className="border-t border-border p-3 text-center">
              <Button variant="outline" size="sm" loading={q.isFetchingNextPage} onClick={() => q.fetchNextPage()}>
                Load older entries
              </Button>
            </div>
          )}
        </Card>
      )}
    </>
  );
}

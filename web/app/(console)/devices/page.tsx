"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, Download, Monitor, Search, UserPlus } from "lucide-react";
import { api, qk, type CommandType, type DeviceFilters } from "@/lib/api";
import { useCan } from "@/lib/session";
import { formatPct, osLabel, relativeTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { Dropdown, DropdownContent, DropdownItem, DropdownTrigger } from "@/components/ui/dropdown";
import { ComplianceBadge, EmptyState, ErrorState, OsIcon, PageHeader, StatusBadge, UsageBar } from "@/components/common";
import { CommandDialog } from "@/components/commands";

const BULK_ACTIONS: { type: CommandType; label: string }[] = [
  { type: "run_script", label: "Run script…" },
  { type: "refresh_inventory", label: "Refresh inventory" },
  { type: "display_message", label: "Send message…" },
  { type: "lock", label: "Lock screen" },
  { type: "reboot", label: "Reboot…" },
];

function useDebounced<T>(value: T, ms = 250): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

export default function DevicesPage() {
  return (
    <Suspense>
      <DevicesView />
    </Suspense>
  );
}

function DevicesView() {
  const router = useRouter();
  const params = useSearchParams();
  const can = useCan();
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [os, setOs] = useState("");
  const [compliance, setCompliance] = useState(params.get("compliance") ?? "");
  const [groupId, setGroupId] = useState(params.get("group") ?? "");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [action, setAction] = useState<CommandType | null>(null);

  const filters: DeviceFilters = { q: useDebounced(q.trim()), status, os, compliance, group_id: groupId };
  const devices = useQuery({ queryKey: qk.devices(filters), queryFn: () => api.devices.list(filters), placeholderData: (p) => p });
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const items = useMemo(() => devices.data?.items ?? [], [devices.data]);

  // Drop selections that are no longer visible.
  useEffect(() => {
    setSelected((prev) => {
      const visible = new Set(items.map((d) => d.id));
      const next = new Set([...prev].filter((id) => visible.has(id)));
      return next.size === prev.size ? prev : next;
    });
  }, [items]);

  const allSelected = items.length > 0 && items.every((d) => selected.has(d.id));
  const toggleAll = () => setSelected(allSelected ? new Set() : new Set(items.map((d) => d.id)));
  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const filtered = !!(q || status || os || compliance || groupId);

  return (
    <>
      <PageHeader
        title="Devices"
        description={devices.data ? `${items.length} device${items.length === 1 ? "" : "s"}${filtered ? " match your filters" : ""}` : "All managed endpoints"}
        actions={
          <>
            <a
              href={api.devices.exportUrl(filters)}
              download
              title={filtered ? "Export the devices matching your filters" : "Export all devices"}
              className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-card px-3.5 text-sm font-medium shadow-sm transition-colors hover:bg-muted [&_svg]:size-4"
            >
              <Download /> Export CSV
            </a>
            {can("enrollment.write") && (
              <Button onClick={() => router.push("/enrollment")}>
                <UserPlus /> Enroll devices
              </Button>
            )}
          </>
        }
      />

      <div className="mb-4 flex flex-wrap items-center gap-2">
        <div className="relative min-w-[14rem] flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Search name, hostname, serial, IP, user or tag"
            className="pl-8"
            aria-label="Search devices"
          />
        </div>
        <Select
          aria-label="Status"
          className="w-36"
          value={status}
          onValueChange={setStatus}
          placeholder="Any status"
          options={[
            { value: "online", label: "Online" },
            { value: "offline", label: "Offline" },
          ]}
        />
        <Select
          aria-label="Operating system"
          className="w-36"
          value={os}
          onValueChange={setOs}
          placeholder="Any OS"
          options={["windows", "macos", "linux"].map((o) => ({ value: o, label: osLabel(o) }))}
        />
        <Select
          aria-label="Compliance"
          className="w-44"
          value={compliance}
          onValueChange={setCompliance}
          placeholder="Any compliance"
          options={[
            { value: "compliant", label: "Compliant" },
            { value: "non_compliant", label: "Non-compliant" },
            { value: "unknown", label: "Unknown" },
          ]}
        />
        <Select
          aria-label="Group"
          className="w-40"
          value={groupId}
          onValueChange={setGroupId}
          placeholder="Any group"
          options={(groups.data?.items ?? []).map((g) => ({ value: g.id, label: g.name }))}
        />
      </div>

      {selected.size > 0 && can("commands.run") && (
        <div className="mb-3 flex items-center gap-3 rounded-lg border border-primary/30 bg-primary/5 px-4 py-2 text-sm animate-fade-in">
          <span className="font-medium">{selected.size} selected</span>
          <Dropdown>
            <DropdownTrigger>
              <Button size="sm">
                Actions <ChevronDown />
              </Button>
            </DropdownTrigger>
            <DropdownContent align="start">
              {BULK_ACTIONS.map((a) => (
                <DropdownItem key={a.type} onSelect={() => setAction(a.type)} destructive={a.type === "reboot"}>
                  {a.label}
                </DropdownItem>
              ))}
            </DropdownContent>
          </Dropdown>
          <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>
            Clear
          </Button>
        </div>
      )}

      {devices.error ? (
        <ErrorState error={devices.error} onRetry={devices.refetch} />
      ) : (
        <Card className="overflow-hidden">
          {devices.isLoading ? (
            <TableSkeleton rows={6} cols={7} />
          ) : items.length === 0 ? (
            <EmptyState
              icon={Monitor}
              title={filtered ? "No devices match these filters" : "No devices enrolled yet"}
              description={filtered ? "Try clearing a filter." : "Create an enrollment token and run the installer on a device."}
              action={
                !filtered && can("enrollment.write") ? (
                  <Button variant="outline" onClick={() => router.push("/enrollment")}>
                    Go to Enrollment
                  </Button>
                ) : undefined
              }
            />
          ) : (
            <Table>
              <THead>
                <TR>
                  {can("commands.run") && (
                    <TH className="w-10">
                      <Checkbox checked={allSelected} onChange={toggleAll} aria-label="Select all devices" />
                    </TH>
                  )}
                  <TH>Device</TH>
                  <TH>Status</TH>
                  <TH>Compliance</TH>
                  <TH className="hidden md:table-cell">Group</TH>
                  <TH className="hidden lg:table-cell">CPU / Mem / Disk</TH>
                  <TH className="hidden md:table-cell">User</TH>
                  <TH>Last seen</TH>
                </TR>
              </THead>
              <TBody>
                {items.map((d) => (
                  <TR key={d.id} className="cursor-pointer" onClick={() => router.push(`/devices/${d.id}`)}>
                    {can("commands.run") && (
                      <TD onClick={(e) => e.stopPropagation()}>
                        <Checkbox checked={selected.has(d.id)} onChange={() => toggle(d.id)} aria-label={`Select ${d.name}`} />
                      </TD>
                    )}
                    <TD>
                      <div className="flex items-center gap-2.5">
                        <OsIcon os={d.os_family} />
                        <div className="min-w-0">
                          <Link
                            href={`/devices/${d.id}`}
                            onClick={(e) => e.stopPropagation()}
                            className="block truncate font-medium hover:underline"
                          >
                            {d.name}
                          </Link>
                          <p className="truncate text-xs text-muted-foreground">
                            {d.os_name} {d.os_version} · {d.ip_address || "no IP"}
                          </p>
                        </div>
                      </div>
                    </TD>
                    <TD>
                      <StatusBadge status={d.status} />
                    </TD>
                    <TD>
                      <ComplianceBadge state={d.compliance_state} />
                    </TD>
                    <TD className="hidden md:table-cell">
                      {d.group_name ?? <span className="text-muted-foreground">—</span>}
                    </TD>
                    <TD className="hidden lg:table-cell">
                      <div className="grid w-44 grid-cols-3 gap-2 text-xs tabular text-muted-foreground">
                        {[d.cpu_pct, d.mem_pct, d.disk_pct].map((v, i) => (
                          <div key={i}>
                            <span>{formatPct(v)}</span>
                            <UsageBar value={v} className="mt-1" />
                          </div>
                        ))}
                      </div>
                    </TD>
                    <TD className="hidden max-w-[10rem] truncate md:table-cell">
                      {d.logged_in_user || <span className="text-muted-foreground">—</span>}
                    </TD>
                    <TD className="whitespace-nowrap text-muted-foreground">{relativeTime(d.last_seen_at)}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </Card>
      )}

      {action && (
        <CommandDialog
          open
          onOpenChange={(o) => !o && setAction(null)}
          type={action}
          deviceIds={[...selected]}
          targetLabel={`${selected.size} device${selected.size === 1 ? "" : "s"}`}
          onIssued={() => setSelected(new Set())}
        />
      )}
    </>
  );
}

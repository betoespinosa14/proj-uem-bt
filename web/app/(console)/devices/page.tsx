"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowUpCircle, ChevronDown, Monitor, Search, UserPlus, X } from "lucide-react";
import { agentOutdated, api, qk, type CommandType, type Device, type DeviceFilters } from "@/lib/api";
import { useDebounced } from "@/lib/hooks";
import { useCan } from "@/lib/session";
import { cn, RESOURCE_LIMITS, formatDateTime, formatPct, osLabel, relativeTime, relativeTimeShort } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { TableSkeleton } from "@/components/ui/skeleton";
import { SortButton, Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { Dropdown, DropdownContent, DropdownItem, DropdownTrigger } from "@/components/ui/dropdown";
import { ComplianceBadge, EmptyState, ErrorState, ExportLink, OsIcon, PageHeader, StatusBadge, UsageBar } from "@/components/common";
import { CommandDialog, useAgentLatest } from "@/components/commands";

const LIMITS = [RESOURCE_LIMITS.cpu, RESOURCE_LIMITS.mem, RESOURCE_LIMITS.disk];

// Column sorting. The first click on a column puts the problems on top (offline,
// non-compliant, highest usage, longest unseen); a second click reverses it.
type SortKey = "name" | "status" | "compliance" | "cpu" | "mem" | "disk" | "seen";
type SortDir = "asc" | "desc";
const SORT_KEYS: SortKey[] = ["name", "status", "compliance", "cpu", "mem", "disk", "seen"];
const COMPLIANCE_RANK: Record<string, number> = { non_compliant: 2, unknown: 1, compliant: 0 };

function sortValue(d: Device, key: SortKey): number | string | null {
  switch (key) {
    case "name":
      return d.name.toLowerCase();
    case "status":
      return d.status === "offline" ? 1 : 0;
    case "compliance":
      return COMPLIANCE_RANK[d.compliance_state] ?? 1;
    case "cpu":
      return d.cpu_pct ?? null;
    case "mem":
      return d.mem_pct ?? null;
    case "disk":
      return d.disk_pct ?? null;
    case "seen":
      // Older is "bigger"; never seen is the stalest of all.
      return d.last_seen_at ? -new Date(d.last_seen_at).getTime() : Number.POSITIVE_INFINITY;
  }
}

function sortDevices(items: Device[], key: SortKey, dir: SortDir): Device[] {
  const sign = dir === "asc" ? 1 : -1;
  return [...items].sort((a, b) => {
    const va = sortValue(a, key);
    const vb = sortValue(b, key);
    if (va !== vb) {
      if (va === null) return 1; // no reading: always last
      if (vb === null) return -1;
      if (va < vb) return -sign;
      if (va > vb) return sign;
    }
    return a.name.localeCompare(b.name);
  });
}

const BULK_ACTIONS: { type: CommandType; label: string }[] = [
  { type: "run_script", label: "Run script…" },
  { type: "refresh_inventory", label: "Refresh inventory" },
  { type: "display_message", label: "Send message…" },
  { type: "lock", label: "Lock screen" },
  { type: "update_agent", label: "Update agent…" },
  { type: "reboot", label: "Reboot…" },
];

export default function DevicesPage() {
  return (
    <Suspense>
      <DevicesView />
    </Suspense>
  );
}

const POSTURE_LABELS: Record<string, string> = {
  unencrypted: "Disk not encrypted",
  firewall_off: "Firewall off",
  no_antivirus: "No antivirus",
  updates_pending: "OS updates pending",
};

function DevicesView() {
  const router = useRouter();
  const params = useSearchParams();
  const can = useCan();
  const [q, setQ] = useState(params.get("q") ?? "");
  // The top-bar search links here with ?q=; follow it when it changes on this page too.
  const urlQ = params.get("q");
  useEffect(() => {
    if (urlQ !== null) setQ(urlQ);
  }, [urlQ]);
  const [status, setStatus] = useState(params.get("status") ?? "");
  // Set by the Dashboard's Security posture links, e.g. ?posture=unencrypted.
  const [posture, setPosture] = useState(params.get("posture") ?? "");
  const [os, setOs] = useState(params.get("os") ?? "");
  const [compliance, setCompliance] = useState(params.get("compliance") ?? "");
  const [groupId, setGroupId] = useState(params.get("group") ?? "");
  const [sort, setSort] = useState<{ key: SortKey; dir: SortDir }>(() => {
    const key = params.get("sort") as SortKey | null;
    return key && SORT_KEYS.includes(key)
      ? { key, dir: params.get("dir") === "asc" ? "asc" : "desc" }
      : { key: "name", dir: "asc" };
  });
  const onSort = (key: SortKey) => {
    const next: { key: SortKey; dir: SortDir } =
      sort.key === key ? { key, dir: sort.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "name" ? "asc" : "desc" };
    setSort(next);
    // Keep the sort in the address so a shared link or Back shows the same order.
    const p = new URLSearchParams(window.location.search);
    if (next.key === "name" && next.dir === "asc") {
      p.delete("sort");
      p.delete("dir");
    } else {
      p.set("sort", next.key);
      p.set("dir", next.dir);
    }
    const qs = p.toString();
    router.replace(qs ? `?${qs}` : window.location.pathname, { scroll: false });
  };
  const ariaSort = (...keys: SortKey[]) =>
    keys.includes(sort.key) ? (sort.dir === "asc" ? "ascending" : "descending") : undefined;
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [action, setAction] = useState<CommandType | null>(null);
  // Devices for "Update agents (N)"; null means the action applies to the selection.
  const [targets, setTargets] = useState<string[] | null>(null);

  const filters: DeviceFilters = { q: useDebounced(q.trim()), status, os, compliance, group_id: groupId, posture };
  const devices = useQuery({ queryKey: qk.devices(filters), queryFn: () => api.devices.list(filters), placeholderData: (p) => p });
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const items = useMemo(() => devices.data?.items ?? [], [devices.data]);
  const rows = useMemo(() => sortDevices(items, sort.key, sort.dir), [items, sort]);
  const latestAgent = useAgentLatest().data?.version;
  const outdated = useMemo(() => items.filter((d) => agentOutdated(d.agent_version, latestAgent)).map((d) => d.id), [items, latestAgent]);

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
  const filtered = !!(q || status || os || compliance || groupId || posture);

  return (
    <>
      <PageHeader
        title="Devices"
        description={devices.data ? `${items.length} device${items.length === 1 ? "" : "s"}${filtered ? " match your filters" : ""}` : "All managed endpoints"}
        actions={
          <>
            {can("commands.run") && outdated.length > 0 && (
              <Button
                variant="outline"
                title={`Agent ${latestAgent} is available`}
                onClick={() => {
                  setTargets(outdated);
                  setAction("update_agent");
                }}
              >
                <ArrowUpCircle /> Update agents ({outdated.length})
              </Button>
            )}
            <ExportLink href={api.devices.exportUrl(filters)} title={filtered ? "Export the devices matching your filters" : "Export all devices"} />
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
        {posture && (
          <button
            type="button"
            onClick={() => setPosture("")}
            className="inline-flex h-9 items-center gap-1.5 rounded-full border border-nav-accent/40 bg-nav-accent/15 px-3 text-sm font-medium text-nav-accent"
            title="Clear this filter"
          >
            {POSTURE_LABELS[posture] ?? posture} <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>

      {selected.size > 0 && can("commands.run") && (
        <div className="mb-3 flex items-center gap-3 rounded-lg border border-primary/30 bg-primary/5 px-4 py-2 text-sm animate-fade-in">
          <span className="font-medium">{allSelected ? `All ${items.length} shown selected` : `${selected.size} selected`}</span>
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
        <Card className="overflow-clip">
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
            // The optional columns follow the table's own width (see .col-res / .col-agent
            // in globals.css), not the screen's: the page content is capped at max-w-7xl.
            <Table containerClassName="table-cq" stickyHeader>
              <THead>
                <TR>
                  {can("commands.run") && (
                    <TH className="w-10">
                      <Checkbox checked={allSelected} onChange={toggleAll} aria-label="Select all devices" />
                    </TH>
                  )}
                  <TH aria-sort={ariaSort("name")}>
                    <SortButton label="Device" k="name" sort={sort} onSort={onSort} />
                  </TH>
                  <TH className="hidden sm:table-cell" aria-sort={ariaSort("status")}>
                    <SortButton label="Status" k="status" sort={sort} onSort={onSort} />
                  </TH>
                  <TH className="hidden sm:table-cell" aria-sort={ariaSort("compliance")}>
                    <SortButton label="Compliance" k="compliance" sort={sort} onSort={onSort} />
                  </TH>
                  <TH className="col-res" aria-sort={ariaSort("cpu", "mem", "disk")}>
                    {/* Same grid as the cells below, so each label sits over its own number. */}
                    <div className="grid w-44 grid-cols-3 gap-2">
                      <SortButton label="CPU" k="cpu" sort={sort} onSort={onSort} />
                      <SortButton label="Mem" k="mem" sort={sort} onSort={onSort} />
                      <SortButton label="Disk" k="disk" sort={sort} onSort={onSort} />
                    </div>
                  </TH>
                  <TH className="col-agent">Agent</TH>
                  <TH className="hidden pr-5 sm:table-cell" aria-sort={ariaSort("seen")}>
                    <SortButton label="Last seen" k="seen" sort={sort} onSort={onSort} />
                  </TH>
                </TR>
              </THead>
              <TBody>
                {rows.map((d) => (
                  <TR key={d.id} className="cursor-pointer" onClick={() => router.push(`/devices/${d.id}`)}>
                    {can("commands.run") && (
                      <TD onClick={(e) => e.stopPropagation()}>
                        <Checkbox checked={selected.has(d.id)} onChange={() => toggle(d.id)} aria-label={`Select ${d.name}`} />
                      </TD>
                    )}
                    <TD>
                      <div className="flex max-w-[22rem] items-center gap-2.5">
                        <OsIcon os={d.os_family} />
                        <div className="min-w-0">
                          <Link
                            href={`/devices/${d.id}`}
                            onClick={(e) => e.stopPropagation()}
                            className="block truncate font-medium hover:underline"
                            title={[d.name, d.logged_in_user && `Signed in: ${d.logged_in_user}`].filter(Boolean).join("\n")}
                          >
                            {d.name}
                          </Link>
                          <p className="truncate text-xs text-muted-foreground" title={`${d.os_name} ${d.os_version} · ${d.ip_address || "no IP"}`}>
                            {d.os_name} {d.os_version} · {d.ip_address || "no IP"}
                          </p>
                          {/* On phones the status columns are hidden; show them here instead. */}
                          <div className="mt-1.5 flex flex-wrap items-center gap-1.5 sm:hidden">
                            <StatusBadge status={d.status} />
                            <ComplianceBadge state={d.compliance_state} />
                            <span className="text-xs text-muted-foreground">{relativeTime(d.last_seen_at)}</span>
                          </div>
                        </div>
                      </div>
                    </TD>
                    <TD className="hidden sm:table-cell">
                      <StatusBadge status={d.status} />
                    </TD>
                    <TD className="hidden sm:table-cell">
                      <ComplianceBadge state={d.compliance_state} />
                    </TD>
                    <TD className="col-res">
                      <div className="grid w-44 grid-cols-3 gap-2 text-xs tabular text-muted-foreground">
                        {LIMITS.map((limit, i) => {
                          const v = [d.cpu_pct, d.mem_pct, d.disk_pct][i];
                          return (
                            <div key={i}>
                              <span className={cn(v != null && v >= limit.alertAt && "font-medium text-foreground")}>{formatPct(v)}</span>
                              <UsageBar value={v} className="mt-1" quiet {...limit} />
                            </div>
                          );
                        })}
                      </div>
                    </TD>
                    <TD className="col-agent whitespace-nowrap">
                      <span className="inline-block max-w-[8.5rem] truncate align-middle font-mono text-xs text-muted-foreground" title={d.agent_version}>
                        {d.agent_version || "—"}
                      </span>
                      {agentOutdated(d.agent_version, latestAgent) && (
                        <span className="ml-1.5 inline-flex items-center gap-1 text-xs font-medium text-foreground" title={`Agent ${latestAgent} is available`}>
                          <span className="h-1.5 w-1.5 rounded-full bg-warning" aria-hidden />
                          Update
                        </span>
                      )}
                    </TD>
                    <TD
                      className="hidden whitespace-nowrap pr-5 text-muted-foreground sm:table-cell"
                      title={d.last_seen_at ? formatDateTime(d.last_seen_at) : undefined}
                    >
                      {relativeTimeShort(d.last_seen_at)}
                    </TD>
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
          onOpenChange={(o) => {
            if (!o) {
              setAction(null);
              setTargets(null);
            }
          }}
          type={action}
          deviceIds={targets ?? [...selected]}
          offlineCount={(() => {
            const ids = new Set(targets ?? selected);
            return items.filter((d) => ids.has(d.id) && d.status !== "online").length;
          })()}
          // Scripts and the default shell follow the OS when every target shares one.
          os={(() => {
            const ids = new Set(targets ?? selected);
            const oses = new Set(items.filter((d) => ids.has(d.id)).map((d) => d.os_family));
            return oses.size === 1 ? [...oses][0] : undefined;
          })()}
          targetLabel={
            targets
              ? `${targets.length} device${targets.length === 1 ? "" : "s"} on an older agent (latest ${latestAgent})`
              : `${selected.size} device${selected.size === 1 ? "" : "s"}`
          }
          onIssued={() => (targets ? setTargets(null) : setSelected(new Set()))}
        />
      )}
    </>
  );
}

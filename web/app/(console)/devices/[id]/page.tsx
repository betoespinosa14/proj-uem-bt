"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import {
  ArrowLeft,
  ChevronDown,
  ChevronRight,
  Clock,
  Cpu,
  HardDrive,
  MemoryStick,
  MoreHorizontal,
  Pencil,
  BatteryMedium,
  Play,
  RefreshCw,
  Search,
  ShieldAlert,
  ShieldCheck,
  Terminal,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { agentOutdated, api, qk, type Command, type CommandType, type DeviceDetail, type Disk, type Inventory } from "@/lib/api";
import { useCan } from "@/lib/session";
import { cn, formatBytes, formatDate, formatDateTime, formatDuration, formatPct, osLabel, relativeTime, titleCase } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Dropdown, DropdownContent, DropdownItem, DropdownSeparator, DropdownTrigger } from "@/components/ui/dropdown";
import { Field, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { Segmented, Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  CommandStatusBadge,
  ComplianceBadge,
  CopyValue,
  EmptyState,
  ErrorState,
  ExportLink,
  HeadlineStats,
  KeyValues,
  OsIcon,
  SeverityBadge,
  StatusBadge,
  TagInput,
  UsageBar,
} from "@/components/common";
import { CommandDialog, CommandOutput, commandLabel, useAgentLatest } from "@/components/commands";
import { toast } from "@/components/toast";
import { UsageExplorer } from "@/components/usage-explorer";
import { DeviceUsageCard } from "@/components/device-usage-card";

export default function DevicePage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const qc = useQueryClient();
  const can = useCan();
  const [tab, setTab] = useState("overview");
  const [action, setAction] = useState<CommandType | null>(null);
  const [editing, setEditing] = useState(false);
  const [retiring, setRetiring] = useState(false);

  const { data: d, isLoading, error, refetch } = useQuery({ queryKey: qk.device(id), queryFn: () => api.devices.get(id) });
  const latestAgent = useAgentLatest().data?.version;

  // Name the browser tab after the device, so several open devices can be told apart.
  useEffect(() => {
    if (!d?.name) return;
    const previous = document.title;
    document.title = `${d.name} · BEACON`;
    return () => {
      document.title = previous;
    };
  }, [d?.name]);

  // Once the device header scrolls under the top bar, show a compact name in the sticky tab row.
  const [headerEl, setHeaderEl] = useState<HTMLDivElement | null>(null);
  const [compact, setCompact] = useState(false);
  useEffect(() => {
    if (!headerEl) return;
    const io = new IntersectionObserver(([e]) => setCompact(!e!.isIntersecting), { rootMargin: "-64px 0px 0px 0px" });
    io.observe(headerEl);
    return () => io.disconnect();
  }, [headerEl]);

  const retire = useMutation({
    mutationFn: () => api.devices.retire(id),
    onSuccess: () => {
      toast.success("Device retired");
      void qc.invalidateQueries({ queryKey: qk.devices() });
      router.replace("/devices");
    },
    onError: (e) => toast.error("Couldn't retire device", e),
  });

  if (error) return <ErrorState error={error} onRetry={refetch} />;
  if (isLoading || !d) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-6 w-32" />
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-80 w-full" />
      </div>
    );
  }

  const quick: { type: CommandType; label: string; icon: typeof Play }[] = [
    { type: "run_script", label: "Run script", icon: Terminal },
    { type: "refresh_inventory", label: "Refresh", icon: RefreshCw },
  ];

  return (
    <>
      <Link href="/devices" className="mb-4 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ArrowLeft className="h-4 w-4" /> Devices
      </Link>

      <div ref={setHeaderEl} className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 items-start gap-3">
          <span className="mt-0.5 rounded-lg border border-border bg-card p-2.5">
            <OsIcon os={d.os_family} className="h-5 w-5" />
          </span>
          <div className="min-w-0">
            <h1 className="truncate text-xl font-semibold tracking-tight">{d.name}</h1>
            <div className="mt-1.5 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
              <StatusBadge status={d.status} />
              <ComplianceBadge state={d.compliance_state} />
              <span className="min-w-0 basis-full sm:basis-auto">
                {d.os_name || osLabel(d.os_family)} {d.os_version} · Last seen {relativeTime(d.last_seen_at)}
              </span>
              {d.tags.map((t) => {
                const rules = d.rule_tags?.[t];
                return rules?.length ? (
                  <Badge
                    key={t}
                    variant="outline"
                    className="gap-1 font-mono"
                    title={`Added by the compliance rule ${rules.map((r) => `"${r}"`).join(" and ")}. Removed automatically when the device passes it.`}
                  >
                    <ShieldAlert className="h-3 w-3" aria-label="Added by a compliance rule" />
                    {t}
                  </Badge>
                ) : (
                  <Badge key={t} variant="outline" className="font-mono">
                    {t}
                  </Badge>
                );
              })}
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {can("commands.run") &&
            quick.map((q) => (
              <Button key={q.type} variant="outline" onClick={() => setAction(q.type)}>
                <q.icon /> {q.label}
              </Button>
            ))}
          {(can("commands.run") || can("devices.write")) && (
            <Dropdown>
              <DropdownTrigger>
                <Button variant="outline" size="icon" aria-label="More actions">
                  <MoreHorizontal />
                </Button>
              </DropdownTrigger>
              <DropdownContent>
                {can("commands.run") && (
                  <>
                    <DropdownItem onSelect={() => setAction("display_message")}>Send message…</DropdownItem>
                    <DropdownItem onSelect={() => setAction("lock")}>Lock screen</DropdownItem>
                    <DropdownItem onSelect={() => setAction("reboot")}>Reboot…</DropdownItem>
                    <DropdownItem onSelect={() => setAction("shutdown")}>Shut down…</DropdownItem>
                  </>
                )}
                {can("devices.write") && (
                  <>
                    <DropdownSeparator />
                    <DropdownItem onSelect={() => setEditing(true)}>
                      <Pencil className="h-4 w-4" /> Edit name, group & tags
                    </DropdownItem>
                  </>
                )}
                {can("commands.run") && agentOutdated(d.agent_version, latestAgent) && (
                  <DropdownItem onSelect={() => setAction("update_agent")}>Update agent to {latestAgent}…</DropdownItem>
                )}
                {can("commands.run") && <DropdownItem onSelect={() => setAction("uninstall_agent")} destructive>Uninstall agent…</DropdownItem>}
                {can("devices.write") && (
                  <DropdownItem onSelect={() => setRetiring(true)} destructive>
                    <Trash2 className="h-4 w-4" /> Retire device…
                  </DropdownItem>
                )}
              </DropdownContent>
            </Dropdown>
          )}
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        {/* Sticky under the 4rem top bar, edge to edge on the same background so nothing shows through. */}
        <div className="sticky top-16 z-10 -mx-4 -my-2 flex items-center gap-3 bg-[hsl(var(--canvas-from))] px-4 py-2 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
          {compact && (
            <span className="flex min-w-0 max-w-[16rem] shrink items-center gap-2 text-sm font-semibold motion-safe:animate-fade-in">
              <span
                className={cn("h-2 w-2 shrink-0 rounded-full", d.status === "online" ? "bg-success" : "bg-muted-foreground/60")}
                aria-label={d.status === "online" ? "Online" : "Offline"}
              />
              <span className="truncate" title={d.name}>
                {d.name}
              </span>
            </span>
          )}
          <TabsList className="min-w-0">
            {["overview", "hardware", "software", "usage", "network", "security", "compliance", "telemetry", "commands"].map((t) => (
              <TabsTrigger key={t} value={t}>
                {titleCase(t)}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        <TabsContent value="overview">
          <Overview d={d} onUpdate={can("commands.run") ? () => setAction("update_agent") : undefined} onOpenUsage={() => setTab("usage")} />
        </TabsContent>
        <TabsContent value="hardware">
          <Hardware inv={d.inventory} />
        </TabsContent>
        <TabsContent value="software">
          <Software id={id} />
        </TabsContent>
        <TabsContent value="usage">
          <Usage id={id} os={d.os_family} />
        </TabsContent>
        <TabsContent value="network">
          <Network inv={d.inventory} />
        </TabsContent>
        <TabsContent value="security">
          <Security inv={d.inventory} />
        </TabsContent>
        <TabsContent value="compliance">
          <Compliance id={id} />
        </TabsContent>
        <TabsContent value="telemetry">
          <Telemetry id={id} />
        </TabsContent>
        <TabsContent value="commands">
          <Commands id={id} onRun={can("commands.run") ? () => setAction("run_script") : undefined} />
        </TabsContent>
      </Tabs>

      {action && (
        <CommandDialog
          open
          onOpenChange={(o) => !o && setAction(null)}
          type={action}
          deviceIds={[id]}
          targetLabel={d.name}
          onIssued={() => setTab("commands")}
        />
      )}
      {editing && <EditDevice d={d} onClose={() => setEditing(false)} />}
      <ConfirmDialog
        open={retiring}
        onOpenChange={setRetiring}
        title={`Retire ${d.name}?`}
        description="The device's credential is revoked immediately and it disappears from the fleet. History stays in the audit log."
        confirmLabel="Retire device"
        loading={retire.isPending}
        onConfirm={() => retire.mutate()}
      />
    </>
  );
}

function NoInventory() {
  return (
    <Card>
      <EmptyState icon={RefreshCw} title="No inventory yet" description="The agent uploads a full inventory shortly after it connects." />
    </Card>
  );
}

function Metric({
  icon: Icon,
  label,
  value,
  text,
  sub,
  charge,
}: {
  icon: typeof Cpu;
  label: string;
  value?: number | null;
  /** Shown instead of a percentage, with no bar (e.g. uptime). */
  text?: string;
  sub?: string;
  /** A charge level (battery): low is the warning, not high. */
  charge?: boolean;
}) {
  if (charge && value != null) value = Math.min(100, value);
  return (
    <Card className="p-4">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Icon className="h-4 w-4" /> {label}
      </div>
      <p className="mt-2 text-2xl font-semibold tabular">{text ?? formatPct(value)}</p>
      {text === undefined && <UsageBar value={value} className="mt-2" charge={charge} />}
      {sub && <p className="mt-1.5 text-xs text-muted-foreground">{sub}</p>}
    </Card>
  );
}

/** The volume the agent reports "System disk" % for: C: on Windows, / elsewhere. */
function systemDisk(inv: Inventory | null, os: string): Disk | undefined {
  const disks = inv?.hardware.disks ?? [];
  const norm = (m: string) => m.replace(/[\\/]+$/, "").toUpperCase();
  const want = os === "windows" ? "C:" : "";
  return (
    disks.find((d) => norm(d.mount) === want) ??
    (os !== "windows" ? disks.find((d) => d.mount === "/") : undefined) ??
    [...disks].sort((a, b) => b.total_bytes - a.total_bytes)[0]
  );
}

const BATTERY_STATE: Record<string, string> = { charging: "Charging", discharging: "On battery", full: "Plugged in" };

function batterySub(inv: Inventory | null): string | undefined {
  const b = inv?.hardware.battery;
  if (!b?.present) return undefined;
  const parts: string[] = [];
  if (b.health_pct != null) parts.push(`Health ${formatPct(b.health_pct)}`);
  // The state comes with the inventory (every few hours); only show it while recent.
  const fresh = inv && Date.now() - new Date(inv.collected_at).getTime() < 3600_000;
  if (fresh && BATTERY_STATE[b.state]) parts.push(BATTERY_STATE[b.state]!);
  return parts.join(" · ") || undefined;
}

function Overview({ d, onUpdate, onOpenUsage }: { d: DeviceDetail; onUpdate?: () => void; onOpenUsage: () => void }) {
  const latest = useAgentLatest().data?.version;
  const behind = agentOutdated(d.agent_version, latest);
  const inv = d.inventory;
  const sysDisk = systemDisk(inv, d.os_family);
  // Uptime at the last inventory plus the time since; a reboot sends a fresh inventory.
  const uptime = inv ? inv.os.uptime_seconds + Math.max(0, (Date.now() - new Date(inv.collected_at).getTime()) / 1000) : null;
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Metric icon={Cpu} label="CPU" value={d.cpu_pct} sub={inv?.hardware.cpu_model} />
        <Metric
          icon={MemoryStick}
          label="Memory"
          value={d.mem_pct}
          sub={inv ? `${formatBytes(inv.hardware.memory_total_bytes)} installed` : undefined}
        />
        <Metric
          icon={HardDrive}
          label="System disk"
          value={d.disk_pct}
          sub={sysDisk ? `${formatBytes(sysDisk.used_bytes)} of ${formatBytes(sysDisk.total_bytes)} used` : undefined}
        />
        {d.battery_pct != null || inv?.hardware.battery.present ? (
          <Metric icon={BatteryMedium} label="Battery" charge value={d.battery_pct} sub={batterySub(inv)} />
        ) : (
          // Desktops and servers have no battery: show how long they've been up instead.
          <Metric
            icon={Clock}
            label="Uptime"
            text={d.status === "online" && uptime != null ? formatDuration(uptime) : d.status === "online" ? "—" : "Offline"}
            sub={d.status === "online" && uptime != null ? `Since ${formatDateTime(new Date(Date.now() - uptime * 1000).toISOString())}` : undefined}
          />
        )}
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Device</CardTitle>
          </CardHeader>
          <CardContent>
            <KeyValues
              items={[
                ["Hostname", d.hostname && <CopyValue key="h" value={d.hostname} label="hostname" />],
                ["Manufacturer", d.manufacturer],
                ["Model", d.model],
                ["Serial number", d.serial && <CopyValue key="s" value={d.serial} label="serial number" mono />],
                ["Group", d.group_name],
                ["Employee", d.employee_name || "—"],
                ["Logged-in user", d.logged_in_user],
                ["Enrolled", formatDateTime(d.enrolled_at)],
              ]}
            />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>System</CardTitle>
          </CardHeader>
          <CardContent>
            <KeyValues
              items={[
                ["Operating system", `${d.os_name} ${d.os_version}`.trim()],
                ["Build", <span key="b" className="font-mono">{d.os_build}</span>],
                ["Architecture", d.arch],
                ["Uptime", d.status !== "online" ? "Offline" : uptime != null ? formatDuration(uptime) : ""],
                ["IP address", d.ip_address && <CopyValue key="ip" value={d.ip_address} label="IP address" mono />],
                ["MAC address", d.mac_address && <CopyValue key="mac" value={d.mac_address} label="MAC address" mono />],
                [
                  "Agent version",
                  <span key="av" className="inline-flex flex-wrap items-center gap-2">
                    {d.agent_version}
                    {behind &&
                      (onUpdate ? (
                        <button type="button" onClick={onUpdate} className="rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50">
                          <Badge variant="warning" title="Update this device's agent">Update available · {latest}</Badge>
                        </button>
                      ) : (
                        <Badge variant="warning">Update available · {latest}</Badge>
                      ))}
                  </span>,
                ],
                ["Inventory collected", inv ? relativeTime(inv.collected_at) : "Never"],
              ]}
            />
          </CardContent>
        </Card>
      </div>
      <DeviceUsageCard deviceId={d.id} os={d.os_family} onOpen={onOpenUsage} />
    </div>
  );
}

function Hardware({ inv }: { inv: Inventory | null }) {
  if (!inv) return <NoInventory />;
  const hw = inv.hardware;
  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Processor & memory</CardTitle>
        </CardHeader>
        <CardContent>
          <KeyValues
            items={[
              ["CPU", hw.cpu_model],
              ["Cores / threads", `${hw.cpu_cores} / ${hw.cpu_threads}`],
              ["Memory", `${formatBytes(hw.memory_used_bytes)} used of ${formatBytes(hw.memory_total_bytes)}`],
              ["Manufacturer", hw.manufacturer],
              ["Model", hw.model],
              ["Serial", hw.serial],
            ]}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>Battery</CardTitle>
        </CardHeader>
        <CardContent>
          {hw.battery.present ? (
            <KeyValues
              items={[
                ["Charge", formatPct(hw.battery.percent)],
                ["State", titleCase(hw.battery.state)],
                ["Health", hw.battery.health_pct == null ? "Unknown" : formatPct(hw.battery.health_pct)],
              ]}
            />
          ) : (
            <p className="text-sm text-muted-foreground">No battery detected.</p>
          )}
        </CardContent>
      </Card>
      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle>Storage</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {hw.disks.length === 0 && <p className="text-sm text-muted-foreground">No volumes reported.</p>}
          {hw.disks.map((disk) => {
            const pct = disk.total_bytes ? (disk.used_bytes / disk.total_bytes) * 100 : 0;
            return (
              <div key={disk.mount}>
                <div className="mb-1.5 flex items-baseline justify-between gap-4 text-sm">
                  <span className="truncate font-mono">
                    {disk.mount} <span className="text-xs text-muted-foreground">{disk.fstype}</span>
                  </span>
                  <span className="shrink-0 text-xs text-muted-foreground tabular">
                    {formatBytes(disk.used_bytes)} / {formatBytes(disk.total_bytes)} · {formatBytes(disk.free_bytes)} free
                  </span>
                </div>
                <UsageBar value={pct} className="h-2" />
              </div>
            );
          })}
        </CardContent>
      </Card>
    </div>
  );
}

function Software({ id }: { id: string }) {
  const [q, setQ] = useState("");
  const [category, setCategory] = useState<"all" | "system" | "third_party" | "flagged">("all");
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.deviceSoftware(id), queryFn: () => api.devices.software(id) });
  const all = useMemo(() => data?.items ?? [], [data]);
  const counts = useMemo(() => ({ system: all.filter((s) => s.category === "system").length, third_party: all.filter((s) => s.category === "third_party").length }), [all]);
  const gambling = all.filter((s) => s.flag === "gambling");
  const games = all.filter((s) => s.flag === "game");
  const items = useMemo(() => {
    const needle = q.trim().toLowerCase();
    const inCategory = (s: (typeof all)[number]) => category === "all" || (category === "flagged" ? !!s.flag : s.category === category);
    return all.filter((s) => inCategory(s) && (!needle || `${s.name} ${s.publisher}`.toLowerCase().includes(needle)));
  }, [all, q, category]);
  if (error) return <ErrorState error={error} onRetry={refetch} />;
  return (
    <Card className="overflow-hidden">
      {(gambling.length > 0 || games.length > 0) && (
        <div role="alert" className="flex items-start gap-3 border-b border-warning/25 bg-warning/10 p-3 text-sm">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
          <div className="space-y-0.5">
            {gambling.length > 0 && (
              <p>
                <span className="font-medium">Gambling apps installed ({gambling.length}):</span> {gambling.map((s) => s.name).join(", ")}
              </p>
            )}
            {games.length > 0 && (
              <p>
                <span className="font-medium">Games installed ({games.length}):</span> {games.map((s) => s.name).join(", ")}
              </p>
            )}
          </div>
        </div>
      )}
      <div className="flex flex-wrap items-center gap-3 border-b border-border p-3">
        <div className="relative max-w-sm flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Filter software" className="pl-8" aria-label="Filter software" />
        </div>
        <Segmented
          value={category}
          onChange={setCategory}
          options={[
            { value: "all", label: `All (${all.length})` },
            { value: "system", label: `System (${counts.system})` },
            { value: "third_party", label: `Third-party (${counts.third_party})` },
            ...(gambling.length + games.length > 0 ? [{ value: "flagged" as const, label: `Flagged (${gambling.length + games.length})` }] : []),
          ]}
        />
        <span className="text-sm text-muted-foreground tabular">
          {items.length}
          {items.length !== all.length ? ` of ${all.length}` : ""} applications
        </span>
        <ExportLink
          href={api.devices.softwareExportUrl(id, { q: q.trim(), category: category === "all" ? "" : category })}
          title={items.length !== all.length ? "Export the applications matching your filters" : "Export all applications"}
          className="ml-auto"
        />
      </div>
      {isLoading ? (
        <TableSkeleton rows={8} cols={6} />
      ) : items.length === 0 ? (
        <EmptyState icon={Search} title={q || category !== "all" ? "No matching software" : "No software reported"} />
      ) : (
        <div className="max-h-[36rem] overflow-auto">
          <Table>
            <THead className="sticky top-0 bg-card">
              <TR>
                <TH>Name</TH>
                <TH>Version</TH>
                <TH>Type</TH>
                <TH className="hidden md:table-cell">Publisher</TH>
                <TH className="hidden sm:table-cell">Source</TH>
                <TH className="hidden lg:table-cell">Installed</TH>
                <TH className="hidden lg:table-cell">Last used</TH>
              </TR>
            </THead>
            <TBody>
              {items.slice(0, 1000).map((s, i) => (
                <TR key={`${s.name}-${s.version}-${i}`}>
                  <TD className="font-medium">
                    {s.name}
                    {s.flag && (
                      <Badge variant={s.flag === "gambling" ? "danger" : "warning"} className="ml-2">
                        {s.flag === "gambling" ? "Gambling" : "Game"}
                      </Badge>
                    )}
                  </TD>
                  <TD className="font-mono text-xs">{s.version}</TD>
                  <TD>
                    <Badge variant={s.category === "system" ? "primary" : "outline"}>{s.category === "system" ? "System" : "Third-party"}</Badge>
                  </TD>
                  <TD className="hidden max-w-[16rem] truncate text-muted-foreground md:table-cell">{s.publisher}</TD>
                  <TD className="hidden sm:table-cell">
                    <Badge variant="neutral">{s.source === "msstore" ? "Microsoft Store" : s.source}</Badge>
                  </TD>
                  <TD className="hidden text-muted-foreground lg:table-cell">{s.install_date ? formatDate(s.install_date) : "—"}</TD>
                  <TD className="hidden text-muted-foreground lg:table-cell" title={s.last_used ? formatDateTime(s.last_used) : undefined}>
                    {s.last_used ? relativeTime(s.last_used) : "—"}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
          {items.length > 1000 && <p className="p-3 text-center text-xs text-muted-foreground">Showing the first 1,000. Refine the filter to see more.</p>}
        </div>
      )}
    </Card>
  );
}

function Usage({ id, os }: { id: string; os: string }) {
  return (
    <UsageExplorer
      deviceId={id}
      deviceOs={os}
      header={({ stats, actions }) => (
        <div className="mb-5 flex flex-wrap items-center justify-between gap-x-8 gap-y-4">
          {stats?.length ? <HeadlineStats stats={stats} /> : <span />}
          <div className="flex flex-wrap items-center gap-2">{actions}</div>
        </div>
      )}
    />
  );
}

function Network({ inv }: { inv: Inventory | null }) {
  if (!inv) return <NoInventory />;
  const n = inv.network;
  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="pt-5">
          <KeyValues
            items={[
              ["Hostname", n.hostname],
              ["Primary IP", <span key="ip" className="font-mono">{n.primary_ip}</span>],
              ["Primary MAC", <span key="mac" className="font-mono">{n.primary_mac}</span>],
              ["Wi-Fi network", n.wifi_ssid ?? "Not connected / unavailable"],
            ]}
          />
        </CardContent>
      </Card>
      <Card className="overflow-hidden">
        <Table>
          <THead>
            <TR>
              <TH>Interface</TH>
              <TH>State</TH>
              <TH>MAC</TH>
              <TH>Addresses</TH>
            </TR>
          </THead>
          <TBody>
            {n.interfaces.map((i) => (
              <TR key={i.name}>
                <TD className="font-medium">{i.name}</TD>
                <TD>
                  <Badge variant={i.up ? "success" : "neutral"} dot>
                    {i.up ? "Up" : "Down"}
                  </Badge>
                </TD>
                <TD className="font-mono text-xs">{i.mac || "—"}</TD>
                <TD className="font-mono text-xs">{i.addrs.length ? i.addrs.join(", ") : "—"}</TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </Card>
    </div>
  );
}

function Posture({ label, value, good, detail }: { label: string; value: boolean | null; good: boolean; detail?: string | null }) {
  const variant = value == null ? "warning" : value === good ? "success" : "danger";
  const text = value == null ? "Unknown" : value ? "Enabled" : "Disabled";
  return (
    <div className="flex items-center justify-between gap-4 py-3">
      <div>
        <p className="text-sm font-medium">{label}</p>
        {detail && <p className="text-xs text-muted-foreground">{detail}</p>}
      </div>
      <Badge variant={variant}>{text}</Badge>
    </div>
  );
}

function Security({ inv }: { inv: Inventory | null }) {
  if (!inv) return <NoInventory />;
  const s = inv.security;
  return (
    <Card>
      <CardContent className="divide-y divide-border pt-2">
        <Posture label="Disk encryption" value={s.disk_encrypted} good detail={s.encryption_method} />
        <Posture label="Firewall" value={s.firewall_enabled} good />
        <Posture label="Screen lock policy" value={s.screen_lock_enabled} good />
        <div className="flex items-center justify-between gap-4 py-3">
          <p className="text-sm font-medium">Pending OS updates</p>
          <Badge variant={s.pending_updates == null ? "warning" : s.pending_updates === 0 ? "success" : "danger"}>
            {s.pending_updates == null ? "Unknown" : s.pending_updates === 0 ? "Up to date" : `${s.pending_updates} pending`}
          </Badge>
        </div>
        <div className="flex items-center justify-between gap-4 py-3">
          <p className="text-sm font-medium">Antivirus</p>
          <span className="text-sm text-muted-foreground">{s.antivirus ?? "Not reported"}</span>
        </div>
      </CardContent>
    </Card>
  );
}

function Compliance({ id }: { id: string }) {
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.deviceCompliance(id), queryFn: () => api.devices.compliance(id) });
  if (error) return <ErrorState error={error} onRetry={refetch} />;
  if (isLoading) return <Skeleton className="h-48" />;
  if (!data?.items.length)
    return (
      <Card>
        <EmptyState
          icon={ShieldCheck}
          title="No compliance rules apply"
          description="Create rules on the Compliance page. They're evaluated whenever the device reports inventory."
        />
      </Card>
    );
  return (
    <Card className="overflow-hidden">
      <Table>
        <THead>
          <TR>
            <TH>Rule</TH>
            <TH>Severity</TH>
            <TH>Result</TH>
            <TH className="hidden md:table-cell">Details</TH>
            <TH className="hidden sm:table-cell">Evaluated</TH>
          </TR>
        </THead>
        <TBody>
          {data.items.map((r) => (
            <TR key={r.rule_id}>
              <TD className="font-medium">{r.rule_name}</TD>
              <TD>
                <SeverityBadge severity={r.severity} />
              </TD>
              <TD>
                <Badge variant={r.passed ? "success" : "danger"}>{r.passed ? "Pass" : "Fail"}</Badge>
              </TD>
              <TD className="hidden max-w-md text-muted-foreground md:table-cell">{r.details}</TD>
              <TD className="hidden whitespace-nowrap text-muted-foreground sm:table-cell">{relativeTime(r.evaluated_at)}</TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </Card>
  );
}

const RANGES = { "1": "1h", "24": "24h", "168": "7d" } as const;

function Telemetry({ id }: { id: string }) {
  const [hours, setHours] = useState<keyof typeof RANGES>("24");
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: qk.deviceTelemetry(id, Number(hours)),
    queryFn: () => api.devices.telemetry(id, Number(hours)),
    refetchInterval: 60_000,
  });
  const points = useMemo(
    () =>
      (data?.items ?? []).map((p) => ({
        t: new Date(p.ts).getTime(),
        CPU: p.cpu_pct,
        Memory: p.mem_pct,
        Disk: p.disk_pct,
        Battery: p.battery_pct,
      })),
    [data],
  );
  const hasBattery = points.some((p) => p.Battery != null);
  const fmt = (t: number) =>
    new Date(t).toLocaleString(undefined, hours === "168" ? { month: "short", day: "numeric" } : { hour: "2-digit", minute: "2-digit" });
  if (error) return <ErrorState error={error} onRetry={refetch} />;
  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between">
        <CardTitle>Utilization</CardTitle>
        <Segmented value={hours} onChange={setHours} options={Object.entries(RANGES).map(([v, l]) => ({ value: v as keyof typeof RANGES, label: l }))} />
      </CardHeader>
      <CardContent className="h-80">
        {isLoading ? (
          <Skeleton className="h-full" />
        ) : points.length < 2 ? (
          <div className="flex h-full items-center justify-center text-sm text-muted-foreground">Not enough samples yet. The agent reports every minute.</div>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={points} margin={{ left: -16, right: 8 }}>
              <CartesianGrid stroke="hsl(var(--border))" vertical={false} />
              <XAxis dataKey="t" type="number" scale="time" domain={["dataMin", "dataMax"]} tickFormatter={fmt} fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} minTickGap={40} />
              <YAxis domain={[0, 100]} unit="%" fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} />
              <Tooltip
                labelFormatter={(t) => new Date(Number(t)).toLocaleString()}
                formatter={(v) => (typeof v === "number" ? `${v.toFixed(1)}%` : String(v))}
                contentStyle={{ background: "hsl(var(--popover))", border: "1px solid hsl(var(--border))", borderRadius: 8, fontSize: 12 }}
              />
              <Legend iconType="plainline" wrapperStyle={{ fontSize: 12 }} />
              <Line type="monotone" dataKey="CPU" stroke="hsl(var(--primary))" dot={false} strokeWidth={1.75} isAnimationActive={false} />
              <Line type="monotone" dataKey="Memory" stroke="hsl(var(--success))" dot={false} strokeWidth={1.75} isAnimationActive={false} />
              <Line type="monotone" dataKey="Disk" stroke="hsl(var(--warning))" dot={false} strokeWidth={1.75} isAnimationActive={false} />
              {hasBattery && <Line type="monotone" dataKey="Battery" stroke="hsl(var(--muted-foreground))" dot={false} strokeWidth={1.5} strokeDasharray="4 3" isAnimationActive={false} />}
            </LineChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  );
}

function Commands({ id, onRun }: { id: string; onRun?: () => void }) {
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.deviceCommands(id), queryFn: () => api.devices.commands(id) });
  const [open, setOpen] = useState<string | null>(null);
  if (error) return <ErrorState error={error} onRetry={refetch} />;
  return (
    <Card className="overflow-hidden">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <p className="text-sm font-medium">Command history</p>
        {onRun && (
          <Button size="sm" onClick={onRun}>
            <Terminal /> Run script
          </Button>
        )}
      </div>
      {isLoading ? (
        <TableSkeleton rows={4} cols={4} />
      ) : !data?.items.length ? (
        <EmptyState icon={Terminal} title="No commands yet" description="Scripts and actions you send to this device appear here with their output." />
      ) : (
        <ul className="divide-y divide-border">
          {data.items.map((c: Command) => {
            const expanded = open === c.id;
            return (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => setOpen(expanded ? null : c.id)}
                  aria-expanded={expanded}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-left text-sm hover:bg-muted/50"
                >
                  {expanded ? <ChevronDown className="h-4 w-4 text-muted-foreground" /> : <ChevronRight className="h-4 w-4 text-muted-foreground" />}
                  <span className="min-w-0 flex-1 truncate font-medium">{commandLabel(c.type, c.payload)}</span>
                  <span className="hidden text-xs text-muted-foreground sm:inline">{c.issued_by_email ?? titleCase(c.source)}</span>
                  <span className="w-28 text-right text-xs text-muted-foreground">{relativeTime(c.created_at)}</span>
                  <CommandStatusBadge status={c.status} />
                </button>
                {expanded && (
                  <div className="border-t border-border bg-muted/20 px-4 py-3 pl-11">
                    <CommandOutput cmd={c} />
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}

function EditDevice({ d, onClose }: { d: DeviceDetail; onClose: () => void }) {
  const qc = useQueryClient();
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const [employee, setEmployee] = useState(d.employee_name);
  const [groupId, setGroupId] = useState(d.group_id ?? "");
  const [tags, setTags] = useState(d.tags);
  const groupName = groups.data?.items.find((g) => g.id === groupId)?.name ?? (groupId === d.group_id ? d.group_name : null);
  const emp = employee.trim();
  // Same rule as the server: "<group> | <employee>", or the employee alone without a group.
  const name = emp ? (groupName ? `${groupName} | ${emp}` : emp) : "";
  const save = useMutation({
    mutationFn: () => api.devices.update(d.id, { employee_name: emp, group_id: groupId || null, tags }),
    onSuccess: (updated) => {
      qc.setQueryData<DeviceDetail>(qk.device(d.id), (old) => (old ? { ...old, ...updated } : old));
      void qc.invalidateQueries({ queryKey: qk.devices() });
      void qc.invalidateQueries({ queryKey: qk.deviceCompliance(d.id) });
      toast.success("Device updated");
      onClose();
    },
    onError: (e) => toast.error("Couldn't update device", e),
  });
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Edit device"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={save.isPending} disabled={!emp} onClick={() => save.mutate()}>
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Employee name" htmlFor="demployee" hint="Who uses this device. For shared machines and servers, describe it instead, e.g. Sprout Server Office.">
          <Input id="demployee" value={employee} onChange={(e) => setEmployee(e.target.value)} placeholder="e.g. Arnold Nacario" maxLength={128} autoFocus required />
        </Field>
        <Field label="Group" htmlFor="dgroup" hint="The group's app blocklist applies to this device.">
          <Select
            id="dgroup"
            value={groupId}
            onValueChange={setGroupId}
            placeholder="No group"
            options={(groups.data?.items ?? []).map((g) => ({ value: g.id, label: g.name }))}
          />
        </Field>
        <Field label="Tags" htmlFor="dtags" hint="Press Enter to add. Tags are searchable and usable in compliance rules. A tag a compliance rule added comes back while the device still fails that rule.">
          <TagInput id="dtags" value={tags} onChange={setTags} placeholder="e.g. finance, loaner" />
        </Field>
        <Field label="Display name" htmlFor="dname" hint="Generated from the group and employee name.">
          <Input id="dname" value={name} readOnly placeholder="Enter an employee name" className="bg-muted/50 text-muted-foreground" />
        </Field>
      </div>
    </Dialog>
  );
}

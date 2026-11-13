"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
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
import { agentOutdated, api, qk, type Command, type CommandType, type ComplianceResult, type DeviceDetail, type Rule, type RuleActions, type Severity, type Disk, type Inventory, type NetInterface, type NetKind } from "@/lib/api";
import { useCan } from "@/lib/session";
import { cn, RESOURCE_LIMITS, copyToClipboard, formatBytes, formatDate, formatDateTime, formatDuration, formatPct, osLabel, relativeTime, relativeTimeShort, titleCase } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Dropdown, DropdownContent, DropdownItem, DropdownSeparator, DropdownTrigger } from "@/components/ui/dropdown";
import { Field, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { SortButton, Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
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
  const [softwareView, setSoftwareView] = useState(0);
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

  const quick: { type: CommandType; label: string; icon: typeof Play; hint: string }[] = [
    { type: "run_script", label: "Run script", icon: Terminal, hint: "Run a script on this device" },
    {
      type: "refresh_inventory",
      label: "Refresh",
      icon: RefreshCw,
      hint: "Asks the device to send a fresh inventory report. The tabs update when it arrives (Compliance > Check now also waits for it)",
    },
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
              <Button key={q.type} variant="outline" onClick={() => setAction(q.type)} title={q.hint}>
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
        <div className="sticky top-16 z-20 -mx-4 -my-2 flex h-14 items-center gap-3 bg-[hsl(var(--canvas-from))] px-4 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
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
          <Hardware d={d} />
        </TabsContent>
        <TabsContent value="software">
          {/* Re-mounted on Flagged when a failing compliance rule links here. */}
          <Software key={softwareView} id={id} initialCategory={softwareView ? "flagged" : "all"} onOpenUsage={() => setTab("usage")} />
        </TabsContent>
        <TabsContent value="usage">
          <Usage id={id} os={d.os_family} />
        </TabsContent>
        <TabsContent value="network">
          <Network d={d} />
        </TabsContent>
        <TabsContent value="security">
          <Security d={d} />
        </TabsContent>
        <TabsContent value="compliance">
          <Compliance
            d={d}
            onOpenTab={(t) => {
              if (t === "software") setSoftwareView((n) => n + 1);
              setTab(t);
            }}
          />
        </TabsContent>
        <TabsContent value="telemetry">
          <Telemetry id={id} />
        </TabsContent>
        <TabsContent value="commands">
          <Commands id={id} />
        </TabsContent>
      </Tabs>

      {action && (
        <CommandDialog
          open
          onOpenChange={(o) => !o && setAction(null)}
          type={action}
          deviceIds={[id]}
          targetLabel={d.name}
          os={d.os_family}
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
  limits,
}: {
  icon: typeof Cpu;
  label: string;
  value?: number | null;
  /** Shown instead of a percentage, with no bar (e.g. uptime). */
  text?: string;
  sub?: string;
  /** A charge level (battery): low is the warning, not high. */
  charge?: boolean;
  /** Usage limits: the bar is soft blue below them, amber/red at them. */
  limits?: { warnAt: number; alertAt: number };
}) {
  if (charge && value != null) value = Math.min(100, value);
  return (
    <Card className="p-4">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Icon className="h-4 w-4" /> {label}
      </div>
      <p className="mt-2 text-2xl font-semibold tabular">{text ?? formatPct(value)}</p>
      {text === undefined && <UsageBar value={value} className="mt-2" charge={charge} quiet={!!limits} {...limits} />}
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
        <Metric icon={Cpu} label="CPU" value={d.cpu_pct} sub={inv?.hardware.cpu_model} limits={RESOURCE_LIMITS.cpu} />
        <Metric
          icon={MemoryStick}
          label="Memory"
          value={d.mem_pct}
          limits={RESOURCE_LIMITS.mem}
          sub={inv ? `${formatBytes(inv.hardware.memory_total_bytes)} installed` : undefined}
        />
        <Metric
          icon={HardDrive}
          label="System disk"
          value={d.disk_pct}
          limits={RESOURCE_LIMITS.disk}
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

function SecureBootBadge({ on, appleSilicon }: { on?: boolean; appleSilicon?: boolean }) {
  if (on === undefined && appleSilicon)
    return <span title="Apple silicon always verifies the boot chain; the security level (Full or Reduced) isn't reported.">Apple silicon secure boot</span>;
  if (on === undefined) return <span className="text-muted-foreground">Not reported</span>;
  return on ? <Badge variant="success">On</Badge> : <Badge variant="warning">Off</Badge>;
}

const DRIVE_HEALTH: Record<string, { label: string; variant: "success" | "warning" | "danger" }> = {
  healthy: { label: "SMART healthy", variant: "success" },
  warning: { label: "SMART warning", variant: "warning" },
  failing: { label: "Failing", variant: "danger" },
};

type Volume = { disk: Disk; label?: string; system: boolean };

/**
 * On macOS the system (/), Data and Recovery volumes share one APFS container,
 * so they'd all show the same numbers. Show them as one "System & data" entry and
 * leave out the internal system volumes nobody manages; other volumes stay.
 */
function macVolumes(disks: Disk[]): Volume[] {
  const root = disks.find((x) => x.mount === "/");
  const data = disks.find((x) => x.mount === "/System/Volumes/Data");
  const out: Volume[] = [];
  if (root || data) out.push({ disk: { ...(data ?? root)!, mount: "/" }, label: "System & data (APFS)", system: true });
  for (const x of disks) {
    if (x === root || x === data || x.mount === "/Volumes/Recovery" || x.mount.startsWith("/System/Volumes/")) continue;
    out.push({ disk: x, system: false });
  }
  return out;
}

const ENCRYPTION_OFF: Record<string, string> = { windows: "BitLocker off", macos: "FileVault off" };

function Hardware({ d }: { d: DeviceDetail }) {
  const inv = d.inventory;
  if (!inv) return <NoInventory />;
  const hw = inv.hardware;
  // Agents before 2026.11.15 don't report firmware, graphics, memory layout or drives.
  const v2 = hw.firmware !== undefined || hw.drives !== undefined || hw.gpus !== undefined || hw.memory !== undefined;
  const updateHint = (
    <p className="mt-3 text-xs text-muted-foreground">Update this device&apos;s agent to see firmware, graphics, memory and drive details.</p>
  );
  const mem = hw.memory;
  const memPct = hw.memory_total_bytes ? (hw.memory_used_bytes / hw.memory_total_bytes) * 100 : null;
  const fw = hw.firmware;
  const sysDisk = systemDisk(inv, d.os_family);
  const volumes: Volume[] =
    d.os_family === "macos" ? macVolumes(hw.disks) : hw.disks.map((disk) => ({ disk, system: disk === sysDisk }));
  const enc = inv.security;
  const b = hw.battery;
  const batteryFresh = Date.now() - new Date(inv.collected_at).getTime() < 3600_000;

  const cards: { key: string; title: string; body: React.ReactNode }[] = [
    {
      key: "cpu",
      title: "Processor & graphics",
      body: (
        <>
          <KeyValues
            items={[
              ["CPU", hw.cpu_model],
              ["Cores / threads", `${hw.cpu_cores} / ${hw.cpu_threads}`],
              ...(hw.gpus ?? []).map(
                (g): [React.ReactNode, React.ReactNode] => [
                  "Graphics",
                  [g.name, g.cores ? `${g.cores}-core` : "", g.driver_version ? `driver ${g.driver_version}` : ""].filter(Boolean).join(" · "),
                ],
              ),
              ...(v2 && !hw.gpus?.length ? [["Graphics", ""] as [React.ReactNode, React.ReactNode]] : []),
            ]}
          />
          {!v2 && updateHint}
        </>
      ),
    },
    {
      key: "memory",
      title: "Memory",
      body: (
        <>
          <div className="mb-4">
            <div className="mb-1.5 flex items-baseline justify-between gap-4 text-sm">
              <span className="font-medium">{formatBytes(hw.memory_total_bytes)} installed</span>
              <span className="text-xs text-muted-foreground tabular">{formatBytes(hw.memory_used_bytes)} in use</span>
            </div>
            <UsageBar value={memPct} className="h-2" quiet {...RESOURCE_LIMITS.mem} />
          </div>
          {mem ? (
            <KeyValues
              items={[
                ["Type", [mem.type, mem.speed_mts ? `${mem.speed_mts} MT/s` : ""].filter(Boolean).join(" · ")],
                [
                  "Upgrade",
                  mem.soldered
                    ? "Soldered to the board, not upgradable"
                    : mem.slots_total
                      ? `${mem.slots_used ?? 0} of ${mem.slots_total} slots used${(mem.slots_used ?? 0) < mem.slots_total ? " · room to add more" : ""}`
                      : "",
                ],
                ...(mem.modules ?? []).map(
                  (m): [React.ReactNode, React.ReactNode] => [
                    m.slot || "Module",
                    [formatBytes(m.capacity_bytes), m.type, m.speed_mts ? `${m.speed_mts} MT/s` : ""].filter(Boolean).join(" · "),
                  ],
                ),
              ]}
            />
          ) : v2 ? (
            <p className="text-sm text-muted-foreground">Module details weren&apos;t available on this device.</p>
          ) : (
            updateHint
          )}
        </>
      ),
    },
    {
      key: "system",
      title: "System & firmware",
      body: (
        <>
          <KeyValues
            items={[
              ["Manufacturer", hw.manufacturer],
              ["Model", hw.model],
              ["Serial number", hw.serial && <CopyValue key="s" value={hw.serial} label="serial number" mono />],
              ...(fw
                ? ([
                    ["Firmware", [fw.vendor && fw.vendor !== hw.manufacturer ? fw.vendor : "", fw.version].filter(Boolean).join(" ")],
                    ["Firmware date", fw.release_date ? formatDate(fw.release_date) : ""],
                    [
                      "Secure Boot",
                      <SecureBootBadge key="sb" on={fw.secure_boot} appleSilicon={fw.security_chip === "Apple Secure Enclave"} />,
                    ],
                    [
                      "Security chip",
                      fw.security_chip ? (
                        <span key="tpm" className="inline-flex flex-wrap items-center gap-2">
                          {fw.security_chip}
                          {fw.tpm_enabled === true && <Badge variant="success">Enabled</Badge>}
                          {fw.tpm_enabled === false && <Badge variant="warning">Disabled</Badge>}
                        </span>
                      ) : (
                        "None found"
                      ),
                    ],
                  ] as [React.ReactNode, React.ReactNode][]).filter(([k, v]) => !((k === "Firmware" || k === "Firmware date") && !v)) // Macs have no firmware date
                : []),
            ]}
          />
          {!v2 && updateHint}
        </>
      ),
    },
  ];
  if (b.present) {
    cards.push({
      key: "battery",
      title: "Battery",
      body: (
        <KeyValues
          items={[
            ["Charge", formatPct(b.percent)],
            ["State", batteryFresh && BATTERY_STATE[b.state] ? BATTERY_STATE[b.state] : ""],
            ["Health", b.health_pct == null ? "Unknown" : formatPct(b.health_pct)],
            ["Charge cycles", b.cycle_count != null ? String(b.cycle_count) : ""],
            [
              "Capacity",
              b.full_mwh && b.design_mwh ? `${(b.full_mwh / 1000).toFixed(1)} Wh of ${(b.design_mwh / 1000).toFixed(1)} Wh when new` : "",
            ],
          ]}
        />
      ),
    });
  }

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      {cards.map((c, i) => (
        // An odd card out spans both columns, so desktops without a battery have no gap.
        <Card key={c.key} className={cn(i === cards.length - 1 && cards.length % 2 === 1 && "lg:col-span-2")}>
          <CardHeader>
            <CardTitle>{c.title}</CardTitle>
          </CardHeader>
          <CardContent>{c.body}</CardContent>
        </Card>
      ))}
      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle>Storage</CardTitle>
        </CardHeader>
        <CardContent className="space-y-5">
          {hw.drives && hw.drives.length > 0 && (
            <div className="space-y-2">
              <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Drives</p>
              {hw.drives.map((dr, i) => {
                const health = dr.health ? DRIVE_HEALTH[dr.health] : undefined;
                return (
                  <div key={`${dr.model}-${i}`} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-sm">
                    <span className="min-w-0">
                      <span className="font-medium">{dr.model}</span>
                      <span className="ml-2 text-xs text-muted-foreground">
                        {[dr.bus, dr.media, dr.size_bytes ? formatBytes(dr.size_bytes) : ""].filter(Boolean).join(" · ")}
                      </span>
                    </span>
                    {health ? <Badge variant={health.variant}>{health.label}</Badge> : <span className="text-xs text-muted-foreground">Health not reported</span>}
                  </div>
                );
              })}
            </div>
          )}
          <div className="space-y-4">
            {hw.drives && hw.drives.length > 0 && <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Volumes</p>}
            {volumes.length === 0 && <p className="text-sm text-muted-foreground">No volumes reported.</p>}
            {volumes.map(({ disk, label, system: isSystem }) => {
              const pct = disk.total_bytes ? (disk.used_bytes / disk.total_bytes) * 100 : 0;
              return (
                <div key={disk.mount}>
                  <div className="mb-1.5 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 text-sm">
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="truncate font-mono">{disk.mount}</span>
                      <span className="text-xs text-muted-foreground">{label ?? disk.fstype}</span>
                      {isSystem && enc.disk_encrypted === true && <Badge variant="success">{enc.encryption_method || "Encrypted"}</Badge>}
                      {isSystem && enc.disk_encrypted === false && (
                        <Badge variant="warning">{ENCRYPTION_OFF[d.os_family] ?? "Not encrypted"}</Badge>
                      )}
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground tabular">
                      {formatBytes(disk.used_bytes)} / {formatBytes(disk.total_bytes)} · {formatBytes(disk.free_bytes)} free
                    </span>
                  </div>
                  <UsageBar value={pct} className="h-2" quiet {...RESOURCE_LIMITS.disk} />
                </div>
              );
            })}
          </div>
          {!v2 && updateHint}
        </CardContent>
      </Card>
    </div>
  );
}

const SOFTWARE_SOURCE: Record<string, string> = {
  msstore: "Microsoft Store",
  registry: "Registry",
  dpkg: "APT (dpkg)",
  rpm: "RPM",
  snap: "Snap",
  flatpak: "Flatpak",
  apple: "Apple",
  mac_app_store: "Mac App Store",
  identified_developer: "Developer ID",
  unknown: "Unknown",
};

// Long package lists (a Linux server can have 2,000) show this many rows until asked for all.
const SOFTWARE_PAGE = 500;

// Software sorting: name A–Z by default; dates newest first on the first click.
// Apps with no date always go last, whichever way the column is sorted.
type SoftwareSortKey = "name" | "installed" | "used";
type SoftwareSort = { key: SoftwareSortKey; dir: "asc" | "desc" };

function sortSoftware<T extends { name: string; install_date?: string | null; last_used?: string | null }>(items: T[], sort: SoftwareSort): T[] {
  const sign = sort.dir === "asc" ? 1 : -1;
  const date = (s: T) => {
    const v = sort.key === "installed" ? s.install_date : s.last_used;
    const t = v ? new Date(v).getTime() : NaN;
    return Number.isNaN(t) ? null : t;
  };
  return [...items].sort((a, b) => {
    if (sort.key !== "name") {
      const da = date(a);
      const db = date(b);
      if (da !== db) {
        if (da === null) return 1;
        if (db === null) return -1;
        return (da - db) * sign;
      }
      return a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
    }
    return a.name.localeCompare(b.name, undefined, { sensitivity: "base" }) * sign;
  });
}

// The warning banner names at most this many apps per line; "+N more" opens the Flagged filter.
const BANNER_NAMES = 3;

function FlaggedLine({ label, apps, onMore }: { label: string; apps: { name: string }[]; onMore: () => void }) {
  // Two installed versions of one app are one finding (the table below still lists both).
  const names = [...new Set(apps.map((a) => a.name))];
  const shown = names.slice(0, BANNER_NAMES);
  const more = names.length - shown.length;
  return (
    <p className="min-w-0">
      <span className="font-medium">
        {label} ({names.length}):
      </span>{" "}
      {shown.map((name, i) => (
        <span key={name}>
          {i > 0 && ", "}
          <span className="inline-block max-w-[18rem] truncate align-bottom" title={name}>
            {name}
          </span>
        </span>
      ))}
      {more > 0 && (
        <>
          {" "}
          <button type="button" onClick={onMore} className="font-medium underline underline-offset-2 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring rounded">
            +{more} more
          </button>
        </>
      )}
    </p>
  );
}

type SoftwareCategory = "all" | "system" | "third_party" | "flagged";

function Software({ id, initialCategory = "all", onOpenUsage }: { id: string; initialCategory?: SoftwareCategory; onOpenUsage: () => void }) {
  const [q, setQ] = useState("");
  const [category, setCategory] = useState<SoftwareCategory>(initialCategory);
  const [showAll, setShowAll] = useState(false);
  const [sort, setSort] = useState<SoftwareSort>({ key: "name", dir: "asc" });
  const onSort = (key: SoftwareSortKey) =>
    setSort((cur) => (cur.key === key ? { key, dir: cur.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "name" ? "asc" : "desc" }));
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.deviceSoftware(id), queryFn: () => api.devices.software(id) });
  const all = useMemo(() => data?.items ?? [], [data]);
  const counts = useMemo(() => ({ system: all.filter((s) => s.category === "system").length, third_party: all.filter((s) => s.category === "third_party").length }), [all]);
  const gambling = all.filter((s) => s.flag === "gambling");
  const games = all.filter((s) => s.flag === "game");
  const items = useMemo(() => {
    const needle = q.trim().toLowerCase();
    const inCategory = (s: (typeof all)[number]) => category === "all" || (category === "flagged" ? !!s.flag : s.category === category);
    const matched = all.filter((s) => inCategory(s) && (!needle || `${s.name} ${s.publisher}`.toLowerCase().includes(needle)));
    // Sorted before the 500-row cap, so "newest installed" really is the newest of all of them.
    return sort.key === "name" && sort.dir === "asc" ? matched : sortSoftware(matched, sort);
  }, [all, q, category, sort]);
  const shown = showAll ? items : items.slice(0, SOFTWARE_PAGE);
  const copy = async (text: string, what: string) => {
    if (await copyToClipboard(text)) toast.success(`Copied ${what}`, text);
    else toast.error("Couldn't copy to clipboard");
  };
  if (error) return <ErrorState error={error} onRetry={refetch} />;
  return (
    // The column headers pin under the device's sticky tab row (top bar 4rem + row 3.5rem).
    <Card className="overflow-clip [--sticky-top:7.5rem]">
      {(gambling.length > 0 || games.length > 0) && (
        <div role="alert" className="flex items-start gap-3 border-b border-warning/25 bg-warning/10 p-3 text-sm">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
          <div className="min-w-0 space-y-0.5">
            {gambling.length > 0 && <FlaggedLine label="Gambling apps installed" apps={gambling} onMore={() => setCategory("flagged")} />}
            {games.length > 0 && <FlaggedLine label="Games installed" apps={games} onMore={() => setCategory("flagged")} />}
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
        <>
          <Table stickyHeader>
            <THead>
              <TR>
                <TH>
                  <SortButton label="Name / publisher" k="name" sort={sort} onSort={onSort} />
                </TH>
                <TH>Version</TH>
                <TH>Type</TH>
                <TH className="hidden sm:table-cell">Source</TH>
                <TH className="hidden lg:table-cell">
                  <SortButton label="Installed" k="installed" sort={sort} onSort={onSort} />
                </TH>
                <TH className="hidden lg:table-cell">
                  <SortButton label="Last used" k="used" sort={sort} onSort={onSort} />
                </TH>
                <TH className="w-10" />
              </TR>
            </THead>
            <TBody>
              {shown.map((s, i) => (
                <TR key={`${s.name}-${s.version}-${i}`}>
                  <TD className="font-medium">
                    {/* Name (and badges) on one line, publisher below; long text is cut with "…" and shown on hover. */}
                    <span className="flex max-w-[20rem] items-center gap-2">
                      <span className="min-w-0 truncate" title={s.name}>
                        {s.name}
                      </span>
                      {s.flag && (
                        <Badge variant={s.flag === "gambling" ? "danger" : "warning"} className="shrink-0">
                          {s.flag === "gambling" ? "Gambling" : "Game"}
                        </Badge>
                      )}
                      {s.infrastructure && (
                        <Badge variant="neutral" className="shrink-0" title="Marked as infrastructure in App usage">
                          Infrastructure
                        </Badge>
                      )}
                    </span>
                    <span className="block max-w-[20rem] truncate text-xs font-normal text-muted-foreground" title={s.publisher}>
                      {s.publisher || "Unknown publisher"}
                    </span>
                  </TD>
                  <TD className="max-w-[10rem] truncate font-mono text-xs" title={s.version}>
                    {s.version}
                  </TD>
                  <TD>
                    <Badge variant={s.category === "system" ? "primary" : "outline"}>{s.category === "system" ? "System" : "Third-party"}</Badge>
                  </TD>
                  <TD className="hidden sm:table-cell">
                    <Badge variant="neutral" className="whitespace-nowrap">
                      {SOFTWARE_SOURCE[s.source] ?? s.source}
                    </Badge>
                  </TD>
                  <TD className="hidden whitespace-nowrap text-muted-foreground lg:table-cell">{s.install_date ? formatDate(s.install_date) : "—"}</TD>
                  <TD className="hidden whitespace-nowrap text-muted-foreground lg:table-cell" title={s.last_used ? formatDateTime(s.last_used) : undefined}>
                    {s.last_used ? relativeTimeShort(s.last_used) : "—"}
                  </TD>
                  <TD className="pr-3 text-right">
                    <Dropdown>
                      <DropdownTrigger>
                        <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${s.name}`}>
                          <MoreHorizontal />
                        </Button>
                      </DropdownTrigger>
                      <DropdownContent align="end">
                        <DropdownItem onSelect={() => void copy(s.name, "name")}>Copy name</DropdownItem>
                        {s.version && <DropdownItem onSelect={() => void copy(s.version, "version")}>Copy version</DropdownItem>}
                        {s.version && <DropdownItem onSelect={() => void copy(`${s.name} ${s.version}`, "name and version")}>Copy name and version</DropdownItem>}
                        {s.last_used && (
                          <>
                            <DropdownSeparator />
                            <DropdownItem onSelect={onOpenUsage}>Show usage</DropdownItem>
                          </>
                        )}
                      </DropdownContent>
                    </Dropdown>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
          {!showAll && items.length > SOFTWARE_PAGE && (
            <div className="border-t border-border p-3 text-center">
              <Button variant="outline" size="sm" onClick={() => setShowAll(true)}>
                Show all {items.length.toLocaleString()}
              </Button>
              <p className="mt-1.5 text-xs text-muted-foreground">Showing the first {SOFTWARE_PAGE}. The filter above searches all of them.</p>
            </div>
          )}
        </>
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

// "bonded" is the console's own: agents report Linux bond/team adapters as
// virtual, which is true but misleading for a server's main link.
type AdapterKind = NetKind | "bonded";

const NET_KIND_LABEL: Record<AdapterKind, string> = {
  bonded: "Bonded",
  ethernet: "Ethernet",
  wifi: "Wi-Fi",
  vpn: "VPN",
  virtual: "Virtual",
  bluetooth: "Bluetooth",
};

// The agent's own guess (internal/agent/collect/netparse.go kindFromName), for
// agents before 2026.11.18 that don't report an adapter's type.
/** Linux link aggregation (bond0, team0) and Windows NIC teaming: several physical links combined. */
const isBonded = (name: string, desc = "") => /^(bond|team)\d/i.test(name) || /multiplexor|nic team/i.test(desc);

function adapterKind(i: NetInterface): AdapterKind | undefined {
  if (isBonded(i.name, i.description)) return "bonded";
  return i.kind ?? guessNetKind(i.name, i.description);
}

function guessNetKind(name: string, desc = ""): NetKind | undefined {
  const s = `${name} ${desc}`;
  const any = (re: RegExp) => re.test(name) || re.test(desc);
  if (/bluetooth/i.test(s)) return "bluetooth";
  if (any(/tailscale|wireguard|openvpn|\bvpn\b|surfshark|nordvpn|expressvpn|protonvpn|tap-windows|wintun|zerotier|fortinet|forticlient|anyconnect|globalprotect|pangp|\bipsec|^utun\d|^tun\d|^wg\d|^ppp\d/i)) return "vpn";
  if (any(/wi-fi direct|^local area connection\*|vethernet|hyper-v|virtualbox|vmware|docker|^veth|^br-|^virbr|^bridge\d|^awdl|^llw|^anpi|^ap\d|^gif\d|^stf\d|isatap|teredo|kernel debug|loopback|wan miniport/i)) return "virtual";
  if (any(/wi-?fi|wireless|wlan|802\.11|^wl/i)) return "wifi";
  if (any(/ethernet|^eth\d|^en[pso]\d|gbe|gigabit/i)) return "ethernet";
  return undefined;
}

/** 169.254.x.x and fe80:: addresses: assigned automatically, never used to reach the device. */
const linkLocal = (a: string) => a.startsWith("169.254.") || a.toLowerCase().startsWith("fe80:");
const bareIP = (a: string) => a.split("/")[0] ?? a;
const isPrivateIP = (ip: string) => /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|127\.|::1$|f[cd][0-9a-f]{2}:)/i.test(ip);

function formatSpeed(mbps?: number) {
  if (!mbps || mbps <= 0) return null;
  if (mbps >= 1000) return `${+(mbps / 1000).toFixed(1)} Gbps`;
  return `${+mbps.toFixed(mbps < 10 ? 1 : 0)} Mbps`;
}

function Address({ addr, muted }: { addr: string; muted?: boolean }) {
  const ip = bareIP(addr);
  const prefix = addr.split("/")[1];
  return (
    <span className={cn("block", muted && "text-muted-foreground")}>
      <CopyValue
        value={ip}
        label="address"
        mono
        display={
          <>
            {ip}
            {prefix && <span className="text-muted-foreground">/{prefix}</span>}
          </>
        }
      />
    </span>
  );
}

// IPv4 first, then global IPv6, then unique-local (fd…), then anything else.
function addrRank(a: string) {
  if (!a.includes(":")) return 0;
  const first = a.slice(0, 4).toLowerCase();
  if (/^[23]/.test(first)) return 1;
  if (/^f[cd]/.test(first)) return 2;
  return 3;
}

const ADDRS_SHOWN = 2;

/** An adapter's addresses: the first two, the rest behind "+N more" so dual-stack rows stay short. */
function AddressList({ addrs, muted }: { addrs: string[]; muted?: boolean }) {
  const [open, setOpen] = useState(false);
  const sorted = [...addrs].sort((a, b) => addrRank(a) - addrRank(b));
  const extra = sorted.length - ADDRS_SHOWN;
  const shown = open || extra <= 0 ? sorted : sorted.slice(0, ADDRS_SHOWN);
  return (
    <>
      {shown.map((a) => (
        <Address key={a} addr={a} muted={muted} />
      ))}
      {extra > 0 && (
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          className="mt-0.5 rounded text-[11px] font-medium text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {open ? "Show less" : `+${extra} more`}
        </button>
      )}
    </>
  );
}

const NET_INACTIVE_KEY = "device-net-show-inactive";

function Network({ d }: { d: DeviceDetail }) {
  const [showInactive, setShowInactiveState] = useState(false);
  useEffect(() => {
    try {
      if (localStorage.getItem(NET_INACTIVE_KEY) === "1") setShowInactiveState(true);
    } catch {}
  }, []);
  const setShowInactive = (v: boolean) => {
    setShowInactiveState(v);
    try {
      localStorage.setItem(NET_INACTIVE_KEY, v ? "1" : "0");
    } catch {}
  };

  const inv = d.inventory;
  if (!inv) return <NoInventory />;
  const n = inv.network;
  const primary =
    n.interfaces.find((i) => n.primary_ip && i.addrs.some((a) => bareIP(a) === n.primary_ip)) ??
    n.interfaces.find((i) => n.primary_mac && i.mac === n.primary_mac);
  // In use: the primary adapter, or one that's up with a real address. macOS
  // reports idle ports and system tunnels as "up" with only a link-local address.
  const usable = (i: NetInterface) => i.addrs.filter((a) => !linkLocal(a));
  const inUse = (i: NetInterface) => i === primary || (i.up && usable(i).length > 0);
  const rank = (i: NetInterface) => (i === primary ? 0 : inUse(i) ? 1 : 2);
  const sorted = n.interfaces.map((i, idx) => ({ i, idx })).sort((a, b) => rank(a.i) - rank(b.i) || a.idx - b.idx);
  const inactive = sorted.filter(({ i }) => !inUse(i)).length;
  const rows = showInactive ? sorted : sorted.filter(({ i }) => inUse(i));

  const wifi = n.wifi ?? (n.wifi_ssid ? { ssid: n.wifi_ssid } : undefined);
  // Recent macOS hides the network name from the agent; the link still shows
  // through the Wi-Fi details or a primary adapter that is Wi-Fi.
  const onWifi = !!primary && adapterKind(primary) === "wifi";
  const hiddenName = d.os_family === "macos" ? "Connected, name hidden by macOS" : "Connected, name not reported";
  const wifiDetail = [wifi?.signal_pct != null ? `${wifi.signal_pct}% signal` : null, wifi?.band, wifi?.radio].filter(Boolean).join(" · ");
  const wifiText =
    wifi || onWifi ? (
      <span key="wifi">
        <span className="block">{wifi?.ssid || hiddenName}</span>
        {wifiDetail && <span className="block text-xs text-muted-foreground">{wifiDetail}</span>}
      </span>
    ) : null;
  // IPv4 servers first: a router's link-local IPv6 address (fe80::…%en0) is real but less useful.
  const dns = [...(n.dns ?? [])].sort((a, b) => Number(a.includes(":")) - Number(b.includes(":")));
  const domainLabel = n.domain_joined === false ? "Workgroup" : "Domain";
  const remote = d.remote_ip;
  const dhcp = primary?.dhcp;

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="pt-5">
          <div className="grid grid-cols-1 gap-x-10 gap-y-2.5 lg:grid-cols-2">
            <KeyValues
              items={[
                ["Hostname", n.hostname && <CopyValue key="h" value={n.hostname} label="hostname" />],
                [
                  "Primary IP",
                  n.primary_ip && (
                    <span key="ip" className="inline-flex flex-wrap items-center gap-x-2">
                      <CopyValue value={n.primary_ip} label="IP address" mono />
                      {primary && <span className="text-xs text-muted-foreground">via {primary.name}</span>}
                    </span>
                  ),
                ],
                ["Primary MAC", n.primary_mac && <CopyValue key="mac" value={n.primary_mac} label="MAC address" mono />],
                ["Address assignment", dhcp == null ? "" : dhcp ? "DHCP (automatic)" : "Static"],
                ["Wi-Fi", wifiText ?? <span className="text-muted-foreground">Not connected</span>],
              ]}
            />
            <KeyValues
              items={[
                ["Default gateway", n.gateway && <CopyValue key="gw" value={n.gateway} label="gateway" mono />],
                [
                  "DNS servers",
                  dns.length ? (
                    <span key="dns">
                      {dns.map((s) => (
                        <span key={s} className="block">
                          <CopyValue value={s} label="DNS server" mono />
                        </span>
                      ))}
                    </span>
                  ) : (
                    ""
                  ),
                ],
                ...(d.os_family === "windows" || n.domain ? [[domainLabel, n.domain ?? ""] as [string, string]] : []),
                [
                  "Connects from",
                  !remote ? (
                    ""
                  ) : isPrivateIP(remote) ? (
                    <span key="rf" title={`Seen by the server as ${remote}, the office firewall`}>
                      Office network
                    </span>
                  ) : (
                    <span key="rf" className="inline-flex flex-wrap items-center gap-x-2">
                      <CopyValue value={remote} label="public IP" mono />
                      <span className="text-xs text-muted-foreground">public IP</span>
                    </span>
                  ),
                ],
              ]}
            />
          </div>
          {!n.details && (
            <p className="mt-4 text-xs text-muted-foreground">
              Update this device&apos;s agent to see the gateway, DNS servers, {d.os_family === "windows" ? "domain, " : ""}link speeds and Wi-Fi signal.
            </p>
          )}
        </CardContent>
      </Card>
      <Card className="overflow-clip [--sticky-top:7.5rem]">
        <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
          <p className="text-sm font-medium">
            Adapters <span className="font-normal text-muted-foreground">· {sorted.length - inactive} in use</span>
          </p>
          {inactive > 0 && (
            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              <Switch checked={showInactive} onCheckedChange={setShowInactive} aria-label="Show inactive adapters" />
              Show inactive ({inactive})
            </label>
          )}
        </div>
        <Table stickyHeader>
          <THead>
            <TR>
              <TH>Adapter</TH>
              <TH>Type</TH>
              <TH>State</TH>
              <TH className="hidden sm:table-cell">Speed</TH>
              <TH className="hidden md:table-cell">MAC</TH>
              <TH>Addresses</TH>
            </TR>
          </THead>
          <TBody>
            {rows.map(({ i }) => {
              const active = inUse(i);
              const kind = adapterKind(i);
              const real = usable(i);
              const local = i.addrs.filter(linkLocal);
              const speed = formatSpeed(kind === "wifi" && i === primary && wifi?.rx_mbps ? wifi.rx_mbps : i.speed_mbps);
              return (
                <TR key={i.name} className={cn(!active && "text-muted-foreground")}>
                  <TD className="font-medium">
                    <span className="flex max-w-[20rem] items-center gap-2">
                      <span className="min-w-0 truncate" title={i.name}>
                        {i.name}
                      </span>
                      {i === primary && (
                        <Badge variant="primary" className="shrink-0" title="The adapter this device uses to reach the internet">
                          Primary
                        </Badge>
                      )}
                    </span>
                    {i.description && i.description !== i.name && (!kind || i.description !== NET_KIND_LABEL[kind]) && (
                      <span className="block max-w-[20rem] truncate text-xs font-normal text-muted-foreground" title={i.description}>
                        {i.description}
                      </span>
                    )}
                  </TD>
                  <TD>{kind ? <Badge variant="neutral" title={kind === "bonded" ? "Several network links combined into one (link aggregation)" : undefined}>{NET_KIND_LABEL[kind]}</Badge> : <span className="text-muted-foreground">—</span>}</TD>
                  <TD className="whitespace-nowrap">
                    {active ? (
                      <Badge variant="success" dot>
                        Up
                      </Badge>
                    ) : i.up ? (
                      <Badge variant="neutral" dot title="Switched on, but it has no usable address (e.g. a VPN that isn't connected)">
                        No address
                      </Badge>
                    ) : (
                      <Badge variant="neutral" dot>
                        Down
                      </Badge>
                    )}
                  </TD>
                  <TD className="hidden whitespace-nowrap tabular sm:table-cell">{speed ?? <span className="text-muted-foreground">—</span>}</TD>
                  <TD className="hidden whitespace-nowrap text-xs md:table-cell">
                    {i.mac ? <CopyValue value={i.mac} label="MAC address" mono /> : <span className="text-muted-foreground">—</span>}
                  </TD>
                  <TD className="text-xs">
                    {real.length > 0 ? (
                      <>
                        <AddressList addrs={real} muted={!active} />
                        {!active && <span className="mt-0.5 block text-[11px] italic">Not in use (last known address)</span>}
                      </>
                    ) : local.length > 0 ? (
                      <span className="text-muted-foreground" title={local.join(", ")}>
                        Link-local only
                      </span>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TD>
                </TR>
              );
            })}
          </TBody>
        </Table>
      </Card>
    </div>
  );
}

type Pill = { text: string; variant: "success" | "warning" | "danger" | "neutral"; title?: string };

/** One security control: label, optional detail line, status pill, and a note when no rule checks a failing value. */
function SecRow({ label, detail, pill, unchecked }: { label: string; detail?: React.ReactNode; pill: Pill; unchecked?: boolean }) {
  // Only a failing value: "Unknown" or "Not reported" isn't something a rule would catch.
  const bad = pill.variant === "danger";
  return (
    <div className="flex items-start justify-between gap-4 py-3">
      <div className="min-w-0">
        <p className="text-sm font-medium">{label}</p>
        {detail && <p className="text-xs text-muted-foreground">{detail}</p>}
        {bad && unchecked && (
          <p className="mt-0.5 text-xs text-muted-foreground">
            Not checked by a compliance rule ·{" "}
            <Link href="/compliance" className="font-medium text-primary underline-offset-2 hover:underline">
              Add a rule
            </Link>
          </p>
        )}
      </div>
      <Badge variant={pill.variant} className="shrink-0" title={pill.title}>
        {pill.text}
      </Badge>
    </div>
  );
}

const onOff = (v: boolean | null | undefined, on = "Enabled", off = "Disabled"): Pill =>
  v == null ? { text: "Unknown", variant: "warning" } : v ? { text: on, variant: "success" } : { text: off, variant: "danger" };

function Security({ d }: { d: DeviceDetail }) {
  // Which security fields the compliance rules that apply to this device check, so
  // a red row can say it isn't enforced (the header's "Compliant" only covers rules).
  const results = useQuery({ queryKey: qk.deviceCompliance(d.id), queryFn: () => api.devices.compliance(d.id) });
  const rules = useQuery({ queryKey: qk.complianceRules, queryFn: api.compliance.rules });
  const inv = d.inventory;
  if (!inv) return <NoInventory />;
  const s = inv.security;
  const fw = inv.hardware.firmware;
  const applying = new Set((results.data?.items ?? []).map((r) => r.rule_id));
  const applyingRules = (rules.data?.items ?? []).filter((r) => r.enabled && applying.has(r.id));
  const checked = new Set(applyingRules.flatMap((r) => r.conditions.map((c) => c.field)));
  const known = !!results.data && !!rules.data;
  const unchecked = (field: string) => known && !checked.has(`security.${field}`);
  const osName = d.os_family === "macos" ? "macOS" : d.os_family === "linux" ? "Linux" : "Windows";

  // Antivirus
  const av = s.av;
  let avPill: Pill;
  if (av?.realtime === false) avPill = { text: "Real-time off", variant: "danger" };
  else if (av?.up_to_date === false) avPill = { text: "Definitions outdated", variant: "warning" };
  else if (av?.realtime) avPill = { text: "Active", variant: "success" };
  else if (av && d.os_family === "macos") avPill = { text: "Built in", variant: "success", title: "XProtect is part of macOS and always on" };
  else if (s.antivirus) avPill = { text: "Installed", variant: "neutral", title: "Update the agent to see whether it's active" };
  else avPill = { text: "None found", variant: d.os_family === "linux" ? "neutral" : "warning" };
  const avAge = av?.signature_age_days;
  const avDetail = [
    s.antivirus ?? (d.os_family === "linux" ? "No antivirus reported" : null),
    avAge != null ? (d.os_family === "macos" ? `updated ${avAge === 0 ? "today" : `${avAge} day${avAge === 1 ? "" : "s"} ago`}` : `definitions ${avAge === 0 ? "from today" : `${avAge} day${avAge === 1 ? "" : "s"} old`}`) : null,
    av?.tamper_protected != null ? `tamper protection ${av.tamper_protected ? "on" : "off"}` : null,
    av?.version ? `version ${av.version}` : null,
  ]
    .filter(Boolean)
    .join(" · ");

  // Firewall
  const profiles = s.firewall_profiles ?? [];
  const winProfiles = d.os_family === "windows" ? profiles : [];
  const offCount = winProfiles.filter((p) => p.enabled === false).length;
  const fwPill: Pill =
    winProfiles.length > 0 && offCount > 0 && offCount < winProfiles.length
      ? { text: `${offCount} of ${winProfiles.length} off`, variant: "danger" }
      : onOff(s.firewall_enabled);
  const fwDetail = profiles.length
    ? profiles.map((p) => `${p.name}: ${p.enabled == null ? "?" : p.enabled ? "On" : "Off"}`).join(" · ")
    : d.os_family === "linux" && s.details && s.firewall_enabled == null
      ? "No ufw, firewalld or nftables rules found"
      : undefined;

  // Updates
  const upd = s.pending_updates;
  const updPill: Pill = upd == null ? { text: "Unknown", variant: "warning" } : upd === 0 ? { text: "Up to date", variant: "success" } : { text: `${upd} pending`, variant: "danger" };
  const updDetail = [`Checked ${relativeTime(inv.collected_at)}`, d.os_build ? `build ${d.os_build}` : null].filter(Boolean).join(" · ");

  // Encryption
  const vol = s.encryption_volume;
  const volLabel = vol ? (vol === "/" ? "System volume (/)" : `System drive (${vol})`) : d.os_family === "windows" ? "System drive" : "System volume";
  const encDetail = [s.disk_encrypted ? s.encryption_method : null, volLabel].filter(Boolean).join(" · ");

  // Platform integrity (from the Hardware inventory)
  const appleSilicon = fw?.security_chip === "Apple Secure Enclave";
  const sbPill: Pill =
    fw?.secure_boot === undefined
      ? appleSilicon
        ? { text: "Always on", variant: "success", title: "Apple silicon always verifies the boot chain" }
        : { text: "Not reported", variant: "neutral" }
      : fw.secure_boot
        ? { text: "On", variant: "success" }
        : { text: "Off", variant: "warning" };
  const chipPill: Pill = !fw?.security_chip
    ? { text: fw ? "None found" : "Not reported", variant: "neutral" }
    : fw.tpm_enabled === false
      ? { text: "Disabled", variant: "warning" }
      : { text: "Present", variant: "success" };

  // Signed-in user
  const signedIn = s.user || d.logged_in_user;
  const userPill: Pill =
    s.user_admin == null
      ? !signedIn
        ? { text: "No one signed in", variant: "neutral" }
        : !s.details || !s.user
          ? { text: "Not checked", variant: "neutral", title: s.details ? "Only a user signed in at the screen is checked, not remote sessions" : "Update the agent to check admin rights" }
          : { text: "Unknown", variant: "neutral", title: "Admin rights may come through a domain group, which can't be checked from the device" }
      : s.user_admin
        ? { text: "Local administrator", variant: "warning", title: "Can install software and change security settings" }
        : { text: "Standard user", variant: "success" };

  // Screen lock
  const lockPill: Pill =
    s.screen_lock_enabled == null
      ? { text: "Not reported", variant: "neutral", title: d.os_family === "windows" ? "No inactivity lock policy is set on this PC" : `${osName} doesn't report this to background services` }
      : s.screen_lock_enabled
        ? { text: "Enabled", variant: "success" }
        : { text: "Off", variant: "danger" };
  const lockDetail =
    s.screen_lock_minutes != null
      ? `Locks after ${s.screen_lock_minutes} min idle`
      : s.screen_lock_enabled == null && d.os_family === "windows"
        ? "No machine lock policy set"
        : undefined;

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {known ? (
          <>
            The {d.compliance_state === "compliant" ? "Compliant" : "compliance"} status above covers only the compliance rules that apply to this device
            {applyingRules.length ? `: ${applyingRules.map((r) => r.name).join(", ")}` : " (none)"}.{" "}
            <Link href="/compliance" className="font-medium text-primary underline-offset-2 hover:underline">
              View rules
            </Link>
          </>
        ) : (
          "The compliance status above covers only the compliance rules that apply to this device."
        )}
      </p>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Threat &amp; network defense</CardTitle>
          </CardHeader>
          <CardContent className="divide-y divide-border pt-0">
            <SecRow label="Antivirus" detail={avDetail || undefined} pill={avPill} />
            <SecRow label="Firewall" detail={fwDetail} pill={fwPill} unchecked={unchecked("firewall_enabled")} />
            <SecRow label="OS updates" detail={updDetail} pill={updPill} unchecked={unchecked("pending_updates")} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Device &amp; access hardening</CardTitle>
          </CardHeader>
          <CardContent className="divide-y divide-border pt-0">
            <SecRow label="Disk encryption" detail={encDetail} pill={onOff(s.disk_encrypted)} unchecked={unchecked("disk_encrypted")} />
            <SecRow label="Secure Boot" detail={appleSilicon && fw?.secure_boot === undefined ? "Apple silicon · Full or Reduced level isn't reported" : undefined} pill={sbPill} />
            <SecRow label="Security chip" detail={fw?.security_chip} pill={chipPill} />
            {(s.os_integrity ?? []).map((o) => (
              <SecRow key={o.name} label={o.name} detail={o.detail} pill={onOff(o.enabled, "On", "Off")} />
            ))}
            <SecRow label="Signed-in user" detail={signedIn || undefined} pill={userPill} />
            <SecRow label="Screen lock" detail={lockDetail} pill={lockPill} unchecked={unchecked("screen_lock_enabled")} />
          </CardContent>
        </Card>
      </div>
      {!s.details && (
        <p className="text-xs text-muted-foreground">
          Update this device&apos;s agent to see antivirus health, firewall profiles, {d.os_family === "windows" ? "User Account Control" : d.os_family === "macos" ? "SIP and Gatekeeper" : "SELinux/AppArmor"} and whether the signed-in user is an administrator.
        </p>
      )}
    </div>
  );
}

// All of a device's rules are evaluated together, so the time lives once in the summary strip.
type ComplianceSortKey = "rule" | "severity" | "result";
const SEVERITY_RANK: Record<Severity, number> = { low: 0, medium: 1, high: 2, critical: 3 };
const PLATFORM: Record<string, string> = { windows: "Windows", macos: "macOS", linux: "Linux" };
const platforms = (os: string[]) => os.map((o) => PLATFORM[o] ?? o).join(" & ");

// Where a failing rule's findings can be seen: the tab its conditions read from.
function drillTarget(rule?: Rule): "software" | "security" | undefined {
  const fields = rule?.conditions.map((c) => c.field) ?? [];
  if (fields.some((f) => f.startsWith("software."))) return "software";
  if (fields.some((f) => f.startsWith("security."))) return "security";
  return undefined;
}

function ruleScope(r: Rule, groupName: (id: string) => string): string {
  const parts = [r.group_id ? `Group: ${groupName(r.group_id)}` : "All devices"];
  if (r.os_families.length && r.os_families.length < 3) parts.push(`${platforms(r.os_families)} only`);
  return parts.join(" · ");
}

function onFailure(a: RuleActions): string {
  const parts: string[] = [];
  if (a.alert) parts.push("Alert");
  if (a.tag) parts.push(`Tag “${a.tag}”`);
  if (a.webhook) parts.push("Webhook");
  if (a.command) parts.push(commandLabel(a.command.type, a.command.payload));
  return parts.length ? `On failure: ${parts.join(" · ")}` : "On failure: marked non-compliant only";
}

// Why an enabled rule skips this device (mirrors compliance.Applies on the server).
function notApplyingReason(r: Rule, d: DeviceDetail, groupName: (id: string) => string): string {
  if (r.os_families.length && !r.os_families.includes(d.os_family)) return `${platforms(r.os_families)} only`;
  if (r.group_id && r.group_id !== d.group_id) return `Only for group ${groupName(r.group_id)}`;
  return "Not evaluated yet";
}

// How long "Check now" waits for the device's fresh report before giving up.
const CHECK_TIMEOUT_MS = 120_000;

function Compliance({ d, onOpenTab }: { d: DeviceDetail; onOpenTab: (tab: "software" | "security") => void }) {
  const id = d.id;
  const can = useCan();
  const qc = useQueryClient();
  // A pending "Check now": when it started (for the timeout) and the newest evaluation
  // time before it, so only the server's clock decides when fresh results have landed.
  const [checking, setChecking] = useState<{ since: number; baseline: string | null } | null>(null);
  const results = useQuery({
    queryKey: qk.deviceCompliance(id),
    queryFn: () => api.devices.compliance(id),
    refetchInterval: checking ? 3000 : false,
  });
  const rules = useQuery({ queryKey: qk.complianceRules, queryFn: api.compliance.rules });
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const [sort, setSort] = useState<{ key: ComplianceSortKey; dir: "asc" | "desc" }>({ key: "result", dir: "asc" });
  const onSort = (key: ComplianceSortKey) =>
    setSort((cur) => (cur.key === key ? { key, dir: cur.dir === "asc" ? "desc" : "asc" } : { key, dir: key === "severity" ? "desc" : "asc" }));
  const [showOthers, setShowOthers] = useState(false);

  const items = useMemo(() => results.data?.items ?? [], [results.data]);
  const latest = items.reduce<string | null>((m, r) => (!m || r.evaluated_at > m ? r.evaluated_at : m), null);

  // Stop waiting once a newer evaluation arrives, or after the timeout.
  useEffect(() => {
    if (!checking) return;
    if (latest && (!checking.baseline || latest > checking.baseline)) {
      setChecking(null);
      toast.success("Compliance checked", "Results are from the device's latest report.");
      void qc.invalidateQueries({ queryKey: qk.device(id) });
      void qc.invalidateQueries({ queryKey: qk.deviceSoftware(id) });
      return;
    }
    const t = setTimeout(
      () => {
        setChecking(null);
        toast.error("No new report yet", "The device hasn't sent its inventory. Results update on their own when it does.");
      },
      Math.max(0, checking.since + CHECK_TIMEOUT_MS - Date.now()),
    );
    return () => clearTimeout(t);
  }, [checking, latest, id, qc]);

  const check = useMutation({
    // Re-checking on the server would only re-read the last report; ask the device for a fresh one.
    mutationFn: () => api.devices.sendCommand(id, { type: "refresh_inventory", payload: {} }),
    onMutate: () => setChecking({ since: Date.now(), baseline: latest }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: qk.deviceCommands(id) }),
    onError: (e) => {
      setChecking(null);
      toast.error("Couldn't start the check", e);
    },
  });

  const ruleById = useMemo(() => new Map((rules.data?.items ?? []).map((r) => [r.id, r])), [rules.data]);
  const groupName = (gid: string) => groups.data?.items.find((g) => g.id === gid)?.name ?? "another group";
  const sorted = useMemo(() => {
    const dir = sort.dir === "asc" ? 1 : -1;
    const bySeverity = (a: ComplianceResult, b: ComplianceResult) => SEVERITY_RANK[b.severity] - SEVERITY_RANK[a.severity];
    return [...items].sort((a, b) => {
      let c = 0;
      if (sort.key === "rule") c = a.rule_name.localeCompare(b.rule_name);
      else if (sort.key === "severity") c = SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity];
      else c = Number(a.passed) - Number(b.passed);
      return c * dir || bySeverity(a, b) || a.rule_name.localeCompare(b.rule_name);
    });
  }, [items, sort]);
  const applying = new Set(items.map((r) => r.rule_id));
  const others = (rules.data?.items ?? []).filter((r) => r.enabled && !applying.has(r.id));

  if (results.error) return <ErrorState error={results.error} onRetry={results.refetch} />;
  if (results.isLoading) return <Skeleton className="h-48" />;

  const failing = items.filter((r) => !r.passed).length;
  const online = d.status === "online";
  const busy = checking !== null || check.isPending;
  const ruleWord = items.length === 1 ? "rule" : "rules";

  const othersList = others.length > 0 && (
    <div className="border-t border-border">
      <button
        type="button"
        onClick={() => setShowOthers((v) => !v)}
        aria-expanded={showOthers}
        className="flex w-full items-center gap-1.5 px-4 py-3 text-left text-sm text-muted-foreground hover:text-foreground"
      >
        {showOthers ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        {others.length === 1 ? "1 other rule doesn’t" : `${others.length} other rules don’t`} apply to this device
      </button>
      {showOthers && (
        <ul className="divide-y divide-border border-t border-border">
          {others.map((r) => (
            <li key={r.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 py-2.5 pl-10 pr-4 text-sm">
              <span className="font-medium">{r.name}</span>
              <span className="text-muted-foreground">{notApplyingReason(r, d, groupName)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );

  if (!items.length)
    return (
      <Card>
        <EmptyState
          icon={ShieldCheck}
          title="No compliance rules apply to this device"
          description={
            others.length
              ? "Every enabled rule is limited to other platforms or groups. Change a rule's scope, or move the device into a group, to check it here."
              : "There are no enabled rules yet. Rules are checked whenever a device reports its inventory."
          }
          action={
            <Link href="/compliance" className="inline-flex h-9 items-center rounded-lg border border-border bg-card px-3.5 text-sm font-medium shadow-sm hover:bg-muted">
              Manage rules
            </Link>
          }
        />
        {othersList}
      </Card>
    );

  return (
    // The column headers pin under the device's sticky tab row (top bar 4rem + row 3.5rem).
    <Card className="overflow-clip [--sticky-top:7.5rem]">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border p-3 pl-4">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
          {failing ? (
            <span className="inline-flex items-center gap-1.5 font-semibold text-destructive">
              <ShieldAlert className="h-4 w-4" />
              {failing} of {items.length} {ruleWord} failing
            </span>
          ) : (
            <span className="inline-flex items-center gap-1.5 font-semibold text-success">
              <ShieldCheck className="h-4 w-4" />
              {items.length} of {items.length} {ruleWord} passing
            </span>
          )}
          {latest && (
            <span className="text-muted-foreground" title={formatDateTime(latest)}>
              Last evaluated {relativeTimeShort(latest)}
            </span>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {can("commands.run") && (
            // Disabled buttons don't show tooltips, so the hint sits on a wrapper.
            <span
              title={
                online
                  ? "Asks the device for a fresh report, then waits here and updates these results when it arrives"
                  : "The device is offline. Its rules are checked again when it next reports."
              }
            >
              <Button variant="outline" size="sm" onClick={() => check.mutate()} disabled={!online || busy}>
                <RefreshCw className={cn(busy && "motion-safe:animate-spin")} /> {busy ? "Checking…" : "Check now"}
              </Button>
            </span>
          )}
          <Link href="/compliance" className="inline-flex h-8 items-center gap-1 rounded-lg px-2.5 text-xs font-medium hover:bg-muted">
            Manage rules <ChevronRight className="h-4 w-4" />
          </Link>
        </div>
      </div>
      <Table stickyHeader>
        <THead>
          <TR>
            <TH>
              <SortButton label="Rule" k="rule" sort={sort} onSort={onSort} />
            </TH>
            <TH>
              <SortButton label="Severity" k="severity" sort={sort} onSort={onSort} />
            </TH>
            <TH>
              <SortButton label="Result" k="result" sort={sort} onSort={onSort} />
            </TH>
            <TH className="hidden md:table-cell">Details</TH>
          </TR>
        </THead>
        <TBody>
          {sorted.map((r) => {
            const rule = ruleById.get(r.rule_id);
            const target = r.passed ? undefined : drillTarget(rule);
            const details = (
              <>
                <div className={cn(!r.passed && "text-foreground")}>{r.details}</div>
                {target && (
                  <button
                    type="button"
                    onClick={() => onOpenTab(target)}
                    className="mt-1 inline-flex items-center gap-0.5 text-xs font-medium text-primary underline-offset-2 hover:underline"
                  >
                    View in {target === "software" ? "Software" : "Security"} <ChevronRight className="h-3 w-3" />
                  </button>
                )}
              </>
            );
            return (
              <TR key={r.rule_id}>
                <TD className="align-top">
                  <div className="font-medium">{r.rule_name}</div>
                  {rule && (
                    <div className="mt-0.5 space-y-0.5 text-xs text-muted-foreground">
                      <div>{ruleScope(rule, groupName)}</div>
                      <div>{onFailure(rule.actions)}</div>
                    </div>
                  )}
                  {/* The Details column is hidden on phones; show it under the rule instead. */}
                  <div className="mt-2 text-sm text-muted-foreground md:hidden">{details}</div>
                </TD>
                <TD className="align-top">
                  <SeverityBadge severity={r.severity} />
                </TD>
                <TD className="align-top">
                  <Badge variant={r.passed ? "success" : "danger"}>{r.passed ? "Pass" : "Fail"}</Badge>
                </TD>
                <TD className="hidden max-w-md align-top text-muted-foreground md:table-cell">{details}</TD>
              </TR>
            );
          })}
        </TBody>
      </Table>
      {othersList}
    </Card>
  );
}

const RANGES = { "1": "1h", "24": "24h", "168": "7d" } as const;
type Range = keyof typeof RANGES;
type Series = "CPU" | "Memory" | "Disk" | "Battery";
type TelemetryRow = { t: number } & Record<Series, number | null>;

const SERIES: { key: Series; color: string; dash?: string; width: number }[] = [
  { key: "CPU", color: "hsl(var(--primary))", width: 1.75 },
  { key: "Memory", color: "hsl(var(--success))", width: 1.75 },
  { key: "Disk", color: "hsl(var(--warning))", width: 1.75 },
  { key: "Battery", color: "hsl(var(--muted-foreground))", dash: "4 3", width: 1.5 },
];
const HOUR = 3_600_000;
// The plot area's horizontal insets (Y-axis width on the left, margin on the right),
// shared by the chart and the availability strip under it so their times line up.
const PLOT_LEFT = 44;
const PLOT_RIGHT = 8;

// The chart's hover line, drawn only where there are values. Over an offline gap the
// hover box has nothing to show (Recharts drops null values), so the line stays hidden
// too instead of standing alone; the strip below already shows the device was off.
function TelemetryCursor({ points, payload }: { points?: { x: number; y: number }[]; payload?: readonly { value?: unknown }[] }) {
  if (!points || points.length < 2 || !payload?.some((p) => typeof p.value === "number")) return null;
  const [a, b] = points as [{ x: number; y: number }, { x: number; y: number }];
  return <path d={`M${a.x},${a.y}L${b.x},${b.y}`} stroke="hsl(var(--muted-foreground))" strokeOpacity={0.5} pointerEvents="none" />;
}

// A slim timeline under the chart: when the device reported (online) and when it didn't.
function AvailabilityStrip({ from, to, gaps, fmt }: { from: number; to: number; gaps: { x1: number; x2: number }[]; fmt: (t: number) => string }) {
  const span = to - from;
  const segs: { x1: number; x2: number; on: boolean }[] = [];
  let cur = from;
  for (const g of gaps) {
    const a = Math.max(from, g.x1);
    const b = Math.min(to, g.x2);
    if (a > cur) segs.push({ x1: cur, x2: a, on: true });
    if (b > a) segs.push({ x1: a, x2: b, on: false });
    cur = Math.max(cur, b);
  }
  if (cur < to) segs.push({ x1: cur, x2: to, on: true });
  const at = (t: number) => `${((t - from) / span) * 100}%`;
  return (
    <div className="relative h-2" style={{ marginLeft: PLOT_LEFT, marginRight: PLOT_RIGHT }} role="img" aria-label="When the device was reporting">
      <div className="absolute inset-0 overflow-hidden rounded-full">
        {segs.map((s) => (
          <div
            key={s.x1}
            className={cn("absolute inset-y-0", s.on ? "bg-success/60" : "bg-muted-foreground/20")}
            style={{ left: at(s.x1), width: `calc(${at(s.x2)} - ${at(s.x1)})` }}
            title={
              s.on
                ? `Online ${fmt(s.x1)} – ${fmt(s.x2)}`
                : `Off ${formatDuration((s.x2 - s.x1) / 1000)} (no samples), ${fmt(s.x1)} – ${fmt(s.x2)}`
            }
          />
        ))}
      </div>
    </div>
  );
}

// Ticks on round wall-clock times: every 10 min (1h), every 3 h (24h), each local midnight (7d).
function telemetryTicks(from: number, to: number, range: Range): number[] {
  const out: number[] = [];
  const d = new Date(from);
  if (range === "168") {
    d.setHours(24, 0, 0, 0);
    for (; d.getTime() <= to; d.setDate(d.getDate() + 1)) out.push(d.getTime());
  } else {
    const stepMin = range === "1" ? 10 : 180;
    d.setSeconds(0, 0);
    const mins = d.getHours() * 60 + d.getMinutes();
    d.setHours(0, Math.ceil(mins / stepMin) * stepMin, 0, 0);
    for (; d.getTime() <= to; d.setMinutes(d.getMinutes() + stepMin)) out.push(d.getTime());
  }
  return out;
}

function Telemetry({ id }: { id: string }) {
  const [hours, setHours] = useState<Range>("24");
  const [hidden, setHidden] = useState<Set<Series>>(new Set());
  const { data, dataUpdatedAt, isLoading, error, refetch } = useQuery({
    queryKey: qk.deviceTelemetry(id, Number(hours)),
    queryFn: () => api.devices.telemetry(id, Number(hours)),
    refetchInterval: 60_000,
  });

  // The window is the full range ending when the data was fetched, so 1h always spans an hour.
  const to = dataUpdatedAt || Date.now();
  const from = to - Number(hours) * HOUR;
  const { rows, gaps, samples } = useMemo(() => {
    const interval = (data?.interval_sec ?? 60) * 1000;
    const maxGap = interval * 3; // a few missed reports in a row: asleep, off or offline
    const samples: TelemetryRow[] = (data?.items ?? []).map((p) => ({
      t: new Date(p.ts).getTime(),
      CPU: p.cpu_pct,
      Memory: p.mem_pct,
      Disk: p.disk_pct,
      Battery: p.battery_pct,
    }));
    const rows: TelemetryRow[] = [];
    const gaps: { x1: number; x2: number }[] = [];
    const empty = (t: number): TelemetryRow => ({ t, CPU: null, Memory: null, Disk: null, Battery: null });
    let prev: number | null = null;
    for (const s of samples) {
      if (prev !== null && s.t - prev > maxGap) {
        gaps.push({ x1: prev, x2: s.t });
        rows.push(empty(prev + interval)); // a null row breaks the line instead of bridging the gap
      }
      rows.push(s);
      prev = s.t;
    }
    if (samples.length) {
      if (samples[0]!.t - from > maxGap) gaps.unshift({ x1: from, x2: samples[0]!.t });
      if (to - prev! > maxGap) gaps.push({ x1: prev!, x2: to });
    }
    return { rows, gaps, samples };
  }, [data, from, to]);

  const hasBattery = samples.some((p) => p.Battery != null);
  const series = SERIES.filter((s) => s.key !== "Battery" || hasBattery);
  const stat = (k: Series) => {
    const v = samples.map((p) => p[k]).filter((x): x is number => x != null);
    if (!v.length) return null;
    return { avg: v.reduce((a, b) => a + b, 0) / v.length, peak: Math.max(...v), low: Math.min(...v), first: v[0]!, last: v[v.length - 1]! };
  };
  const cpu = stat("CPU");
  const mem = stat("Memory");
  const disk = stat("Disk");
  const bat = stat("Battery");
  const label = RANGES[hours];
  const ticks = telemetryTicks(from, to, hours);
  const fmt = (t: number) => new Date(t).toLocaleString(undefined, hours === "168" ? { month: "short", day: "numeric" } : { hour: "2-digit", minute: "2-digit" });
  const stripFmt = (t: number) =>
    new Date(t).toLocaleString(undefined, hours === "168" ? { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" } : { hour: "2-digit", minute: "2-digit" });
  const toggle = (k: Series) =>
    setHidden((cur) => {
      const next = new Set(cur);
      if (next.has(k)) next.delete(k);
      else next.add(k);
      return next;
    });

  if (error) return <ErrorState error={error} onRetry={refetch} />;
  const pts = (n: number) => `${n > 0 ? "+" : ""}${n.toFixed(1)} pts`;
  return (
    <div className="space-y-4">
      {samples.length > 0 && (
        <div className={cn("grid grid-cols-1 gap-4 sm:grid-cols-2", bat ? "lg:grid-cols-4" : "lg:grid-cols-3")}>
          <Metric icon={Cpu} label={`CPU · ${label} average`} value={cpu?.avg} sub={cpu ? `Peak ${formatPct(cpu.peak)}` : undefined} limits={RESOURCE_LIMITS.cpu} />
          <Metric icon={MemoryStick} label={`Memory · ${label} average`} value={mem?.avg} sub={mem ? `Peak ${formatPct(mem.peak)}` : undefined} limits={RESOURCE_LIMITS.mem} />
          <Metric
            icon={HardDrive}
            label="System disk · latest"
            value={disk?.last}
            sub={disk ? `${pts(disk.last - disk.first)} over ${label}` : undefined}
            limits={RESOURCE_LIMITS.disk}
          />
          {bat && <Metric icon={BatteryMedium} label="Battery · latest" charge value={bat.last} sub={`Lowest ${formatPct(bat.low)} over ${label}`} />}
        </div>
      )}
      <Card>
        <CardHeader className="flex-row flex-wrap items-center justify-between gap-3">
          <CardTitle>Utilization</CardTitle>
          <Segmented value={hours} onChange={setHours} options={Object.entries(RANGES).map(([v, l]) => ({ value: v as Range, label: l }))} />
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Skeleton className="h-80" />
          ) : samples.length < 2 ? (
            <div className="flex h-80 items-center justify-center text-sm text-muted-foreground">
              No samples in the last {label}. The agent reports every minute while the device is on.
            </div>
          ) : (
            <>
              <div className="mb-3 flex flex-wrap gap-2" role="group" aria-label="Show or hide series">
                {series.map((s) => {
                  const on = !hidden.has(s.key);
                  return (
                    <button
                      key={s.key}
                      type="button"
                      aria-pressed={on}
                      onClick={() => toggle(s.key)}
                      className={cn(
                        "inline-flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                        on ? "border-border bg-card text-foreground hover:bg-muted" : "border-dashed border-border text-muted-foreground line-through hover:text-foreground",
                      )}
                      title={on ? `Hide ${s.key}` : `Show ${s.key}`}
                    >
                      <svg width="14" height="4" aria-hidden className={cn(!on && "opacity-40")}>
                        <line x1="0" y1="2" x2="14" y2="2" stroke={s.color} strokeWidth="2.5" strokeDasharray={s.dash} />
                      </svg>
                      {s.key}
                    </button>
                  );
                })}
              </div>
              <div className="h-80">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={rows} margin={{ left: 0, right: PLOT_RIGHT }}>
                    <CartesianGrid stroke="hsl(var(--border))" vertical={false} />
                    <XAxis
                      dataKey="t"
                      type="number"
                      scale="time"
                      domain={[from, to]}
                      allowDataOverflow
                      ticks={ticks}
                      tickFormatter={fmt}
                      fontSize={11}
                      stroke="hsl(var(--muted-foreground))"
                      tickLine={false}
                      axisLine={false}
                      minTickGap={24}
                    />
                    <YAxis width={PLOT_LEFT} domain={[0, 100]} ticks={[0, 25, 50, 75, 100]} allowDataOverflow unit="%" fontSize={11} stroke="hsl(var(--muted-foreground))" tickLine={false} axisLine={false} />
                    <Tooltip
                      cursor={<TelemetryCursor />}
                      labelFormatter={(t) => new Date(Number(t)).toLocaleString()}
                      formatter={(v) => (typeof v === "number" ? `${v.toFixed(1)}%` : String(v))}
                      contentStyle={{ background: "hsl(var(--popover))", border: "1px solid hsl(var(--border))", borderRadius: 8, fontSize: 12 }}
                    />
                    {series.map((s) => (
                      <Line
                        key={s.key}
                        type="linear"
                        dataKey={s.key}
                        hide={hidden.has(s.key)}
                        stroke={s.color}
                        strokeDasharray={s.dash}
                        dot={false}
                        strokeWidth={s.width}
                        connectNulls={false}
                        isAnimationActive={false}
                      />
                    ))}
                  </LineChart>
                </ResponsiveContainer>
              </div>
              <AvailabilityStrip from={from} to={to} gaps={gaps} fmt={stripFmt} />
              <p className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground" style={{ marginLeft: PLOT_LEFT }}>
                <span className="inline-flex items-center gap-1.5">
                  <span className="h-2 w-3 rounded-full bg-success/60" /> Online
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <span className="h-2 w-3 rounded-full bg-muted-foreground/20" /> Off, asleep or offline (no samples)
                </span>
                {hours === "168" && <span>Points are 5-minute averages</span>}
              </p>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

type CommandFilter = "all" | "failed" | "scripts" | "actions" | "updates";
const AGENT_TYPES: CommandType[] = ["update_agent", "uninstall_agent"];
// The one-time update for old agents is sent as a script but is an agent update.
const commandKind = (c: Command): Exclude<CommandFilter, "all" | "failed"> =>
  AGENT_TYPES.includes(c.type) || commandLabel(c.type, c.payload).startsWith("Update agent") ? "updates" : c.type === "run_script" ? "scripts" : "actions";
const FAILED_STATUSES = ["failed", "timeout"];

function Commands({ id }: { id: string }) {
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.deviceCommands(id), queryFn: () => api.devices.commands(id) });
  const can = useCan();
  const [open, setOpen] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<CommandFilter>("all");
  const all = useMemo(() => data?.items ?? [], [data]);
  const counts = useMemo(
    () => ({
      all: all.length,
      failed: all.filter((c) => FAILED_STATUSES.includes(c.status)).length,
      scripts: all.filter((c) => commandKind(c) === "scripts").length,
      actions: all.filter((c) => commandKind(c) === "actions").length,
      updates: all.filter((c) => commandKind(c) === "updates").length,
    }),
    [all],
  );
  const items = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return all.filter((c) => {
      if (filter === "failed" ? !FAILED_STATUSES.includes(c.status) : filter !== "all" && commandKind(c) !== filter) return false;
      if (!needle) return true;
      return `${commandLabel(c.type, c.payload)} ${c.issued_by_email ?? c.source} ${c.status}`.toLowerCase().includes(needle);
    });
  }, [all, q, filter]);
  const filtered = filter !== "all" || q.trim() !== "";
  const pills: { value: CommandFilter; label: string }[] = [
    { value: "all", label: "All" },
    { value: "failed", label: "Failed" },
    { value: "scripts", label: "Scripts" },
    { value: "actions", label: "Actions" },
    { value: "updates", label: "Agent updates" },
  ];
  if (error) return <ErrorState error={error} onRetry={refetch} />;
  return (
    <Card className="overflow-hidden">
      <div className="flex flex-wrap items-center gap-3 border-b border-border p-3">
        <div className="relative w-full max-w-xs">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search commands" className="pl-8" aria-label="Search commands" />
        </div>
        {/* On narrow screens the pills scroll sideways rather than squashing into two-line labels. */}
        <div className="max-w-full overflow-x-auto [&_button]:whitespace-nowrap">
          <Segmented
            value={filter}
            onChange={setFilter}
            options={pills.map((p) => ({ value: p.value, label: `${p.label} (${counts[p.value]})` }))}
          />
        </div>
        {filtered && (
          <span className="text-xs text-muted-foreground">
            {items.length} of {all.length} commands
          </span>
        )}
      </div>
      {isLoading ? (
        <TableSkeleton rows={4} cols={4} />
      ) : !all.length ? (
        <EmptyState icon={Terminal} title="No commands yet" description="Scripts and actions you send to this device appear here with their output." />
      ) : !items.length ? (
        <EmptyState
          icon={Search}
          title="No commands match"
          description={filter === "failed" && !q ? "Nothing has failed on this device." : "Try another search or filter."}
          action={
            <Button
              variant="outline"
              onClick={() => {
                setQ("");
                setFilter("all");
              }}
            >
              Clear filters
            </Button>
          }
        />
      ) : (
        <ul className="divide-y divide-border">
          {items.map((c: Command) => {
            const expanded = open === c.id;
            const who = c.issued_by_email ?? titleCase(c.source);
            return (
              <li key={c.id}>
                {/* Fixed columns so operator, time and status line up whatever the badge width. */}
                <button
                  type="button"
                  onClick={() => setOpen(expanded ? null : c.id)}
                  aria-expanded={expanded}
                  className="grid w-full grid-cols-[1rem_minmax(0,1fr)_6.5rem] items-center gap-3 px-4 py-2.5 text-left text-sm hover:bg-muted/50 sm:grid-cols-[1rem_minmax(0,1fr)_14rem_4.5rem_6.5rem]"
                >
                  {expanded ? <ChevronDown className="h-4 w-4 text-muted-foreground" /> : <ChevronRight className="h-4 w-4 text-muted-foreground" />}
                  <span className="min-w-0">
                    <span className="block truncate font-medium">{commandLabel(c.type, c.payload)}</span>
                    {/* Phones: the time sits under the name so the name keeps its width. */}
                    <span className="block text-xs text-muted-foreground sm:hidden">{relativeTimeShort(c.created_at)}</span>
                  </span>
                  <span className="hidden truncate text-xs text-muted-foreground sm:block" title={who}>
                    {who}
                  </span>
                  <span className="hidden text-right text-xs text-muted-foreground tabular sm:block" title={formatDateTime(c.created_at)}>
                    {relativeTimeShort(c.created_at)}
                  </span>
                  <span>
                    <CommandStatusBadge status={c.status} />
                  </span>
                </button>
                {expanded && (
                  <div className="border-t border-border bg-muted/20 px-4 py-3 pl-11">
                    <CommandOutput cmd={c} onRerun={can("commands.run") ? () => setFilter("all") : undefined} />
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

/**
 * Typed client for the UEM REST API (docs/API.md, v1).
 *
 * - All URLs are relative (/api/...) so the same build works behind Caddy and
 *   behind the Next.js dev/standalone rewrites.
 * - Mutating requests carry X-CSRF-Token copied from the readable uem_csrf cookie.
 * - Errors come back as {"error": "..."}; they are thrown as ApiError.
 * - A 401 redirects the browser to /login (except for the auth endpoints themselves).
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export const PERMISSIONS = [
  "devices.read",
  "devices.write",
  "commands.run",
  "scripts.write",
  "compliance.write",
  "tasks.write",
  "enrollment.write",
  "users.write",
  "webhooks.write",
  "alerts.write",
  "audit.read",
  "settings.write",
] as const;
export type Permission = (typeof PERMISSIONS)[number];

export type Role = "owner" | "admin" | "helpdesk" | "auditor";
export const ROLES: Role[] = ["owner", "admin", "helpdesk", "auditor"];

export type User = {
  id: string;
  email: string;
  name: string;
  role: Role;
  created_at: string;
  last_login_at: string | null;
};

export type Org = { id: string; name: string; server_url: string; created_at: string };

export type Me = { user: User; org: Org; permissions: string[] };

export type DeviceStatus = "online" | "offline";
export type ComplianceState = "compliant" | "non_compliant" | "unknown";
export type OsFamily = "windows" | "macos" | "linux" | (string & {});

export type Device = {
  id: string;
  name: string;
  hostname: string;
  os_family: OsFamily;
  os_name: string;
  os_version: string;
  os_build: string;
  arch: string;
  serial: string;
  model: string;
  manufacturer: string;
  agent_version: string;
  ip_address: string;
  mac_address: string;
  logged_in_user: string;
  status: DeviceStatus;
  last_seen_at: string | null;
  enrolled_at: string;
  compliance_state: ComplianceState;
  group_id: string | null;
  group_name: string | null;
  tags: string[];
  cpu_pct: number | null;
  mem_pct: number | null;
  disk_pct: number | null;
  battery_pct: number | null;
};

export type Disk = {
  mount: string;
  fstype: string;
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
};

export type NetInterface = { name: string; mac: string; addrs: string[]; up: boolean };

export type Inventory = {
  collected_at: string;
  os: {
    family: string;
    name: string;
    version: string;
    build: string;
    kernel: string;
    arch: string;
    uptime_seconds: number;
    boot_time: string;
  };
  hardware: {
    manufacturer: string;
    model: string;
    serial: string;
    cpu_model: string;
    cpu_cores: number;
    cpu_threads: number;
    memory_total_bytes: number;
    memory_used_bytes: number;
    disks: Disk[];
    battery: { present: boolean; percent: number; state: string; health_pct: number | null };
  };
  network: {
    hostname: string;
    primary_ip: string;
    primary_mac: string;
    wifi_ssid: string | null;
    interfaces: NetInterface[];
  };
  security: {
    disk_encrypted: boolean | null;
    encryption_method: string | null;
    firewall_enabled: boolean | null;
    screen_lock_enabled: boolean | null;
    pending_updates: number | null;
    antivirus: string | null;
  };
  users: { logged_in: string[] };
};

export type DeviceDetail = Device & { inventory: Inventory | null };

export type SoftwareItem = {
  name: string;
  version: string;
  publisher: string;
  source: string;
  install_date: string | null;
};

export type TelemetryPoint = {
  ts: string;
  cpu_pct: number;
  mem_pct: number;
  disk_pct: number;
  battery_pct: number | null;
};

export type CommandType =
  | "run_script"
  | "reboot"
  | "shutdown"
  | "lock"
  | "display_message"
  | "refresh_inventory"
  | "uninstall_agent";

export const COMMAND_TYPES: CommandType[] = [
  "run_script",
  "reboot",
  "shutdown",
  "lock",
  "display_message",
  "refresh_inventory",
  "uninstall_agent",
];

export type Shell = "bash" | "sh" | "zsh" | "powershell" | "cmd";
export const SHELLS: Shell[] = ["bash", "sh", "zsh", "powershell", "cmd"];

export type RunScriptPayload =
  | { script_id: string }
  | { shell: Shell; script: string; timeout_seconds?: number };
export type PowerPayload = { delay_seconds?: number };
export type MessagePayload = { title: string; message: string };

export type CommandStatus =
  | "queued"
  | "sent"
  | "running"
  | "succeeded"
  | "failed"
  | "timeout"
  | "cancelled";

export const TERMINAL_COMMAND_STATUSES: CommandStatus[] = ["succeeded", "failed", "timeout", "cancelled"];

export type Command = {
  id: string;
  device_id: string;
  device_name: string;
  type: CommandType;
  payload: Record<string, unknown>;
  status: CommandStatus;
  exit_code: number | null;
  stdout: string;
  stderr: string;
  error: string | null;
  issued_by_email: string | null;
  source: "user" | "scheduler" | "compliance";
  created_at: string;
  sent_at: string | null;
  completed_at: string | null;
};

export type CommandRequest = { type: CommandType; payload: Record<string, unknown> };

export type Group = {
  id: string;
  name: string;
  description: string;
  blocked_apps: string[];
  device_count: number;
  created_at: string;
};
export type GroupInput = { name: string; description: string; blocked_apps: string[] };

export type EnrollmentToken = {
  id: string;
  token: string;
  description: string;
  group_id: string | null;
  group_name: string | null;
  expires_at: string | null;
  max_uses: number | null;
  uses: number;
  revoked: boolean;
  created_at: string;
};
export type EnrollmentTokenInput = {
  description: string;
  group_id?: string;
  expires_in_hours?: number;
  max_uses?: number;
};
export type InstallCommands = { linux: string; macos: string; windows: string };

export type Script = {
  id: string;
  name: string;
  description: string;
  shell: Shell;
  body: string;
  timeout_seconds: number;
  created_at: string;
  updated_at: string;
};
export type ScriptInput = Pick<Script, "name" | "description" | "shell" | "body" | "timeout_seconds">;

export type ConditionOp =
  | "eq"
  | "neq"
  | "gt"
  | "gte"
  | "lt"
  | "lte"
  | "version_gte"
  | "version_lt"
  | "contains"
  | "not_contains"
  | "is_true"
  | "is_false";

export type Condition = { field: string; op: ConditionOp; value: unknown };
export type Severity = "low" | "medium" | "high" | "critical";
export const SEVERITIES: Severity[] = ["low", "medium", "high", "critical"];

export type RuleActions = {
  alert: boolean;
  webhook: boolean;
  tag: string | null;
  command: CommandRequest | null;
};

export type Rule = {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  severity: Severity;
  os_families: string[];
  group_id: string | null;
  match: "all" | "any";
  conditions: Condition[];
  actions: RuleActions;
  created_at: string;
  updated_at: string;
  violating_devices: number;
};
export type RuleInput = Omit<Rule, "id" | "created_at" | "updated_at" | "violating_devices">;

export type ComplianceResult = {
  rule_id: string;
  rule_name: string;
  severity: Severity;
  passed: boolean;
  details: string;
  evaluated_at: string;
};

export type FieldType = "bool" | "number" | "string" | "version" | "list";
export type ComplianceField = { field: string; label: string; type: FieldType; ops: string[]; unit?: string };

export type TaskActionType = CommandType | "compliance_check";
export type TaskTarget = { group_id: string | null; device_ids: string[]; all: boolean };
export type Task = {
  id: string;
  name: string;
  enabled: boolean;
  cron: string;
  target: TaskTarget;
  action: { type: TaskActionType; payload: Record<string, unknown> };
  last_run_at: string | null;
  next_run_at: string | null;
  created_at: string;
};
export type TaskInput = Pick<Task, "name" | "enabled" | "cron" | "target" | "action">;

export type Alert = {
  id: string;
  device_id: string | null;
  device_name: string | null;
  rule_id: string | null;
  severity: Severity;
  title: string;
  message: string;
  status: "open" | "resolved";
  created_at: string;
  resolved_at: string | null;
};

export type Dashboard = {
  devices_total: number;
  online: number;
  offline: number;
  compliant: number;
  non_compliant: number;
  unknown: number;
  open_alerts: number;
  by_os: { os_family: string; count: number }[];
  recent_alerts: Alert[];
  recent_commands: Command[];
};

export type AuditEntry = {
  id: number;
  ts: string;
  actor_type: "user" | "system" | "device";
  actor_id: string | null;
  actor_email: string | null;
  action: string;
  target_type: string | null;
  target_id: string | null;
  details: Record<string, unknown>;
  ip: string | null;
  hash: string;
};
export type AuditPage = { items: AuditEntry[]; next_before_id: number | null };
export type AuditVerify = { valid: boolean; checked: number; broken_at_id: number | null };
export type AuditFilters = { action?: string; actor?: string; target_id?: string; limit?: number; before_id?: number };

export type UserInput = { email: string; name: string; role: Role; password: string };
export type UserPatch = { name?: string; role?: Role; password?: string };

export const WEBHOOK_EVENTS = [
  "alert.created",
  "device.enrolled",
  "device.offline",
  "command.completed",
  "audit.*",
] as const;

export type Webhook = {
  id: string;
  name: string;
  url: string;
  secret: string;
  events: string[];
  enabled: boolean;
  created_at: string;
  last_status: number | null;
  last_delivery_at: string | null;
};
export type WebhookInput = { name: string; url: string; secret?: string; events: string[]; enabled: boolean };

export type DeviceFilters = {
  q?: string;
  status?: string;
  os?: string;
  compliance?: string;
  group_id?: string;
};

export type Items<T> = { items: T[] };
export type Ok = { ok: boolean };

export type LiveEvent =
  | { type: "device.updated"; data: Device }
  | { type: "command.updated"; data: Command }
  | { type: "alert.created"; data: Alert };

// ---------------------------------------------------------------------------
// Fetch wrapper
// ---------------------------------------------------------------------------

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export function getCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const prefix = `${name}=`;
  for (const part of document.cookie.split(";")) {
    const c = part.trim();
    if (c.startsWith(prefix)) return decodeURIComponent(c.slice(prefix.length));
  }
  return null;
}

const NO_REDIRECT_PATHS = ["/api/auth/login", "/api/auth/signup", "/api/auth/logout"];
const PUBLIC_PAGES = ["/login", "/signup"];

let redirecting = false;
function redirectToLogin() {
  if (typeof window === "undefined" || redirecting) return;
  if (PUBLIC_PAGES.some((p) => window.location.pathname.startsWith(p))) return;
  redirecting = true;
  const next = window.location.pathname + window.location.search;
  window.location.assign(`/login?next=${encodeURIComponent(next)}`);
}

type Query = Record<string, string | number | boolean | null | undefined>;

export function withQuery(path: string, query?: Query): string {
  if (!query) return path;
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === "") continue;
    sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `${path}?${s}` : path;
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET" && method !== "HEAD") {
    const csrf = getCookie("uem_csrf");
    if (csrf) headers["X-CSRF-Token"] = csrf;
  }

  let res: Response;
  try {
    res = await fetch(path, {
      method,
      headers,
      credentials: "same-origin",
      body: body !== undefined ? JSON.stringify(body) : undefined,
      cache: "no-store",
    });
  } catch {
    throw new ApiError(0, "Unable to reach the server. Check your connection.");
  }

  const text = await res.text();
  let data: unknown = undefined;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = undefined;
    }
  }

  const errMsg =
    data && typeof data === "object" && "error" in data && typeof (data as { error: unknown }).error === "string"
      ? (data as { error: string }).error
      : null;

  if (!res.ok || errMsg) {
    if (res.status === 401 && !NO_REDIRECT_PATHS.includes(path)) redirectToLogin();
    throw new ApiError(res.status, errMsg || defaultMessage(res.status));
  }
  return data as T;
}

function defaultMessage(status: number): string {
  if (status === 401) return "You are not signed in.";
  if (status === 403) return "You don't have permission to do that.";
  if (status === 404) return "Not found.";
  if (status >= 500) return `Server error (${status}).`;
  return `Request failed (${status}).`;
}

const get = <T>(path: string, query?: Query) => request<T>("GET", withQuery(path, query));
const post = <T>(path: string, body?: unknown) => request<T>("POST", path, body ?? {});
const patch = <T>(path: string, body: unknown) => request<T>("PATCH", path, body);
const del = <T>(path: string) => request<T>("DELETE", path);

const enc = encodeURIComponent;

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

export const api = {
  auth: {
    signup: (b: { org_name: string; name: string; email: string; password: string }) =>
      post<Me>("/api/auth/signup", b),
    login: (b: { email: string; password: string }) => post<Me>("/api/auth/login", b),
    logout: () => post<Ok>("/api/auth/logout"),
    me: () => get<Me>("/api/auth/me"),
  },
  dashboard: () => get<Dashboard>("/api/dashboard"),
  devices: {
    list: (f: DeviceFilters = {}) => get<Items<Device>>("/api/devices", f),
    get: (id: string) => get<DeviceDetail>(`/api/devices/${enc(id)}`),
    update: (id: string, b: { name?: string; group_id?: string | null; tags?: string[] }) =>
      patch<Device>(`/api/devices/${enc(id)}`, b),
    retire: (id: string) => del<Ok>(`/api/devices/${enc(id)}`),
    software: (id: string) => get<Items<SoftwareItem>>(`/api/devices/${enc(id)}/software`),
    telemetry: (id: string, hours: number) =>
      get<Items<TelemetryPoint>>(`/api/devices/${enc(id)}/telemetry`, { hours }),
    compliance: (id: string) => get<Items<ComplianceResult>>(`/api/devices/${enc(id)}/compliance`),
    commands: (id: string) => get<Items<Command>>(`/api/devices/${enc(id)}/commands`),
    sendCommand: (id: string, b: CommandRequest) => post<Command>(`/api/devices/${enc(id)}/commands`, b),
  },
  commands: {
    bulk: (b: { device_ids: string[] } & CommandRequest) => post<Items<Command>>("/api/commands/bulk", b),
    get: (id: string) => get<Command>(`/api/commands/${enc(id)}`),
  },
  groups: {
    list: () => get<Items<Group>>("/api/groups"),
    create: (b: GroupInput) => post<Group>("/api/groups", b),
    update: (id: string, b: Partial<GroupInput>) => patch<Group>(`/api/groups/${enc(id)}`, b),
    remove: (id: string) => del<Ok>(`/api/groups/${enc(id)}`),
  },
  enrollment: {
    list: () => get<Items<EnrollmentToken>>("/api/enrollment-tokens"),
    create: (b: EnrollmentTokenInput) => post<EnrollmentToken>("/api/enrollment-tokens", b),
    revoke: (id: string) => del<Ok>(`/api/enrollment-tokens/${enc(id)}`),
    install: (id: string) => get<InstallCommands>(`/api/enrollment-tokens/${enc(id)}/install`),
  },
  scripts: {
    list: () => get<Items<Script>>("/api/scripts"),
    get: (id: string) => get<Script>(`/api/scripts/${enc(id)}`),
    create: (b: ScriptInput) => post<Script>("/api/scripts", b),
    update: (id: string, b: Partial<ScriptInput>) => patch<Script>(`/api/scripts/${enc(id)}`, b),
    remove: (id: string) => del<Ok>(`/api/scripts/${enc(id)}`),
  },
  compliance: {
    fields: () => get<Items<ComplianceField>>("/api/compliance/fields"),
    rules: () => get<Items<Rule>>("/api/compliance/rules"),
    rule: (id: string) => get<Rule>(`/api/compliance/rules/${enc(id)}`),
    create: (b: RuleInput) => post<Rule>("/api/compliance/rules", b),
    update: (id: string, b: Partial<RuleInput>) => patch<Rule>(`/api/compliance/rules/${enc(id)}`, b),
    remove: (id: string) => del<Ok>(`/api/compliance/rules/${enc(id)}`),
    evaluate: () => post<{ evaluated: number }>("/api/compliance/evaluate"),
  },
  tasks: {
    list: () => get<Items<Task>>("/api/tasks"),
    create: (b: TaskInput) => post<Task>("/api/tasks", b),
    update: (id: string, b: Partial<TaskInput>) => patch<Task>(`/api/tasks/${enc(id)}`, b),
    remove: (id: string) => del<Ok>(`/api/tasks/${enc(id)}`),
    run: (id: string) => post<unknown>(`/api/tasks/${enc(id)}/run`),
  },
  alerts: {
    list: (status?: "open" | "resolved") => get<Items<Alert>>("/api/alerts", { status }),
    resolve: (id: string) => post<unknown>(`/api/alerts/${enc(id)}/resolve`),
  },
  audit: {
    list: (f: AuditFilters = {}) => get<AuditPage>("/api/audit", f),
    verify: () => get<AuditVerify>("/api/audit/verify"),
    exportUrl: "/api/audit/export.csv",
  },
  users: {
    list: () => get<Items<User>>("/api/users"),
    create: (b: UserInput) => post<User>("/api/users", b),
    update: (id: string, b: UserPatch) => patch<User>(`/api/users/${enc(id)}`, b),
    remove: (id: string) => del<Ok>(`/api/users/${enc(id)}`),
  },
  webhooks: {
    list: () => get<Items<Webhook>>("/api/webhooks"),
    create: (b: WebhookInput) => post<Webhook>("/api/webhooks", b),
    update: (id: string, b: Partial<WebhookInput>) => patch<Webhook>(`/api/webhooks/${enc(id)}`, b),
    remove: (id: string) => del<Ok>(`/api/webhooks/${enc(id)}`),
    test: (id: string) => post<unknown>(`/api/webhooks/${enc(id)}/test`),
  },
  org: {
    get: () => get<Org>("/api/org"),
    update: (b: { name?: string; server_url?: string }) => patch<Org>("/api/org", b),
  },
};

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "Something went wrong.";
}

/** Query keys, centralised so the live hook can patch/invalidate them. */
export const qk = {
  me: ["me"] as const,
  dashboard: ["dashboard"] as const,
  devices: (f?: DeviceFilters) => (f ? (["devices", f] as const) : (["devices"] as const)),
  device: (id: string) => ["device", id] as const,
  deviceSoftware: (id: string) => ["device", id, "software"] as const,
  deviceTelemetry: (id: string, hours: number) => ["device", id, "telemetry", hours] as const,
  deviceCompliance: (id: string) => ["device", id, "compliance"] as const,
  deviceCommands: (id: string) => ["device", id, "commands"] as const,
  command: (id: string) => ["command", id] as const,
  groups: ["groups"] as const,
  enrollment: ["enrollment-tokens"] as const,
  scripts: ["scripts"] as const,
  complianceFields: ["compliance", "fields"] as const,
  complianceRules: ["compliance", "rules"] as const,
  tasks: ["tasks"] as const,
  alerts: (status?: string) => (status ? (["alerts", status] as const) : (["alerts"] as const)),
  audit: (f?: Omit<AuditFilters, "before_id">) => (f ? (["audit", f] as const) : (["audit"] as const)),
  users: ["users"] as const,
  webhooks: ["webhooks"] as const,
  org: ["org"] as const,
};

"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import {
  AlertCircle,
  Apple,
  ChevronRight,
  Download,
  Monitor,
  Search,
  Server,
  X,
  type LucideIcon,
} from "lucide-react";
import type {
  CommandStatus,
  ComplianceState,
  DeviceStatus,
  Severity,
} from "@/lib/api";
import { errorMessage } from "@/lib/api";
import { cn, osLabel, titleCase } from "@/lib/utils";
import { Badge, type BadgeVariant } from "./ui/badge";
import { Button } from "./ui/button";
import { Card } from "./ui/card";

/**
 * BEACON's mark: a teal ribbon folded into a shield (a nod to The V's folded-ribbon logo) around the
 * signal light. Flat fills in the company teals. The dot follows the sidebar accent (dark green on
 * light, neon on dark); pass `dotClassName` for a fixed surface, e.g. "fill-neon" on the dark login.
 * Source artwork: public/logo.svg (light) and public/logo-dark.svg.
 */
export function BrandMark({
  className,
  dotClassName = "fill-nav-accent",
}: {
  className?: string;
  dotClassName?: string;
}) {
  return (
    <svg
      viewBox="0 0 64 64"
      aria-hidden
      className={cn("aspect-square h-8 w-auto shrink-0 select-none", className)}
    >
      <path d="M56 9 L54.5 33 L45 31.5 L46 17 Z" fill="#00a4cc" />
      <path d="M54.5 33 L36.5 55.4 L32 47 L45 31.5 Z" fill="#00889c" />
      <path d="M39.65 51.48 L36.5 55.4 L32 47 Z" fill="#005870" />
      <path d="M19 31.5 L9.5 33 L32 61 L36.5 55.4 L32 47 Z" fill="#00ccec" />
      <path d="M9.5 33 L8 9 L18 17 L19 31.5 Z" fill="#00a4cc" />
      <path d="M8 9 L32 3 L56 9 L46 17 L32 13.5 L18 17 Z" fill="#00889c" />
      <path d="M8 9 L18 17 L13 20 Z" fill="#004858" />
      <path d="M56 9 L51.5 13.5 L46 17 Z" fill="#005870" />
      <circle cx="32" cy="27.5" r="5" className={dotClassName} />
    </svg>
  );
}

export type HeadlineStat = {
  label: string;
  value: React.ReactNode;
  icon: LucideIcon;
  tone?: "default" | "brand" | "success" | "danger" | "warning";
  /** Makes the figure a button, e.g. to open the list behind it. */
  onClick?: () => void;
  /** Label for a small action pill under the figure, e.g. "Review list"; needs onClick. */
  cta?: string;
  hint?: string;
};

const headlineTone = {
  default: "text-foreground",
  brand: "text-primary",
  success: "text-success",
  danger: "text-destructive",
  warning: "text-warning",
};

export function PageHeader({
  title,
  description,
  actions,
  stats,
  statsAside,
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
  /** Large headline figures shown under the title. */
  stats?: HeadlineStat[];
  /** Shown at the right end of the stats row, e.g. a search box. */
  statsAside?: React.ReactNode;
}) {
  return (
    <div className="mb-7">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <span
            className="mb-2.5 block h-1 w-8 rounded-full bg-brand"
            aria-hidden
          />
          <h1 className="text-2xl font-semibold tracking-tight sm:text-[28px]">
            {title}
          </h1>
          {description && (
            <p className="mt-1.5 max-w-3xl text-sm text-muted-foreground">
              {description}
            </p>
          )}
        </div>
        {actions && (
          <div className="flex flex-wrap items-center gap-2">{actions}</div>
        )}
      </div>
      {(stats?.length || statsAside) && (
        <div className="mt-5 flex flex-wrap items-center justify-between gap-x-8 gap-y-4">
          {stats?.length ? <HeadlineStats stats={stats} /> : null}
          {statsAside}
        </div>
      )}
    </div>
  );
}

/** Large headline figures with icons, e.g. under a page title. */
export function HeadlineStats({ stats, className }: { stats: HeadlineStat[]; className?: string }) {
  return (
    <dl className={cn("flex flex-wrap gap-x-8 gap-y-3", className)}>
      {stats.map((s) => {
        const body = (
          <>
            <span className="flex h-9 w-9 items-center justify-center rounded-xl border border-edge/70 bg-card text-muted-foreground dark:border-edge">
              <s.icon className="h-4 w-4" />
            </span>
            <div className="text-left leading-tight">
              <dd
                className={cn(
                  "text-xl font-semibold tracking-tight tabular",
                  headlineTone[s.tone ?? "default"],
                )}
              >
                {s.value}
              </dd>
              <dt className="text-xs text-muted-foreground">{s.label}</dt>
              {s.cta && (
                <span className="mt-1 inline-flex items-center gap-0.5 rounded-full bg-primary/10 px-2 py-0.5 text-[11px] font-semibold text-primary">
                  {s.cta} <ChevronRight className="h-3 w-3" />
                </span>
              )}
            </div>
          </>
        );
        return s.onClick ? (
          // A div (not a button) so the dt/dd stay valid children of the dl.
          <div
            key={s.label}
            role="button"
            tabIndex={0}
            onClick={s.onClick}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                s.onClick?.();
              }
            }}
            title={s.hint}
            className="-m-1.5 flex cursor-pointer items-center gap-2.5 rounded-xl p-1.5 outline-none transition-colors hover:bg-muted/70 focus-visible:ring-2 focus-visible:ring-ring/50"
          >
            {body}
          </div>
        ) : (
          <div
            key={s.label}
            className="flex items-center gap-2.5"
            title={s.hint}
          >
            {body}
          </div>
        );
      })}
    </dl>
  );
}

/** Search pill: opens the device list filtered by the typed text. */
export function DeviceSearch({ className }: { className?: string }) {
  const router = useRouter();
  const [q, setQ] = React.useState("");
  return (
    <form
      role="search"
      className={cn("relative w-full sm:w-72", className)}
      onSubmit={(e) => {
        e.preventDefault();
        const v = q.trim();
        router.push(v ? `/devices?q=${encodeURIComponent(v)}` : "/devices");
      }}
    >
      <Search className="pointer-events-none absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
      <input
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder="Search devices…"
        aria-label="Search devices"
        className="h-11 w-full rounded-full border border-edge/70 bg-card pl-11 pr-4 text-sm shadow-card outline-none transition-all placeholder:text-muted-foreground focus:border-ring/50 focus:ring-2 focus:ring-ring/30"
      />
    </form>
  );
}

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}: {
  icon: LucideIcon;
  title: string;
  description?: React.ReactNode;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center px-6 py-14 text-center",
        className,
      )}
    >
      <div className="relative mb-4 flex h-14 w-14 items-center justify-center">
        <span className="absolute inset-0 rounded-full bg-brand/10" />
        <span className="absolute inset-2 rounded-full bg-brand/15" />
        <Icon className="relative h-5 w-5 text-primary" />
      </div>
      <p className="font-medium">{title}</p>
      {description && (
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          {description}
        </p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function ErrorState({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry?: () => void;
}) {
  return (
    <Card className="flex flex-col items-center px-6 py-10 text-center">
      <AlertCircle className="mb-2 h-5 w-5 text-destructive" />
      <p className="font-medium">Couldn&apos;t load this data</p>
      <p className="mt-1 text-sm text-muted-foreground">
        {errorMessage(error)}
      </p>
      {onRetry && (
        <Button variant="outline" size="sm" className="mt-4" onClick={onRetry}>
          Try again
        </Button>
      )}
    </Card>
  );
}

export function StatusBadge({ status }: { status: DeviceStatus }) {
  return (
    <Badge variant={status === "online" ? "success" : "neutral"} dot>
      {status === "online" ? "Online" : "Offline"}
    </Badge>
  );
}

const complianceVariant: Record<ComplianceState, BadgeVariant> = {
  compliant: "success",
  non_compliant: "danger",
  unknown: "warning",
};

export function ComplianceBadge({ state }: { state: ComplianceState }) {
  return (
    <Badge variant={complianceVariant[state] ?? "neutral"}>
      {titleCase(state === "non_compliant" ? "non-compliant" : state)}
    </Badge>
  );
}

const severityVariant: Record<Severity, BadgeVariant> = {
  low: "neutral",
  medium: "warning",
  high: "danger",
  critical: "danger",
};

export function SeverityBadge({ severity }: { severity: Severity }) {
  return (
    <Badge
      variant={severityVariant[severity] ?? "neutral"}
      className={severity === "critical" ? "font-semibold" : undefined}
    >
      {titleCase(severity)}
    </Badge>
  );
}

const commandVariant: Record<CommandStatus, BadgeVariant> = {
  queued: "neutral",
  sent: "primary",
  running: "primary",
  succeeded: "success",
  failed: "danger",
  timeout: "warning",
  cancelled: "neutral",
};

export function CommandStatusBadge({ status }: { status: CommandStatus }) {
  return (
    <Badge
      variant={commandVariant[status] ?? "neutral"}
      dot={status === "running" || status === "sent"}
    >
      {titleCase(status)}
    </Badge>
  );
}

export function OsIcon({ os, className }: { os: string; className?: string }) {
  const Icon = os === "macos" ? Apple : os === "linux" ? Server : Monitor;
  return (
    <Icon
      className={cn("h-4 w-4 text-muted-foreground", className)}
      aria-label={osLabel(os)}
    />
  );
}

/** Thin usage bar; turns amber/red as it fills. */
export function UsageBar({
  value,
  className,
  charge,
  quiet,
  warnAt = 75,
  alertAt = 90,
}: {
  value: number | null | undefined;
  className?: string;
  /** A charge level (battery): low is red/amber and high is healthy, unlike usage. */
  charge?: boolean;
  /** A soft blue below warnAt, so values that need attention stand out (for long lists). */
  quiet?: boolean;
  warnAt?: number;
  alertAt?: number;
}) {
  const v = value == null ? 0 : Math.max(0, Math.min(100, value));
  const color = charge
    ? v <= 20
      ? "bg-destructive"
      : v <= 40
        ? "bg-warning"
        : "bg-success dark:bg-neon"
    : v >= alertAt
      ? "bg-destructive"
      : v >= warnAt
        ? "bg-warning"
        : quiet
          ? "bg-brand/60 dark:bg-brand/60"
          : "bg-brand dark:bg-gradient-to-r dark:from-brand dark:to-neon";
  return (
    <div
      className={cn(
        "h-1.5 w-full overflow-hidden rounded-full bg-muted",
        className,
      )}
    >
      <div
        className={cn("h-full rounded-full transition-all", color)}
        style={{ width: `${v}%` }}
      />
    </div>
  );
}

/** Label/value rows for detail panels. */
export function KeyValues({
  items,
}: {
  items: [React.ReactNode, React.ReactNode][];
}) {
  return (
    <dl className="grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-6 gap-y-2.5 text-sm">
      {items.map(([k, v], i) => (
        <React.Fragment key={i}>
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="min-w-0 break-words">
            {v === "" || v == null ? (
              <span className="text-muted-foreground">—</span>
            ) : (
              v
            )}
          </dd>
        </React.Fragment>
      ))}
    </dl>
  );
}

/** Free-form list editor: type and press Enter or comma to add. */
export function TagInput({
  value,
  onChange,
  placeholder,
  id,
}: {
  value: string[];
  onChange: (v: string[]) => void;
  placeholder?: string;
  id?: string;
}) {
  const [draft, setDraft] = React.useState("");
  const add = (raw: string) => {
    const parts = raw
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean)
      .filter((s) => !value.includes(s));
    if (parts.length) onChange([...value, ...parts]);
    setDraft("");
  };
  return (
    <div className="flex min-h-9 flex-wrap items-center gap-1.5 rounded-md border border-input bg-card px-2 py-1.5 shadow-sm focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/40">
      {value.map((t) => (
        <span
          key={t}
          className="inline-flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 font-mono text-xs"
        >
          {t}
          <button
            type="button"
            onClick={() => onChange(value.filter((x) => x !== t))}
            aria-label={`Remove ${t}`}
          >
            <X className="h-3 w-3 text-muted-foreground hover:text-foreground" />
          </button>
        </span>
      ))}
      <input
        id={id}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === ",") {
            e.preventDefault();
            add(draft);
          } else if (e.key === "Backspace" && !draft && value.length) {
            onChange(value.slice(0, -1));
          }
        }}
        onBlur={() => draft && add(draft)}
        placeholder={value.length ? "" : placeholder}
        className="min-w-[8rem] flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
      />
    </div>
  );
}

export function StatCard({
  label,
  value,
  sub,
  icon: Icon,
  tone = "default",
}: {
  label: string;
  value: React.ReactNode;
  sub?: React.ReactNode;
  icon: LucideIcon;
  tone?: "default" | "success" | "danger" | "warning";
}) {
  const toneClass = {
    default: "text-primary bg-brand/10",
    success: "text-success bg-success/10",
    danger: "text-destructive bg-destructive/10",
    warning: "text-warning bg-warning/10",
  }[tone];
  return (
    <Card className="p-5 transition-all duration-200 hover:shadow-lift motion-safe:hover:-translate-y-0.5">
      <div className="flex items-center gap-2.5">
        <span className={cn("rounded-lg p-2", toneClass)}>
          <Icon className="h-4 w-4" />
        </span>
        <p className="text-sm font-medium text-muted-foreground">{label}</p>
      </div>
      <p className="mt-3 text-3xl font-semibold tracking-tight tabular">
        {value}
      </p>
      {sub && <p className="mt-1 text-xs text-muted-foreground">{sub}</p>}
    </Card>
  );
}

/** A download link styled as an outline button, for CSV exports. */
export function ExportLink({
  href,
  title,
  className,
  label = "Export CSV",
}: {
  href: string;
  title?: string;
  className?: string;
  label?: string;
}) {
  return (
    <a
      href={href}
      download
      title={title}
      className={cn(
        "inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-card px-3.5 text-sm font-medium shadow-sm transition-colors hover:bg-muted [&_svg]:size-4",
        className,
      )}
    >
      <Download /> {label}
    </a>
  );
}

"use client";

import * as React from "react";
import { AlertCircle, Apple, Download, Monitor, Server, X, type LucideIcon } from "lucide-react";
import type { CommandStatus, ComplianceState, DeviceStatus, Severity } from "@/lib/api";
import { errorMessage } from "@/lib/api";
import { cn, osLabel, titleCase } from "@/lib/utils";
import { Badge, type BadgeVariant } from "./ui/badge";
import { Button } from "./ui/button";
import { Card } from "./ui/card";

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: React.ReactNode;
  description?: React.ReactNode;
  actions?: React.ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
      <div className="min-w-0">
        <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
        {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
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
    <div className={cn("flex flex-col items-center justify-center px-6 py-14 text-center", className)}>
      <div className="mb-3 rounded-full bg-muted p-3">
        <Icon className="h-5 w-5 text-muted-foreground" />
      </div>
      <p className="font-medium">{title}</p>
      {description && <p className="mt-1 max-w-sm text-sm text-muted-foreground">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  return (
    <Card className="flex flex-col items-center px-6 py-10 text-center">
      <AlertCircle className="mb-2 h-5 w-5 text-destructive" />
      <p className="font-medium">Couldn&apos;t load this data</p>
      <p className="mt-1 text-sm text-muted-foreground">{errorMessage(error)}</p>
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
  return <Badge variant={complianceVariant[state] ?? "neutral"}>{titleCase(state === "non_compliant" ? "non-compliant" : state)}</Badge>;
}

const severityVariant: Record<Severity, BadgeVariant> = {
  low: "neutral",
  medium: "warning",
  high: "danger",
  critical: "danger",
};

export function SeverityBadge({ severity }: { severity: Severity }) {
  return (
    <Badge variant={severityVariant[severity] ?? "neutral"} className={severity === "critical" ? "font-semibold" : undefined}>
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
    <Badge variant={commandVariant[status] ?? "neutral"} dot={status === "running" || status === "sent"}>
      {titleCase(status)}
    </Badge>
  );
}

export function OsIcon({ os, className }: { os: string; className?: string }) {
  const Icon = os === "macos" ? Apple : os === "linux" ? Server : Monitor;
  return <Icon className={cn("h-4 w-4 text-muted-foreground", className)} aria-label={osLabel(os)} />;
}

/** Thin usage bar; turns amber/red as it fills. */
export function UsageBar({ value, className }: { value: number | null | undefined; className?: string }) {
  const v = value == null ? 0 : Math.max(0, Math.min(100, value));
  const color = v >= 90 ? "bg-destructive" : v >= 75 ? "bg-warning" : "bg-primary";
  return (
    <div className={cn("h-1.5 w-full overflow-hidden rounded-full bg-muted", className)}>
      <div className={cn("h-full rounded-full transition-all", color)} style={{ width: `${v}%` }} />
    </div>
  );
}

/** Label/value rows for detail panels. */
export function KeyValues({ items }: { items: [React.ReactNode, React.ReactNode][] }) {
  return (
    <dl className="grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-6 gap-y-2.5 text-sm">
      {items.map(([k, v], i) => (
        <React.Fragment key={i}>
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="min-w-0 break-words">{v === "" || v == null ? <span className="text-muted-foreground">—</span> : v}</dd>
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
        <span key={t} className="inline-flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
          {t}
          <button type="button" onClick={() => onChange(value.filter((x) => x !== t))} aria-label={`Remove ${t}`}>
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
    default: "text-primary bg-primary/10",
    success: "text-success bg-success/10",
    danger: "text-destructive bg-destructive/10",
    warning: "text-warning bg-warning/10",
  }[tone];
  return (
    <Card className="p-4">
      <div className="flex items-start justify-between">
        <p className="text-sm text-muted-foreground">{label}</p>
        <span className={cn("rounded-md p-1.5", toneClass)}>
          <Icon className="h-4 w-4" />
        </span>
      </div>
      <p className="mt-2 text-2xl font-semibold tracking-tight tabular">{value}</p>
      {sub && <p className="mt-0.5 text-xs text-muted-foreground">{sub}</p>}
    </Card>
  );
}

/** A download link styled as an outline button, for CSV exports. */
export function ExportLink({ href, title, className }: { href: string; title?: string; className?: string }) {
  return (
    <a
      href={href}
      download
      title={title}
      className={cn("inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-card px-3.5 text-sm font-medium shadow-sm transition-colors hover:bg-muted [&_svg]:size-4", className)}
    >
      <Download /> Export CSV
    </a>
  );
}

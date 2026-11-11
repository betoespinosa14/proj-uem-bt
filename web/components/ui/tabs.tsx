"use client";
import * as React from "react";
import { cn } from "@/lib/utils";

type Ctx = { value: string; setValue: (v: string) => void; id: string };
const TabsCtx = React.createContext<Ctx | null>(null);
const useTabs = () => {
  const c = React.useContext(TabsCtx);
  if (!c) throw new Error("Tabs components must be used inside <Tabs>");
  return c;
};

export function Tabs({
  value,
  defaultValue,
  onValueChange,
  className,
  children,
}: {
  value?: string;
  defaultValue?: string;
  onValueChange?: (v: string) => void;
  className?: string;
  children: React.ReactNode;
}) {
  const [inner, setInner] = React.useState(defaultValue ?? "");
  const id = React.useId();
  const current = value ?? inner;
  const setValue = React.useCallback(
    (v: string) => {
      if (value === undefined) setInner(v);
      onValueChange?.(v);
    },
    [value, onValueChange],
  );
  return (
    <TabsCtx.Provider value={{ value: current, setValue, id }}>
      <div className={className}>{children}</div>
    </TabsCtx.Provider>
  );
}

export function TabsList({ className, children }: { className?: string; children: React.ReactNode }) {
  const ref = React.useRef<HTMLDivElement>(null);
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
    const tabs = Array.from(ref.current?.querySelectorAll<HTMLButtonElement>('[role="tab"]:not([disabled])') ?? []);
    const i = tabs.indexOf(document.activeElement as HTMLButtonElement);
    if (i === -1) return;
    const next = tabs[(i + (e.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length];
    next?.focus();
    next?.click();
  };
  return (
    <div
      ref={ref}
      role="tablist"
      onKeyDown={onKeyDown}
      className={cn("flex w-fit max-w-full items-center gap-1 overflow-x-auto rounded-full bg-muted/70 p-1", className)}
    >
      {children}
    </div>
  );
}

export function TabsTrigger({
  value,
  className,
  children,
  disabled,
}: {
  value: string;
  className?: string;
  children: React.ReactNode;
  disabled?: boolean;
}) {
  const t = useTabs();
  const active = t.value === value;
  return (
    <button
      type="button"
      role="tab"
      id={`${t.id}-tab-${value}`}
      aria-selected={active}
      aria-controls={`${t.id}-panel-${value}`}
      tabIndex={active ? 0 : -1}
      disabled={disabled}
      onClick={() => t.setValue(value)}
      className={cn(
        "relative inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-3.5 py-1.5 text-sm font-medium transition-all duration-200",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-50",
        // Same accent as the pickers and the sidebar; the underline keeps page tabs distinct from filters.
        active
          ? "bg-card font-semibold text-nav-accent shadow-[inset_0_-2px_0_hsl(var(--nav-accent))]"
          : "text-muted-foreground hover:text-foreground",
        className,
      )}
    >
      {children}
    </button>
  );
}

export function TabsContent({ value, className, children }: { value: string; className?: string; children: React.ReactNode }) {
  const t = useTabs();
  if (t.value !== value) return null;
  return (
    <div
      role="tabpanel"
      id={`${t.id}-panel-${value}`}
      aria-labelledby={`${t.id}-tab-${value}`}
      className={cn("pt-5 motion-safe:animate-fade-in", className)}
    >
      {children}
    </div>
  );
}

/** Compact pill-style segmented control (e.g. 1h / 24h / 7d). */
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  className,
}: {
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: React.ReactNode }[];
  className?: string;
}) {
  return (
    <div className={cn("inline-flex rounded-full bg-muted/70 p-1", className)}>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          aria-pressed={value === o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            "rounded-full px-3 py-1 text-xs font-medium transition-all duration-200",
            // Selected looks like the sidebar's current page: accent text on an accent tint.
            value === o.value
              ? "bg-nav-accent/15 font-semibold text-nav-accent ring-1 ring-inset ring-nav-accent/40"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

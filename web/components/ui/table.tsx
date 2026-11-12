"use client";

import * as React from "react";
import { ChevronDown, ChevronsUpDown, ChevronUp } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * A table that scrolls sideways when it doesn't fit. A soft fade on the left or
 * right edge shows that more columns are hidden that way.
 */
export function Table({
  className,
  containerClassName,
  stickyHeader,
  ...props
}: React.TableHTMLAttributes<HTMLTableElement> & {
  containerClassName?: string;
  /** Keep the column headers in view under the top bar while the page scrolls (only while the table fits). */
  stickyHeader?: boolean;
}) {
  const ref = React.useRef<HTMLDivElement>(null);
  const [edges, setEdges] = React.useState({ left: false, right: false });
  React.useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const update = () => {
      const max = el.scrollWidth - el.clientWidth;
      const next = { left: el.scrollLeft > 1, right: el.scrollLeft < max - 1 };
      setEdges((e) => (e.left === next.left && e.right === next.right ? e : next));
    };
    update();
    el.addEventListener("scroll", update, { passive: true });
    const ro = new ResizeObserver(update);
    ro.observe(el);
    if (el.firstElementChild) ro.observe(el.firstElementChild);
    return () => {
      el.removeEventListener("scroll", update);
      ro.disconnect();
    };
  }, []);
  // A sideways-scroll box would trap a sticky header, so it only becomes one when the
  // table is too wide; while the table fits, overflow is clipped instead.
  const overflowing = edges.left || edges.right;
  const fade = "pointer-events-none absolute inset-y-0 w-6 from-card to-transparent transition-opacity duration-200";
  return (
    <div className="relative w-full">
      <div
        ref={ref}
        className={cn("w-full", stickyHeader && !overflowing ? "sticky-head overflow-x-clip" : "overflow-x-auto", containerClassName)}
      >
        <table className={cn("w-full caption-bottom text-sm", className)} {...props} />
      </div>
      <div aria-hidden className={cn(fade, "left-0 bg-gradient-to-r", edges.left ? "opacity-100" : "opacity-0")} />
      <div aria-hidden className={cn(fade, "right-0 bg-gradient-to-l", edges.right ? "opacity-100" : "opacity-0")} />
    </div>
  );
}
export function THead({ className, ...props }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <thead className={cn("[&_tr]:border-b [&_tr]:border-border/70", className)} {...props} />;
}
export function TBody({ className, ...props }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody className={cn("[&_tr:last-child]:border-0", className)} {...props} />;
}
export function TR({ className, ...props }: React.HTMLAttributes<HTMLTableRowElement>) {
  return (
    <tr
      className={cn("border-b border-border/60 transition-colors hover:bg-muted/60 data-[state=selected]:bg-brand/10", className)}
      {...props}
    />
  );
}
export function TH({ className, ...props }: React.ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th
      className={cn(
        "h-9 whitespace-nowrap px-2.5 text-left align-middle text-[11px] font-semibold uppercase tracking-wider text-muted-foreground",
        className,
      )}
      {...props}
    />
  );
}
export function TD({ className, ...props }: React.TdHTMLAttributes<HTMLTableCellElement>) {
  return <td className={cn("px-2.5 py-2 align-middle", className)} {...props} />;
}

/**
 * A column header that sorts. The active column shows its direction (up for
 * ascending, down for descending); the others show a faint up/down arrow at
 * rest, stronger on hover, so it's clear they can be sorted too.
 */
export function SortButton<K extends string>({
  label,
  k,
  sort,
  onSort,
}: {
  label: string;
  k: K;
  sort: { key: K; dir: "asc" | "desc" };
  onSort: (k: K) => void;
}) {
  const active = sort.key === k;
  const Icon = !active ? ChevronsUpDown : sort.dir === "asc" ? ChevronUp : ChevronDown;
  return (
    <button
      type="button"
      onClick={() => onSort(k)}
      className={cn(
        "group -mx-1 inline-flex items-center gap-1 rounded px-1 uppercase tracking-wider hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        active && "text-foreground",
      )}
      title={`Sort by ${label.toLowerCase()}`}
    >
      {label}
      <Icon className={cn("h-3 w-3 shrink-0", active ? "opacity-100" : "opacity-30 group-hover:opacity-70 group-focus-visible:opacity-70")} aria-hidden />
    </button>
  );
}

"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * A table that scrolls sideways when it doesn't fit. A soft fade on the left or
 * right edge shows that more columns are hidden that way.
 */
export function Table({ className, ...props }: React.TableHTMLAttributes<HTMLTableElement>) {
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
  const fade = "pointer-events-none absolute inset-y-0 w-10 from-card to-transparent transition-opacity duration-200";
  return (
    <div className="relative w-full">
      <div ref={ref} className="w-full overflow-x-auto">
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

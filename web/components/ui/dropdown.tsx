"use client";
import * as React from "react";
import { cn } from "@/lib/utils";

type Ctx = { open: boolean; setOpen: (o: boolean) => void };
const DropdownCtx = React.createContext<Ctx | null>(null);
const useDropdown = () => {
  const c = React.useContext(DropdownCtx);
  if (!c) throw new Error("Dropdown components must be used inside <Dropdown>");
  return c;
};

export function Dropdown({ children, className }: { children: React.ReactNode; className?: string }) {
  const [open, setOpen] = React.useState(false);
  const ref = React.useRef<HTMLDivElement>(null);
  React.useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);
  return (
    <DropdownCtx.Provider value={{ open, setOpen }}>
      <div ref={ref} className={cn("relative inline-block", className)}>
        {children}
      </div>
    </DropdownCtx.Provider>
  );
}

/** Wraps a single element (usually a Button) and toggles the menu on click. */
export function DropdownTrigger({ children }: { children: React.ReactElement<React.HTMLAttributes<HTMLElement>> }) {
  const { open, setOpen } = useDropdown();
  return React.cloneElement(children, {
    "aria-haspopup": "menu",
    "aria-expanded": open,
    onClick: (e: React.MouseEvent<HTMLElement>) => {
      children.props.onClick?.(e);
      setOpen(!open);
    },
  } as React.HTMLAttributes<HTMLElement>);
}

export function DropdownContent({
  children,
  align = "end",
  className,
}: {
  children: React.ReactNode;
  align?: "start" | "end";
  className?: string;
}) {
  const { open } = useDropdown();
  const ref = React.useRef<HTMLDivElement>(null);
  React.useEffect(() => {
    if (open) ref.current?.querySelector<HTMLElement>('[role="menuitem"]:not([disabled])')?.focus();
  }, [open]);
  if (!open) return null;
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    const items = Array.from(ref.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])') ?? []);
    const i = items.indexOf(document.activeElement as HTMLElement);
    const n = items[(i + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length];
    n?.focus();
  };
  return (
    <div
      ref={ref}
      role="menu"
      onKeyDown={onKeyDown}
      className={cn(
        "absolute z-40 mt-1 min-w-[12rem] overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-lg animate-fade-in",
        align === "end" ? "right-0" : "left-0",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function DropdownItem({
  children,
  onSelect,
  disabled,
  destructive,
  className,
}: {
  children: React.ReactNode;
  onSelect?: () => void;
  disabled?: boolean;
  destructive?: boolean;
  className?: string;
}) {
  const { setOpen } = useDropdown();
  return (
    <button
      type="button"
      role="menuitem"
      disabled={disabled}
      onClick={() => {
        setOpen(false);
        onSelect?.();
      }}
      className={cn(
        "flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none transition-colors",
        "hover:bg-muted focus:bg-muted disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:text-muted-foreground",
        destructive && "text-destructive [&_svg]:text-destructive",
        className,
      )}
    >
      {children}
    </button>
  );
}

export function DropdownSeparator() {
  return <div className="-mx-1 my-1 h-px bg-border" />;
}

export function DropdownLabel({ children }: { children: React.ReactNode }) {
  return <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">{children}</div>;
}

"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  AppWindow,
  ChevronDown,
  Bell,
  CalendarClock,
  FileCode2,
  FolderTree,
  History,
  LayoutDashboard,
  LogOut,
  Menu,
  Monitor,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  Plug,
  Settings,
  ShieldCheck,
  Sun,
  UserPlus,
  Users,
  X,
  type LucideIcon,
} from "lucide-react";
import { api, type Permission } from "@/lib/api";
import { APP_NAME } from "@/lib/config";
import { onLiveAlert, useLive, useLiveState } from "@/lib/live";
import { hasPermission, useMe } from "@/lib/session";
import { cn, titleCase } from "@/lib/utils";
import { BrandMark } from "./common";
import { Button } from "./ui/button";
import { Dropdown, DropdownContent, DropdownItem, DropdownSeparator, DropdownTrigger } from "./ui/dropdown";
import { Skeleton } from "./ui/skeleton";
import { toast } from "./toast";

type NavItem = { href: string; label: string; icon: LucideIcon; perm?: Permission };

const NAV: { section: string; items: NavItem[] }[] = [
  {
    section: "Fleet",
    items: [
      { href: "/", label: "Dashboard", icon: LayoutDashboard },
      { href: "/devices", label: "Devices", icon: Monitor },
      { href: "/enrollment", label: "Enrollment", icon: UserPlus },
      { href: "/groups", label: "Groups", icon: FolderTree },
      { href: "/usage", label: "App usage", icon: AppWindow },
    ],
  },
  {
    section: "Policy",
    items: [
      { href: "/compliance", label: "Compliance", icon: ShieldCheck },
      { href: "/scripts", label: "Scripts", icon: FileCode2 },
      { href: "/tasks", label: "Scheduled tasks", icon: CalendarClock },
      { href: "/alerts", label: "Alerts", icon: Bell },
    ],
  },
  {
    section: "Administration",
    items: [
      { href: "/audit", label: "Audit log", icon: History, perm: "audit.read" },
      { href: "/users", label: "Users & roles", icon: Users, perm: "users.write" },
      { href: "/integrations", label: "Integrations", icon: Plug, perm: "webhooks.write" },
      { href: "/settings", label: "Settings", icon: Settings },
    ],
  },
];

/**
 * The current page's marker, the same in collapsed and expanded mode: an accent bar (dark green in
 * light mode, neon in dark mode) 2px wide and 24px
 * tall, centered vertically, 4px left of the highlighted item (6px offset minus its 2px width).
 * Positioned against the item's <li>, which starts where the highlight starts.
 */
const ACTIVE_MARKER = "absolute -left-1.5 top-1/2 h-6 w-[2px] -translate-y-1/2 rounded-full bg-nav-accent";

function isActive(pathname: string, href: string) {
  return href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(href + "/");
}

function ThemeToggle() {
  const [dark, setDark] = useState(false);
  useEffect(() => setDark(document.documentElement.classList.contains("dark")), []);
  const toggle = () => {
    const next = !dark;
    setDark(next);
    document.documentElement.classList.toggle("dark", next);
    try {
      localStorage.setItem("theme", next ? "dark" : "light");
    } catch {
      /* storage unavailable: theme still applies for this page */
    }
  };
  return (
    <Button variant="ghost" size="icon" onClick={toggle} aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}>
      {dark ? <Sun /> : <Moon />}
    </Button>
  );
}

function LiveIndicator() {
  const state = useLiveState();
  const label = state === "open" ? "Live" : state === "connecting" ? "Connecting" : "Offline";
  return (
    <span className="hidden items-center gap-1.5 text-xs text-muted-foreground sm:inline-flex" title="Real-time updates">
      <span
        className={cn(
          "h-2 w-2 rounded-full",
          state === "open" ? "bg-success dark:bg-neon" : state === "connecting" ? "animate-pulse bg-warning" : "bg-muted-foreground/50",
        )}
      />
      {label}
    </span>
  );
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const qc = useQueryClient();
  const { data: me, isLoading, error } = useMe();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);
  useLive(!!me);

  // Remember whether the sidebar shows labels. Read after mount so server and client render the same.
  useEffect(() => {
    try {
      setExpanded(localStorage.getItem("sidebar") === "expanded");
    } catch {
      /* storage unavailable: start collapsed */
    }
  }, []);
  const toggleExpanded = () => {
    const next = !expanded;
    setExpanded(next);
    try {
      localStorage.setItem("sidebar", next ? "expanded" : "collapsed");
    } catch {
      /* storage unavailable: applies until reload */
    }
  };

  useEffect(() => setMobileOpen(false), [pathname]);
  useEffect(
    () =>
      onLiveAlert((a) => {
        toast.error(`New ${a.severity} alert`, a.title);
      }),
    [],
  );

  const logout = async () => {
    try {
      await api.auth.logout();
    } catch {
      /* session may already be gone */
    }
    qc.clear();
    router.replace("/login");
  };

  if (error && !me) {
    // 401s redirect to /login from the API client; other errors land here.
    return (
      <div className="flex min-h-screen items-center justify-center p-6 text-sm text-muted-foreground">
        Unable to load your session. <Button variant="link" onClick={() => location.reload()}>Retry</Button>
      </div>
    );
  }

  const navGroups = NAV.map((g) => ({ ...g, items: g.items.filter((i) => !i.perm || hasPermission(me, i.perm)) })).filter(
    (g) => g.items.length > 0,
  );

  const railTooltip =
    "pointer-events-none absolute left-full top-1/2 z-50 ml-3 -translate-y-1/2 whitespace-nowrap rounded-lg bg-popover px-2.5 py-1 text-xs font-medium text-popover-foreground opacity-0 shadow-lift transition-opacity duration-150 group-hover:opacity-100 group-focus-within:opacity-100";
  const railItem = (wide: boolean, active = false) =>
    cn(
      "flex items-center rounded-xl text-sm transition-all duration-200",
      "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/70",
      wide ? "w-full gap-3 px-3 py-2.5" : "h-11 w-11 justify-center",
      active
        ? "bg-card font-semibold text-nav-accent shadow-sm dark:bg-nav-accent/10 dark:shadow-none"
        : "text-nav-foreground/75 hover:bg-nav-foreground/5 hover:text-nav-foreground",
    );

  // `wide` shows labels (expanded sidebar and the mobile drawer). `toggle` adds the collapse/expand button.
  const rail = (wide: boolean, toggle: boolean) => (
    <nav className="flex h-full flex-col overflow-y-auto overflow-x-hidden px-3 pb-4 pt-6 text-nav-foreground" aria-label="Main">
      <Link href="/" className={cn("flex shrink-0 items-center gap-3", wide ? "px-2" : "justify-center")} title={APP_NAME}>
        <BrandMark className="h-9 shrink-0" />
        {wide && (
          <span className="leading-tight">
            <span className="flex items-center gap-1.5 text-[15px] font-bold tracking-[0.18em]">
              {APP_NAME}
              <span className="h-1.5 w-1.5 rounded-full bg-nav-accent" />
            </span>
            <span className="block text-[10px] font-medium uppercase tracking-wider text-nav-foreground/70">Endpoint oversight</span>
          </span>
        )}
      </Link>
      {/* Groups spread over the rail's height; the spacers shrink to a minimum on short screens, then it scrolls. */}
      <div className="flex flex-1 flex-col">
        {navGroups.map((group, gi) => (
          <div key={group.section} className="contents">
            <div className={cn("min-h-5 flex-1", gi === 0 ? "max-h-10" : "max-h-14")} aria-hidden />
            <div>
              {wide ? (
                <p className="mb-2 px-3 text-[10px] font-semibold uppercase tracking-[0.14em] text-nav-foreground/70">{group.section}</p>
              ) : (
                gi > 0 && <div className="mx-auto mb-3 h-px w-8 bg-nav-foreground/25" />
              )}
              <ul className={cn("flex flex-col gap-[clamp(4px,0.9vh,10px)]", !wide && "items-center")}>
                {group.items.map((item) => {
                  const active = isActive(pathname, item.href);
                  return (
                    <li key={item.href} className={cn("group relative", wide && "w-full")}>
                      {active && (
                        <span className={ACTIVE_MARKER} aria-hidden />
                      )}
                      <Link
                        href={item.href}
                        aria-current={active ? "page" : undefined}
                        aria-label={wide ? undefined : item.label}
                        className={railItem(wide, active)}
                      >
                        <item.icon className="h-5 w-5 shrink-0" />
                        {wide && item.label}
                      </Link>
                      {!wide && (
                        <span role="tooltip" className={railTooltip}>
                          {/* The section name keeps Fleet / Policy / Administration one hover away when collapsed. */}
                          <span className="block text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground" aria-hidden>
                            {group.section}
                          </span>
                          {item.label}
                        </span>
                      )}
                    </li>
                  );
                })}
              </ul>
            </div>
          </div>
        ))}
        <div className="min-h-5 flex-1" aria-hidden />
      </div>
      {toggle && (
        <div className={cn("shrink-0 border-t border-nav-foreground/10 pt-3", !wide && "flex justify-center")}>
          <div className={cn("group relative", wide && "w-full")}>
            <button
              type="button"
              onClick={toggleExpanded}
              className={railItem(wide)}
              aria-label={wide ? "Collapse sidebar" : "Expand sidebar"}
            >
              {wide ? <PanelLeftClose className="h-5 w-5 shrink-0" /> : <PanelLeftOpen className="h-5 w-5 shrink-0" />}
              {wide && <span>Collapse</span>}
            </button>
            {!wide && (
              <span role="tooltip" className={railTooltip}>
                Expand sidebar
              </span>
            )}
          </div>
        </div>
      )}
    </nav>
  );

  return (
    <div className="min-h-screen bg-canvas lg:bg-nav lg:bg-none">
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-30 hidden bg-nav transition-[width] duration-300 lg:block",
          expanded ? "w-60" : "w-[84px]",
        )}
      >
        {rail(expanded, true)}
      </aside>
      {mobileOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-black/50 motion-safe:animate-fade-in" onClick={() => setMobileOpen(false)} />
          <aside className="absolute inset-y-0 left-0 w-64 bg-nav shadow-xl motion-safe:animate-fade-in">
            <button
              type="button"
              className="absolute right-3 top-5 flex h-8 w-8 items-center justify-center rounded-lg text-nav-foreground/60 hover:bg-nav-foreground/10 hover:text-nav-foreground"
              onClick={() => setMobileOpen(false)}
              aria-label="Close menu"
            >
              <X className="h-4 w-4" />
            </button>
            {rail(true, false)}
          </aside>
        </div>
      )}
      <div className={cn("transition-[padding] duration-300 lg:py-3 lg:pr-3", expanded ? "lg:pl-60" : "lg:pl-[84px]")}>
        <div className="lg:h-[calc(100vh-1.5rem)] lg:overflow-y-auto lg:rounded-[28px] lg:bg-canvas lg:shadow-lift lg:ring-1 lg:ring-edge dark:lg:shadow-frame dark:lg:ring-0">
          <header className="sticky top-0 z-20 flex h-16 items-center gap-3 bg-[hsl(var(--canvas-from)/0.8)] px-4 backdrop-blur-md sm:px-6 lg:px-8">
            <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setMobileOpen(true)} aria-label="Open menu">
              <Menu />
            </Button>
            <div className="min-w-0 flex-1 truncate text-sm font-medium text-muted-foreground">
              {isLoading ? <Skeleton className="h-4 w-32" /> : me?.org.name}
            </div>
            <div className="ml-auto flex items-center gap-1.5">
              <LiveIndicator />
              <ThemeToggle />
              {me && <UserMenu name={me.user.name || me.user.email} email={me.user.email} role={titleCase(me.user.role)} onLogout={logout} />}
            </div>
          </header>
          <main key={pathname} className="mx-auto max-w-7xl px-4 pb-12 pt-3 sm:px-6 lg:px-8 motion-safe:animate-slide-in">
            {me ? children : <PageSkeleton />}
          </main>
        </div>
      </div>
    </div>
  );
}

function initials(name: string) {
  const parts = name.split(/[\s@._-]+/).filter(Boolean);
  return ((parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? "")).toUpperCase() || "?";
}

function UserMenu({ name, email, role, onLogout }: { name: string; email: string; role: string; onLogout: () => void }) {
  return (
    <Dropdown>
      <DropdownTrigger>
        <button
          type="button"
          className="ml-1 flex items-center gap-2 rounded-full p-0.5 pr-2 transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
          aria-label="Account menu"
        >
          <span className="flex h-9 w-9 items-center justify-center rounded-full bg-brand text-xs font-bold text-rail">
            {initials(name)}
          </span>
          <ChevronDown className="hidden h-4 w-4 text-muted-foreground sm:block" />
        </button>
      </DropdownTrigger>
      <DropdownContent>
        <div className="px-2 py-2">
          <p className="truncate text-sm font-semibold">{name}</p>
          {name !== email && <p className="truncate text-xs text-muted-foreground">{email}</p>}
          <p className="mt-1 text-xs text-muted-foreground">{role}</p>
        </div>
        <DropdownSeparator />
        <DropdownItem onSelect={onLogout}>
          <LogOut /> Sign out
        </DropdownItem>
      </DropdownContent>
    </Dropdown>
  );
}

function PageSkeleton() {
  return (
    <div className="space-y-4">
      <Skeleton className="h-7 w-48" />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-24" />
        ))}
      </div>
      <Skeleton className="h-64" />
    </div>
  );
}

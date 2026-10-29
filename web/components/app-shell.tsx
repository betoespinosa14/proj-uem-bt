"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  AppWindow,
  Bell,
  CalendarClock,
  FileCode2,
  FolderTree,
  LayoutDashboard,
  LogOut,
  Menu,
  Monitor,
  Moon,
  Plug,
  ScrollText,
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
import { Button } from "./ui/button";
import { Skeleton } from "./ui/skeleton";
import { toast } from "./toast";

type NavItem = { href: string; label: string; icon: LucideIcon; perm?: Permission };

const NAV: { section: string; items: NavItem[] }[] = [
  {
    section: "Fleet",
    items: [
      { href: "/", label: "Dashboard", icon: LayoutDashboard },
      { href: "/devices", label: "Devices", icon: Monitor },
      { href: "/usage", label: "App usage", icon: AppWindow },
      { href: "/groups", label: "Groups", icon: FolderTree },
      { href: "/enrollment", label: "Enrollment", icon: UserPlus },
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
      { href: "/audit", label: "Audit log", icon: ScrollText, perm: "audit.read" },
      { href: "/users", label: "Users & roles", icon: Users, perm: "users.write" },
      { href: "/integrations", label: "Integrations", icon: Plug, perm: "webhooks.write" },
      { href: "/settings", label: "Settings", icon: Settings },
    ],
  },
];

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
          state === "open" ? "bg-success" : state === "connecting" ? "animate-pulse bg-warning" : "bg-muted-foreground/50",
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
  useLive(!!me);

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

  const sidebar = (
    <nav className="flex h-full flex-col gap-6 overflow-y-auto px-3 py-4">
      <Link href="/" className="flex items-center gap-2 px-2">
        <span className="flex h-7 w-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <ShieldCheck className="h-4 w-4" />
        </span>
        <span className="font-semibold tracking-tight">{APP_NAME}</span>
      </Link>
      {NAV.map((group) => {
        const items = group.items.filter((i) => !i.perm || hasPermission(me, i.perm));
        if (!items.length) return null;
        return (
          <div key={group.section}>
            <p className="mb-1 px-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">{group.section}</p>
            <ul className="space-y-0.5">
              {items.map((item) => {
                const active = isActive(pathname, item.href);
                return (
                  <li key={item.href}>
                    <Link
                      href={item.href}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm transition-colors",
                        active ? "bg-primary/10 font-medium text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                      )}
                    >
                      <item.icon className="h-4 w-4" />
                      {item.label}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        );
      })}
    </nav>
  );

  return (
    <div className="min-h-screen">
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 border-r border-border bg-sidebar lg:block">{sidebar}</aside>
      {mobileOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <div className="absolute inset-0 bg-black/40" onClick={() => setMobileOpen(false)} />
          <aside className="absolute inset-y-0 left-0 w-64 border-r border-border bg-sidebar shadow-xl animate-fade-in">
            <Button variant="ghost" size="icon-sm" className="absolute right-2 top-3" onClick={() => setMobileOpen(false)} aria-label="Close menu">
              <X />
            </Button>
            {sidebar}
          </aside>
        </div>
      )}
      <div className="lg:pl-60">
        <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-border bg-background/80 px-4 backdrop-blur sm:px-6">
          <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setMobileOpen(true)} aria-label="Open menu">
            <Menu />
          </Button>
          <div className="min-w-0 flex-1 truncate text-sm font-medium">{isLoading ? <Skeleton className="h-4 w-32" /> : me?.org.name}</div>
          <LiveIndicator />
          <ThemeToggle />
          {me && (
            <div className="flex items-center gap-3 border-l border-border pl-3">
              <div className="hidden text-right leading-tight sm:block">
                <p className="text-sm font-medium">{me.user.name || me.user.email}</p>
                <p className="text-xs text-muted-foreground">{titleCase(me.user.role)}</p>
              </div>
              <Button variant="ghost" size="icon" onClick={logout} aria-label="Sign out" title="Sign out">
                <LogOut />
              </Button>
            </div>
          )}
        </header>
        <main className="mx-auto max-w-7xl px-4 py-6 sm:px-6">{me ? children : <PageSkeleton />}</main>
      </div>
    </div>
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

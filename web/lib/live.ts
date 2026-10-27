"use client";

import { useEffect, useSyncExternalStore } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import {
  qk,
  type Alert,
  type Command,
  type Dashboard,
  type Device,
  type DeviceDetail,
  type DeviceFilters,
  type Items,
  type LiveEvent,
} from "./api";
import { liveUrl } from "./config";

// ---------------------------------------------------------------------------
// Connection state store (read by the top bar indicator)
// ---------------------------------------------------------------------------

export type LiveState = "connecting" | "open" | "closed";
let state: LiveState = "closed";
const listeners = new Set<() => void>();
function setState(s: LiveState) {
  state = s;
  listeners.forEach((l) => l());
}
function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}
export function useLiveState(): LiveState {
  return useSyncExternalStore(
    subscribe,
    () => state,
    () => "closed",
  );
}

// Alert listeners (e.g. to raise a toast) without coupling this module to UI.
type AlertListener = (a: Alert) => void;
const alertListeners = new Set<AlertListener>();
export function onLiveAlert(fn: AlertListener) {
  alertListeners.add(fn);
  return () => {
    alertListeners.delete(fn);
  };
}

// ---------------------------------------------------------------------------
// Cache updaters
// ---------------------------------------------------------------------------

function matchesFilters(d: Device, f: DeviceFilters | undefined): boolean {
  if (!f) return true;
  if (f.status && d.status !== f.status) return false;
  if (f.os && d.os_family !== f.os) return false;
  if (f.compliance && d.compliance_state !== f.compliance) return false;
  if (f.group_id && d.group_id !== f.group_id) return false;
  return true;
}

function applyDevice(qc: QueryClient, d: Device, scheduleInvalidate: (key: readonly unknown[]) => void) {
  // Detail: merge, keep inventory (device.updated carries a Device, not a DeviceDetail).
  qc.setQueryData<DeviceDetail>(qk.device(d.id), (old) => (old ? { ...old, ...d } : old));

  // Lists: patch in place. If the device is new to a list, or no longer matches the
  // list's server-side filter, refetch that list (throttled).
  const lists = qc.getQueriesData<Items<Device>>({ queryKey: qk.devices() });
  for (const [key, data] of lists) {
    if (!data) continue;
    const filters = key[1] as DeviceFilters | undefined;
    const idx = data.items.findIndex((x) => x.id === d.id);
    if (idx === -1) {
      if (matchesFilters(d, filters)) scheduleInvalidate(key);
      continue;
    }
    if (!matchesFilters(d, filters)) {
      scheduleInvalidate(key);
    }
    const items = data.items.slice();
    items[idx] = { ...items[idx]!, ...d };
    qc.setQueryData<Items<Device>>(key, { ...data, items });
  }
  scheduleInvalidate(qk.dashboard);
}

function upsertCommand(list: Command[], c: Command): Command[] {
  const idx = list.findIndex((x) => x.id === c.id);
  if (idx === -1) return [c, ...list];
  const next = list.slice();
  next[idx] = c;
  return next;
}

function applyCommand(qc: QueryClient, c: Command) {
  qc.setQueryData<Command>(qk.command(c.id), c);
  qc.setQueryData<Items<Command>>(qk.deviceCommands(c.device_id), (old) =>
    old ? { ...old, items: upsertCommand(old.items, c) } : old,
  );
  qc.setQueryData<Dashboard>(qk.dashboard, (old) => {
    if (!old) return old;
    const idx = old.recent_commands.findIndex((x) => x.id === c.id);
    const recent =
      idx === -1 ? [c, ...old.recent_commands].slice(0, Math.max(old.recent_commands.length, 10)) : upsertCommand(old.recent_commands, c);
    return { ...old, recent_commands: recent };
  });
}

function applyAlert(qc: QueryClient, a: Alert) {
  qc.setQueryData<Items<Alert>>(qk.alerts("open"), (old) =>
    old && !old.items.some((x) => x.id === a.id) ? { ...old, items: [a, ...old.items] } : old,
  );
  void qc.invalidateQueries({ queryKey: qk.alerts() });
  void qc.invalidateQueries({ queryKey: qk.dashboard });
  if (a.device_id) void qc.invalidateQueries({ queryKey: qk.deviceCompliance(a.device_id) });
  void qc.invalidateQueries({ queryKey: qk.complianceRules });
  alertListeners.forEach((l) => l(a));
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Opens the /api/live WebSocket once (mount it in the authenticated app shell)
 * and keeps TanStack Query caches in sync. Reconnects with exponential backoff
 * (1s → 30s, with jitter); on reconnect it invalidates everything so state
 * missed while disconnected is refetched.
 */
export function useLive(enabled = true) {
  const qc = useQueryClient();

  useEffect(() => {
    if (!enabled) return;
    let ws: WebSocket | null = null;
    let attempt = 0;
    let stopped = false;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let hadConnection = false;

    // Throttled invalidation so a burst of telemetry doesn't hammer the API.
    const pending = new Map<string, readonly unknown[]>();
    let flushTimer: ReturnType<typeof setTimeout> | null = null;
    const scheduleInvalidate = (key: readonly unknown[]) => {
      pending.set(JSON.stringify(key), key);
      if (flushTimer) return;
      flushTimer = setTimeout(() => {
        flushTimer = null;
        const keys = [...pending.values()];
        pending.clear();
        keys.forEach((k) => void qc.invalidateQueries({ queryKey: k, exact: true }));
      }, 2000);
    };

    const connect = () => {
      if (stopped) return;
      setState("connecting");
      try {
        ws = new WebSocket(liveUrl());
      } catch {
        scheduleReconnect();
        return;
      }
      ws.onopen = () => {
        attempt = 0;
        setState("open");
        if (hadConnection) void qc.invalidateQueries();
        hadConnection = true;
      };
      ws.onmessage = (ev) => {
        let msg: LiveEvent;
        try {
          msg = JSON.parse(typeof ev.data === "string" ? ev.data : "") as LiveEvent;
        } catch {
          return;
        }
        if (!msg || typeof msg !== "object" || !("type" in msg)) return;
        switch (msg.type) {
          case "device.updated":
            applyDevice(qc, msg.data, scheduleInvalidate);
            break;
          case "command.updated":
            applyCommand(qc, msg.data);
            break;
          case "alert.created":
            applyAlert(qc, msg.data);
            break;
        }
      };
      ws.onclose = () => {
        ws = null;
        if (!stopped) scheduleReconnect();
      };
      ws.onerror = () => {
        // onclose follows; nothing else to do.
      };
    };

    const scheduleReconnect = () => {
      setState("closed");
      if (stopped) return;
      const base = Math.min(30000, 1000 * 2 ** attempt);
      const delay = base / 2 + Math.random() * (base / 2);
      attempt++;
      retryTimer = setTimeout(connect, delay);
    };

    const onVisible = () => {
      // Reconnect promptly when the tab becomes visible again.
      if (document.visibilityState === "visible" && !ws && !stopped) {
        if (retryTimer) clearTimeout(retryTimer);
        attempt = 0;
        connect();
      }
    };
    document.addEventListener("visibilitychange", onVisible);

    connect();
    return () => {
      stopped = true;
      document.removeEventListener("visibilitychange", onVisible);
      if (retryTimer) clearTimeout(retryTimer);
      if (flushTimer) clearTimeout(flushTimer);
      ws?.close();
      setState("closed");
    };
  }, [enabled, qc]);
}

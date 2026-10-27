"use client";

import { useSyncExternalStore } from "react";
import { AlertTriangle, CheckCircle2, Info, X } from "lucide-react";
import { cn } from "@/lib/utils";
import { errorMessage } from "@/lib/api";

type Kind = "success" | "error" | "info";
type Toast = { id: number; kind: Kind; title: string; body?: string };

let toasts: Toast[] = [];
let nextId = 1;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((l) => l());

function push(kind: Kind, title: string, body?: string) {
  const id = nextId++;
  toasts = [...toasts.slice(-4), { id, kind, title, body }];
  emit();
  setTimeout(() => dismiss(id), kind === "error" ? 7000 : 4000);
}

function dismiss(id: number) {
  toasts = toasts.filter((t) => t.id !== id);
  emit();
}

export const toast = {
  success: (title: string, body?: string) => push("success", title, body),
  info: (title: string, body?: string) => push("info", title, body),
  error: (title: string, err?: unknown) => push("error", title, err === undefined ? undefined : errorMessage(err)),
};

const icons = { success: CheckCircle2, error: AlertTriangle, info: Info };
const tone = { success: "text-success", error: "text-destructive", info: "text-primary" };

export function Toaster() {
  const list = useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => toasts,
    () => toasts,
  );
  return (
    <div aria-live="polite" className="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-[min(24rem,calc(100vw-2rem))] flex-col gap-2">
      {list.map((t) => {
        const Icon = icons[t.kind];
        return (
          <div
            key={t.id}
            role={t.kind === "error" ? "alert" : "status"}
            className="pointer-events-auto flex items-start gap-3 rounded-lg border border-border bg-popover p-3 text-sm shadow-lg animate-slide-in"
          >
            <Icon className={cn("mt-0.5 h-4 w-4 shrink-0", tone[t.kind])} />
            <div className="min-w-0 flex-1">
              <p className="font-medium">{t.title}</p>
              {t.body && <p className="mt-0.5 break-words text-muted-foreground">{t.body}</p>}
            </div>
            <button onClick={() => dismiss(t.id)} className="text-muted-foreground hover:text-foreground" aria-label="Dismiss">
              <X className="h-4 w-4" />
            </button>
          </div>
        );
      })}
    </div>
  );
}

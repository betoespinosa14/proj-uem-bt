"use client";

import { useState } from "react";
import { TriangleAlert } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, qk, SHELLS, type Command, type CommandRequest, type CommandType, type Shell } from "@/lib/api";
import { formatDateTime, titleCase } from "@/lib/utils";
import { Button } from "./ui/button";
import { Dialog } from "./ui/dialog";
import { Field, Input } from "./ui/input";
import { Select } from "./ui/select";
import { CodeEditor } from "./ui/textarea";
import { Segmented } from "./ui/tabs";
import { CommandStatusBadge } from "./common";
import { toast } from "./toast";

/** The agent version the server offers; cached for the session (it changes only on a release). */
export function useAgentLatest() {
  return useQuery({ queryKey: qk.agentLatest, queryFn: api.agents.latest, staleTime: 5 * 60_000 });
}

/** First line of the script the server sends to agents that predate self-update. */
const ONE_TIME_UPDATE = "# BEACON: one-time update";

export function commandLabel(type: string, payload: Record<string, unknown>): string {
  if (type === "update_agent") return typeof payload.version === "string" ? `Update agent to ${payload.version}` : "Update agent";
  if (type === "run_script" && typeof payload.script === "string" && payload.script.startsWith(ONE_TIME_UPDATE)) {
    return "Update agent (first time)";
  }
  if (type === "run_script") {
    const name = payload.script_name;
    return typeof name === "string" ? `Script: ${name}` : `Run ${String(payload.shell ?? "script")}`;
  }
  return titleCase(type);
}

const ACTION_COPY: Record<Exclude<CommandType, "run_script">, { title: string; confirm: string; danger?: boolean }> = {
  reboot: { title: "Reboot", confirm: "Reboot", danger: true },
  shutdown: { title: "Shut down", confirm: "Shut down", danger: true },
  lock: { title: "Lock screen", confirm: "Lock" },
  display_message: { title: "Send message", confirm: "Send" },
  refresh_inventory: { title: "Refresh inventory", confirm: "Refresh" },
  uninstall_agent: { title: "Uninstall agent", confirm: "Uninstall", danger: true },
  update_agent: { title: "Update agent", confirm: "Update" },
};

/**
 * Dialog that issues one command to one device or to many (bulk).
 * Script commands can use a library script or an ad-hoc body.
 */
// Disruptive commands need the device count typed in before they go to a large batch.
const DISRUPTIVE: CommandType[] = ["reboot", "shutdown", "lock", "run_script", "uninstall_agent"];
const BIG_BATCH = 10;

export function CommandDialog({
  open,
  onOpenChange,
  type,
  deviceIds,
  targetLabel,
  onIssued,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  type: CommandType;
  deviceIds: string[];
  targetLabel: string;
  onIssued?: (cmds: Command[]) => void;
}) {
  const qc = useQueryClient();
  const scripts = useQuery({ queryKey: qk.scripts, queryFn: api.scripts.list, enabled: open && type === "run_script" });
  const [mode, setMode] = useState<"library" | "adhoc">("library");
  const [typedCount, setTypedCount] = useState("");
  const [scriptId, setScriptId] = useState("");
  const [shell, setShell] = useState<Shell>("bash");
  const [body, setBody] = useState("");
  const [timeout, setTimeoutSecs] = useState(300);
  const [delay, setDelay] = useState(0);
  const [title, setTitle] = useState("Message from IT");
  const [message, setMessage] = useState("");

  const buildRequest = (): CommandRequest | string => {
    switch (type) {
      case "run_script":
        if (mode === "library") return scriptId ? { type, payload: { script_id: scriptId } } : "Choose a script.";
        return body.trim() ? { type, payload: { shell, script: body, timeout_seconds: timeout } } : "Enter a script.";
      case "reboot":
      case "shutdown":
        return { type, payload: { delay_seconds: delay } };
      case "display_message":
        return message.trim() ? { type, payload: { title, message } } : "Enter a message.";
      default:
        return { type, payload: {} };
    }
  };

  const mutation = useMutation({
    mutationFn: async (req: CommandRequest) => {
      if (deviceIds.length === 1) return [await api.devices.sendCommand(deviceIds[0]!, req)];
      return (await api.commands.bulk({ device_ids: deviceIds, ...req })).items;
    },
    onSuccess: (cmds) => {
      toast.success(
        cmds.length === 1 ? "Command queued" : `Command queued for ${cmds.length} devices`,
        "Results appear as devices respond.",
      );
      deviceIds.forEach((id) => void qc.invalidateQueries({ queryKey: qk.deviceCommands(id) }));
      onIssued?.(cmds);
      onOpenChange(false);
    },
    onError: (err) => toast.error("Command failed", err),
  });

  const submit = () => {
    // The button is disabled until the count is typed; this keeps any other way of
    // submitting (a future shortcut, say) from slipping past the large-batch guard.
    if (deviceIds.length > BIG_BATCH && DISRUPTIVE.includes(type) && typedCount.trim() !== String(deviceIds.length)) return;
    const req = buildRequest();
    if (typeof req === "string") {
      toast.error(req);
      return;
    }
    mutation.mutate(req);
  };

  const copy = type === "run_script" ? { title: "Run script", confirm: "Run", danger: false } : ACTION_COPY[type];
  const count = deviceIds.length;
  const guarded = count > BIG_BATCH && DISRUPTIVE.includes(type);
  const confirmLabel = count > 1 ? `${copy.confirm} ${count} devices` : copy.confirm;

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={copy.title}
      description={`Target: ${targetLabel}`}
      size={type === "run_script" ? "lg" : "sm"}
      footer={
        <>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant={copy.danger ? "destructive" : "default"}
            loading={mutation.isPending}
            disabled={guarded && typedCount.trim() !== String(count)}
            onClick={submit}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {type === "run_script" && (
        <div className="space-y-4">
          <Segmented
            value={mode}
            onChange={setMode}
            options={[
              { value: "library", label: "From library" },
              { value: "adhoc", label: "Ad-hoc script" },
            ]}
          />
          {mode === "library" ? (
            <Field label="Script" htmlFor="script">
              <Select
                id="script"
                value={scriptId}
                onValueChange={setScriptId}
                placeholder={scripts.isLoading ? "Loading…" : scripts.data?.items.length ? "Choose a script" : "No scripts in library"}
                options={(scripts.data?.items ?? []).map((s) => ({ value: s.id, label: `${s.name} (${s.shell})` }))}
              />
            </Field>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Shell" htmlFor="shell">
                  <Select id="shell" value={shell} onValueChange={(v) => setShell(v as Shell)} options={SHELLS.map((s) => ({ value: s, label: s }))} />
                </Field>
                <Field label="Timeout (seconds)" htmlFor="timeout">
                  <Input id="timeout" type="number" min={1} max={3600} value={timeout} onChange={(e) => setTimeoutSecs(Number(e.target.value))} />
                </Field>
              </div>
              <Field label="Script" htmlFor="body">
                <CodeEditor id="body" rows={10} value={body} onChange={(e) => setBody(e.target.value)} placeholder={shell === "powershell" ? "Get-Service | Where Status -eq 'Running'" : "uname -a"} />
              </Field>
            </>
          )}
        </div>
      )}
      {(type === "reboot" || type === "shutdown") && (
        <Field label="Delay (seconds)" htmlFor="delay" hint="Users are warned by the OS when supported. 0 = immediately.">
          <Input id="delay" type="number" min={0} max={86400} value={delay} onChange={(e) => setDelay(Number(e.target.value))} />
        </Field>
      )}
      {type === "display_message" && (
        <div className="space-y-3">
          <Field label="Title" htmlFor="mtitle">
            <Input id="mtitle" value={title} onChange={(e) => setTitle(e.target.value)} />
          </Field>
          <Field label="Message" htmlFor="mbody">
            <CodeEditor id="mbody" rows={4} className="font-sans" value={message} onChange={(e) => setMessage(e.target.value)} />
          </Field>
        </div>
      )}
      {type === "uninstall_agent" && (
        <p className="text-sm text-muted-foreground">
          The agent removes its service and credentials. The device stops reporting and must be re-enrolled to be managed again.
        </p>
      )}
      {type === "update_agent" && (
        <p className="text-sm text-muted-foreground">
          Each device downloads the newest agent from BEACON, checks that it&apos;s intact and runs, then restarts on it. It
          takes about a minute and the device stays managed throughout. Devices already up to date are skipped, and
          offline devices update when they next connect.
        </p>
      )}
      {(type === "lock" || type === "refresh_inventory") && (
        <p className="text-sm text-muted-foreground">
          {type === "lock" ? "Locks the active user session." : "Asks the agent to collect and upload a full inventory now."}
        </p>
      )}
      {guarded && (
        <div className="mt-4 space-y-2 rounded-lg border border-warning/40 bg-warning/10 p-3 text-sm">
          <p className="flex items-start gap-2 font-medium">
            <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden />
            This affects {count} devices at once.
          </p>
          <Field label={`Type ${count} to confirm`} htmlFor="confirm-count">
            <Input
              id="confirm-count"
              inputMode="numeric"
              autoComplete="off"
              value={typedCount}
              onChange={(e) => setTypedCount(e.target.value)}
            />
          </Field>
        </div>
      )}
    </Dialog>
  );
}

/** Expandable command row content: payload, timing and output. */
export function CommandOutput({ cmd }: { cmd: Command }) {
  const script = typeof cmd.payload.script === "string" ? cmd.payload.script : null;
  return (
    <div className="space-y-3 text-sm">
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
        <span>
          Status: <CommandStatusBadge status={cmd.status} />
        </span>
        {cmd.exit_code != null && <span>Exit code: <span className="font-mono text-foreground">{cmd.exit_code}</span></span>}
        <span>Issued: {formatDateTime(cmd.created_at)}</span>
        {cmd.completed_at && <span>Completed: {formatDateTime(cmd.completed_at)}</span>}
        <span>By: {cmd.issued_by_email ?? titleCase(cmd.source)}</span>
      </div>
      {script && <OutputBlock label={`Script (${String(cmd.payload.shell)})`} text={script} />}
      {cmd.error && <OutputBlock label="Error" text={cmd.error} tone="error" />}
      {cmd.stdout && <OutputBlock label="stdout" text={cmd.stdout} />}
      {cmd.stderr && <OutputBlock label="stderr" text={cmd.stderr} tone="error" />}
      {!cmd.stdout && !cmd.stderr && !cmd.error && (cmd.status === "queued" || cmd.status === "sent" || cmd.status === "running") && (
        <p className="text-xs text-muted-foreground">
          {cmd.status === "queued" ? "Waiting for the device to come online…" : "Running on the device…"}
        </p>
      )}
    </div>
  );
}

function OutputBlock({ label, text, tone }: { label: string; text: string; tone?: "error" }) {
  return (
    <div>
      <p className="mb-1 text-xs font-medium text-muted-foreground">{label}</p>
      <pre
        className={
          "max-h-80 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-muted/50 p-3 font-mono text-xs " +
          (tone === "error" ? "text-destructive" : "")
        }
      >
        {text}
      </pre>
    </div>
  );
}

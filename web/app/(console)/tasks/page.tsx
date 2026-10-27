"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, Pencil, Play, Plus, Trash2 } from "lucide-react";
import { api, qk, type Task, type TaskActionType, type TaskInput } from "@/lib/api";
import { useCan } from "@/lib/session";
import { formatDateTime, relativeTime, titleCase } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Segmented } from "@/components/ui/tabs";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

const CRON_PRESETS = [
  { value: "0 * * * *", label: "Every hour" },
  { value: "0 2 * * *", label: "Daily at 02:00 UTC" },
  { value: "0 9 * * 1-5", label: "Weekdays at 09:00 UTC" },
  { value: "0 3 * * 0", label: "Sundays at 03:00 UTC" },
  { value: "0 0 1 * *", label: "Monthly on the 1st" },
];

const ACTIONS: { value: TaskActionType; label: string }[] = [
  { value: "run_script", label: "Run a library script" },
  { value: "compliance_check", label: "Re-evaluate compliance" },
  { value: "refresh_inventory", label: "Refresh inventory" },
  { value: "reboot", label: "Reboot" },
  { value: "lock", label: "Lock screen" },
];

function describeCron(cron: string) {
  return CRON_PRESETS.find((p) => p.value === cron)?.label ?? cron;
}

export default function TasksPage() {
  const can = useCan();
  const qc = useQueryClient();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.tasks, queryFn: api.tasks.list });
  const scripts = useQuery({ queryKey: qk.scripts, queryFn: api.scripts.list });
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const [editing, setEditing] = useState<Task | "new" | null>(null);
  const [deleting, setDeleting] = useState<Task | null>(null);

  const invalidate = () => void qc.invalidateQueries({ queryKey: qk.tasks });
  const run = useMutation({
    mutationFn: (id: string) => api.tasks.run(id) as Promise<{ devices: number }>,
    onSuccess: (r) => {
      toast.success("Task started", `Dispatched to ${r.devices} device${r.devices === 1 ? "" : "s"}.`);
      invalidate();
    },
    onError: (e) => toast.error("Couldn't run task", e),
  });
  const toggle = useMutation({
    mutationFn: (t: Task) => api.tasks.update(t.id, { enabled: !t.enabled }),
    onSuccess: invalidate,
    onError: (e) => toast.error("Couldn't update task", e),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.tasks.remove(id),
    onSuccess: () => {
      toast.success("Task deleted");
      setDeleting(null);
      invalidate();
    },
    onError: (e) => toast.error("Couldn't delete task", e),
  });

  const actionLabel = (t: Task) => {
    if (t.action.type === "run_script") {
      const s = scripts.data?.items.find((x) => x.id === t.action.payload.script_id);
      return s ? `Script: ${s.name}` : "Run script";
    }
    return ACTIONS.find((a) => a.value === t.action.type)?.label ?? titleCase(t.action.type);
  };
  const targetLabel = (t: Task) =>
    t.target.all
      ? "All devices"
      : t.target.group_id
        ? `Group: ${groups.data?.items.find((g) => g.id === t.target.group_id)?.name ?? "…"}`
        : `${t.target.device_ids.length} device${t.target.device_ids.length === 1 ? "" : "s"}`;

  return (
    <>
      <PageHeader
        title="Scheduled tasks"
        description="Automate scripts, reboots, inventory refreshes and compliance checks on a schedule."
        actions={
          can("tasks.write") && (
            <Button onClick={() => setEditing("new")}>
              <Plus /> New task
            </Button>
          )
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={3} cols={5} />
          ) : !data?.items.length ? (
            <EmptyState icon={CalendarClock} title="No scheduled tasks" description="For example: re-check compliance nightly, or run a cleanup script every Sunday." />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>Task</TH>
                  <TH>Schedule</TH>
                  <TH className="hidden md:table-cell">Target</TH>
                  <TH className="hidden lg:table-cell">Last / next run</TH>
                  <TH>Enabled</TH>
                  <TH className="w-28" />
                </TR>
              </THead>
              <TBody>
                {data.items.map((t) => (
                  <TR key={t.id}>
                    <TD>
                      <p className="font-medium">{t.name}</p>
                      <p className="text-xs text-muted-foreground">{actionLabel(t)}</p>
                    </TD>
                    <TD>
                      <p className="text-sm">{describeCron(t.cron)}</p>
                      <p className="font-mono text-xs text-muted-foreground">{t.cron}</p>
                    </TD>
                    <TD className="hidden md:table-cell">{targetLabel(t)}</TD>
                    <TD className="hidden text-xs text-muted-foreground lg:table-cell">
                      <p>Last: {t.last_run_at ? relativeTime(t.last_run_at) : "never"}</p>
                      <p>Next: {t.next_run_at ? formatDateTime(t.next_run_at) : "—"}</p>
                    </TD>
                    <TD>
                      <Switch checked={t.enabled} disabled={!can("tasks.write")} onCheckedChange={() => toggle.mutate(t)} aria-label={`Enable ${t.name}`} />
                    </TD>
                    <TD>
                      {can("tasks.write") && (
                        <div className="flex justify-end gap-1">
                          <Button variant="ghost" size="icon-sm" onClick={() => run.mutate(t.id)} aria-label={`Run ${t.name} now`} title="Run now">
                            <Play />
                          </Button>
                          <Button variant="ghost" size="icon-sm" onClick={() => setEditing(t)} aria-label={`Edit ${t.name}`}>
                            <Pencil />
                          </Button>
                          <Button variant="ghost" size="icon-sm" onClick={() => setDeleting(t)} aria-label={`Delete ${t.name}`}>
                            <Trash2 />
                          </Button>
                        </div>
                      )}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </Card>
      )}
      {editing && <TaskDialog task={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete "${deleting?.name}"?`}
        confirmLabel="Delete task"
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </>
  );
}

function TaskDialog({ task, onClose }: { task: Task | null; onClose: () => void }) {
  const qc = useQueryClient();
  const scripts = useQuery({ queryKey: qk.scripts, queryFn: api.scripts.list });
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const devices = useQuery({ queryKey: qk.devices({}), queryFn: () => api.devices.list({}) });
  const [name, setName] = useState(task?.name ?? "");
  const [cron, setCron] = useState(task?.cron ?? "0 2 * * *");
  const [enabled, setEnabled] = useState(task?.enabled ?? true);
  const [actionType, setActionType] = useState<TaskActionType>(task?.action.type ?? "run_script");
  const [scriptId, setScriptId] = useState(typeof task?.action.payload.script_id === "string" ? task.action.payload.script_id : "");
  const initialTarget = task ? (task.target.all ? "all" : task.target.group_id ? "group" : "devices") : "all";
  const [targetKind, setTargetKind] = useState<"all" | "group" | "devices">(initialTarget);
  const [groupId, setGroupId] = useState(task?.target.group_id ?? "");
  const [deviceIds, setDeviceIds] = useState<string[]>(task?.target.device_ids ?? []);

  const save = useMutation({
    mutationFn: () => {
      const body: TaskInput = {
        name,
        cron,
        enabled,
        action: { type: actionType, payload: actionType === "run_script" ? { script_id: scriptId } : {} },
        target: {
          all: targetKind === "all",
          group_id: targetKind === "group" ? groupId : null,
          device_ids: targetKind === "devices" ? deviceIds : [],
        },
      };
      return task ? api.tasks.update(task.id, body) : api.tasks.create(body);
    },
    onSuccess: () => {
      toast.success(task ? "Task saved" : "Task created");
      void qc.invalidateQueries({ queryKey: qk.tasks });
      onClose();
    },
    onError: (e) => toast.error("Couldn't save task", e),
  });

  const valid =
    name.trim() &&
    cron.trim() &&
    (actionType !== "run_script" || scriptId) &&
    (targetKind !== "group" || groupId) &&
    (targetKind !== "devices" || deviceIds.length > 0);

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={task ? "Edit task" : "New scheduled task"}
      size="lg"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={save.isPending} disabled={!valid} onClick={() => save.mutate()}>
            Save task
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Name" htmlFor="tname">
          <Input id="tname" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Nightly compliance check" />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Schedule" htmlFor="tpreset">
            <Select
              id="tpreset"
              value={CRON_PRESETS.some((p) => p.value === cron) ? cron : ""}
              onValueChange={(v) => v && setCron(v)}
              placeholder="Custom"
              options={CRON_PRESETS}
            />
          </Field>
          <Field label="Cron expression (UTC)" htmlFor="tcron" hint="minute hour day month weekday">
            <Input id="tcron" className="font-mono" value={cron} onChange={(e) => setCron(e.target.value)} />
          </Field>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Action" htmlFor="taction">
            <Select id="taction" value={actionType} onValueChange={(v) => setActionType(v as TaskActionType)} options={ACTIONS} />
          </Field>
          {actionType === "run_script" && (
            <Field label="Script" htmlFor="tscript">
              <Select
                id="tscript"
                value={scriptId}
                onValueChange={setScriptId}
                placeholder="Choose a script"
                options={(scripts.data?.items ?? []).map((s) => ({ value: s.id, label: `${s.name} (${s.shell})` }))}
              />
            </Field>
          )}
        </div>
        <div>
          <p className="mb-2 text-sm font-medium">Target</p>
          <Segmented
            value={targetKind}
            onChange={setTargetKind}
            options={[
              { value: "all", label: "All devices" },
              { value: "group", label: "A group" },
              { value: "devices", label: "Specific devices" },
            ]}
          />
          {targetKind === "group" && (
            <Select
              className="mt-3"
              aria-label="Target group"
              value={groupId}
              onValueChange={setGroupId}
              placeholder="Choose a group"
              options={(groups.data?.items ?? []).map((g) => ({ value: g.id, label: g.name }))}
            />
          )}
          {targetKind === "devices" && (
            <div className="mt-3 max-h-48 space-y-1 overflow-y-auto rounded-md border border-border p-2">
              {(devices.data?.items ?? []).map((d) => (
                <label key={d.id} className="flex items-center gap-2 rounded px-1 py-0.5 text-sm hover:bg-muted">
                  <Checkbox
                    checked={deviceIds.includes(d.id)}
                    onChange={(e) => setDeviceIds((xs) => (e.target.checked ? [...xs, d.id] : xs.filter((x) => x !== d.id)))}
                  />
                  {d.name}
                </label>
              ))}
              {devices.data?.items.length === 0 && <p className="text-sm text-muted-foreground">No devices enrolled.</p>}
            </div>
          )}
        </div>
        <label className="flex items-center gap-3 text-sm">
          <Switch checked={enabled} onCheckedChange={setEnabled} aria-label="Task enabled" /> Enabled
        </label>
      </div>
    </Dialog>
  );
}

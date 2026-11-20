"use client";

import Link from "next/link";
import { Fragment, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Pencil, Plus, RefreshCw, ShieldCheck, Trash2, X } from "lucide-react";
import {
  api,
  qk,
  SEVERITIES,
  type ComplianceField,
  type Condition,
  type ConditionOp,
  type Rule,
  type RuleInput,
  type Severity,
} from "@/lib/api";
import { useCan } from "@/lib/session";
import { cn, formatDateTime, osLabel, relativeTime, titleCase } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Label } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Segmented } from "@/components/ui/tabs";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader, SeverityBadge } from "@/components/common";
import { toast } from "@/components/toast";

const OP_LABELS: Record<string, string> = {
  eq: "equals",
  neq: "does not equal",
  gt: "is greater than",
  gte: "is at least",
  lt: "is less than",
  lte: "is at most",
  version_gte: "is version ≥",
  version_lt: "is version <",
  contains: "contains",
  not_contains: "does not contain",
  is_true: "is true",
  is_false: "is false",
  none_found: "none found",
  any_found: "any found",
};

const NO_VALUE_OPS: string[] = ["is_true", "is_false", "none_found", "any_found"];

const ALL_OS = ["windows", "macos", "linux"];

/** True when a rule covers every operating system: none picked, or all of them. */
const coversAllOS = (os: string[]) => os.length === 0 || ALL_OS.every((o) => os.includes(o));

/** Ready-made rules for the most common baselines. */
const TEMPLATES: { label: string; rule: Partial<RuleInput> }[] = [
  {
    label: "Disk encryption required",
    rule: { name: "Disk encryption required", severity: "high", conditions: [{ field: "security.disk_encrypted", op: "is_true", value: null }] },
  },
  {
    label: "Firewall enabled",
    rule: { name: "Firewall enabled", severity: "medium", conditions: [{ field: "security.firewall_enabled", op: "is_true", value: null }] },
  },
  {
    label: "Checked in within 24h",
    rule: { name: "Checked in within 24 hours", severity: "low", conditions: [{ field: "agent.last_seen_minutes", op: "lte", value: 1440 }] },
  },
  {
    label: "At least 10% free disk",
    rule: { name: "Low disk space", severity: "medium", conditions: [{ field: "hardware.disk_free_pct", op: "gte", value: 10 }] },
  },
  {
    label: "No gambling apps or games",
    rule: {
      name: "No gambling apps or games",
      severity: "medium",
      conditions: [
        { field: "software.gambling", op: "none_found", value: null },
        { field: "software.games", op: "none_found", value: null },
      ],
    },
  },
];

export default function CompliancePage() {
  const can = useCan();
  const qc = useQueryClient();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.complianceRules, queryFn: api.compliance.rules });
  const fields = useQuery({ queryKey: qk.complianceFields, queryFn: api.compliance.fields, staleTime: Infinity });
  const [editing, setEditing] = useState<{ rule: Rule | null; template?: Partial<RuleInput> } | null>(null);
  const [deleting, setDeleting] = useState<Rule | null>(null);
  const [open, setOpen] = useState<Set<string>>(new Set());
  const toggleOpen = (id: string) =>
    setOpen((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });

  const evaluate = useMutation({
    mutationFn: api.compliance.evaluate,
    onSuccess: (r) => {
      toast.success(`Re-evaluated ${r.evaluated} device${r.evaluated === 1 ? "" : "s"}`);
      void qc.invalidateQueries({ queryKey: qk.complianceRules });
      void qc.invalidateQueries({ queryKey: qk.devices() });
      void qc.invalidateQueries({ queryKey: qk.dashboard });
    },
    onError: (e) => toast.error("Evaluation failed", e),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.compliance.remove(id),
    onSuccess: () => {
      toast.success("Rule deleted");
      setDeleting(null);
      void qc.invalidateQueries({ queryKey: qk.complianceRules });
    },
    onError: (e) => toast.error("Couldn't delete rule", e),
  });
  const toggle = useMutation({
    mutationFn: (r: Rule) => api.compliance.update(r.id, { enabled: !r.enabled }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: qk.complianceRules }),
    onError: (e) => toast.error("Couldn't update rule", e),
  });

  const fieldLabel = (name: string) => fields.data?.items.find((f) => f.field === name)?.label ?? name;
  const describe = (c: Condition) =>
    `${fieldLabel(c.field)} ${OP_LABELS[c.op] ?? c.op}${NO_VALUE_OPS.includes(c.op) ? "" : ` ${String(c.value)}`}`;

  return (
    <>
      <PageHeader
        title="Compliance"
        description="Baseline rules evaluated on every inventory report. Violations can alert, notify, tag or remediate automatically."
        actions={
          can("compliance.write") && (
            <>
              <Button variant="outline" loading={evaluate.isPending} onClick={() => evaluate.mutate()}>
                <RefreshCw /> Re-evaluate all
              </Button>
              <Button onClick={() => setEditing({ rule: null })}>
                <Plus /> New rule
              </Button>
            </>
          )
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={4} cols={5} />
          ) : !data?.items.length ? (
            <EmptyState
              icon={ShieldCheck}
              title="No compliance rules yet"
              description="Start from a common baseline or build your own rule."
              action={
                can("compliance.write") ? (
                  <div className="flex flex-wrap justify-center gap-2">
                    {TEMPLATES.map((t) => (
                      <Button key={t.label} variant="outline" size="sm" onClick={() => setEditing({ rule: null, template: t.rule })}>
                        {t.label}
                      </Button>
                    ))}
                  </div>
                ) : undefined
              }
            />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>Rule</TH>
                  <TH>Severity</TH>
                  <TH className="hidden lg:table-cell">Scope</TH>
                  <TH>Violations</TH>
                  <TH>Enabled</TH>
                  <TH className="w-24">
                    <span className="sr-only">Actions</span>
                  </TH>
                </TR>
              </THead>
              <TBody>
                {data.items.map((r) => (
                  <Fragment key={r.id}>
                    <TR>
                      <TD className="max-w-md">
                        <p className="font-medium">{r.name}</p>
                        <p className="text-xs text-muted-foreground">
                          {r.conditions.map(describe).join(r.match === "all" ? " AND " : " OR ")}
                        </p>
                      </TD>
                      <TD>
                        <SeverityBadge severity={r.severity} />
                      </TD>
                      <TD className="hidden text-sm text-muted-foreground lg:table-cell">
                        {coversAllOS(r.os_families) ? "All OS" : r.os_families.map(osLabel).join(", ")}
                        {r.group_id ? " · one group" : ""}
                      </TD>
                      <TD>
                        {r.violating_devices > 0 ? (
                          <button
                            type="button"
                            onClick={() => toggleOpen(r.id)}
                            aria-expanded={open.has(r.id)}
                            aria-controls={`viol-${r.id}`}
                            title={open.has(r.id) ? "Hide the failing devices" : "Show which devices fail, and why"}
                            className="rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                          >
                            <Badge variant="danger" className="cursor-pointer gap-1 hover:brightness-110">
                              {r.violating_devices} device{r.violating_devices === 1 ? "" : "s"}
                              <ChevronDown className={cn("h-3 w-3 transition-transform", open.has(r.id) && "rotate-180")} aria-hidden />
                            </Badge>
                          </button>
                        ) : (
                          <Badge variant="success">None</Badge>
                        )}
                      </TD>
                      <TD>
                        <Switch
                          checked={r.enabled}
                          disabled={!can("compliance.write") || toggle.isPending}
                          onCheckedChange={() => toggle.mutate(r)}
                          aria-label={`Enable ${r.name}`}
                        />
                      </TD>
                      <TD>
                        {can("compliance.write") && (
                          <div className="flex justify-end gap-2">
                            <Button variant="ghost" size="icon-sm" onClick={() => setEditing({ rule: r })} aria-label={`Edit ${r.name}`} title="Edit rule">
                              <Pencil />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => setDeleting(r)}
                              aria-label={`Delete ${r.name}`}
                              title="Delete rule"
                              className="hover:text-destructive"
                            >
                              <Trash2 />
                            </Button>
                          </div>
                        )}
                      </TD>
                    </TR>
                    {open.has(r.id) && r.violating_devices > 0 && (
                      <TR className="hover:bg-transparent">
                        <TD colSpan={6} id={`viol-${r.id}`} className="bg-muted/30 px-4 py-3">
                          <Violations rule={r} />
                        </TD>
                      </TR>
                    )}
                  </Fragment>
                ))}
              </TBody>
            </Table>
          )}
        </Card>
      )}
      {editing && fields.data && (
        <RuleDialog rule={editing.rule} template={editing.template} fields={fields.data.items} onClose={() => setEditing(null)} />
      )}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete "${deleting?.name}"?`}
        description="Its results are removed and devices are re-evaluated without it."
        confirmLabel="Delete rule"
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </>
  );
}

type Remediation = "none" | "run_script" | "refresh_inventory" | "lock";

function RuleDialog({
  rule,
  template,
  fields,
  onClose,
}: {
  rule: Rule | null;
  template?: Partial<RuleInput>;
  fields: ComplianceField[];
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const scripts = useQuery({ queryKey: qk.scripts, queryFn: api.scripts.list });
  const base = rule ?? template;
  const [name, setName] = useState(base?.name ?? "");
  const [description, setDescription] = useState(base?.description ?? "");
  const [severity, setSeverity] = useState<Severity>(base?.severity ?? "medium");
  const [enabled, setEnabled] = useState(base?.enabled ?? true);
  const [match, setMatch] = useState<"all" | "any">(base?.match ?? "all");
  const [conditions, setConditions] = useState<Condition[]>(
    base?.conditions?.length ? base.conditions : [{ field: fields[0]!.field, op: fields[0]!.ops[0] as ConditionOp, value: null }],
  );
  // Nothing stored means every OS, so it opens with all three ticked.
  const [osFamilies, setOsFamilies] = useState<string[]>(base?.os_families?.length ? base.os_families : ALL_OS);
  const [groupId, setGroupId] = useState(base?.group_id ?? "");
  const [alert, setAlert] = useState(base?.actions?.alert ?? true);
  const [webhook, setWebhook] = useState(base?.actions?.webhook ?? false);
  const [tag, setTag] = useState(base?.actions?.tag ?? "");
  const initialCmd = base?.actions?.command;
  const [remediation, setRemediation] = useState<Remediation>((initialCmd?.type as Remediation) ?? "none");
  const [scriptId, setScriptId] = useState(typeof initialCmd?.payload?.script_id === "string" ? initialCmd.payload.script_id : "");

  const fieldOf = (name: string) => fields.find((f) => f.field === name) ?? fields[0]!;
  const update = (i: number, patch: Partial<Condition>) =>
    setConditions((cs) => cs.map((c, j) => (j === i ? { ...c, ...patch } : c)));

  const save = useMutation({
    mutationFn: () => {
      const body: RuleInput = {
        name,
        description,
        severity,
        enabled,
        match,
        conditions: conditions.map((c) => {
          const f = fieldOf(c.field);
          if (NO_VALUE_OPS.includes(c.op)) return { ...c, value: null };
          return { ...c, value: f.type === "number" ? Number(c.value) : String(c.value ?? "") };
        }),
        // All three ticked is saved as "every OS", so new platforms are covered too.
        os_families: coversAllOS(osFamilies) ? [] : ALL_OS.filter((o) => osFamilies.includes(o)),
        group_id: groupId || null,
        actions: {
          alert,
          webhook,
          tag: tag.trim() || null,
          command:
            remediation === "none"
              ? null
              : remediation === "run_script"
                ? { type: "run_script", payload: { script_id: scriptId } }
                : { type: remediation, payload: {} },
        },
      };
      return rule ? api.compliance.update(rule.id, body) : api.compliance.create(body);
    },
    onSuccess: () => {
      toast.success(rule ? "Rule saved" : "Rule created", "Devices are being re-evaluated.");
      void qc.invalidateQueries({ queryKey: qk.complianceRules });
      setTimeout(() => {
        void qc.invalidateQueries({ queryKey: qk.complianceRules });
        void qc.invalidateQueries({ queryKey: qk.devices() });
        void qc.invalidateQueries({ queryKey: qk.dashboard });
      }, 1500);
      onClose();
    },
    onError: (e) => toast.error("Couldn't save rule", e),
  });

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={rule ? "Edit compliance rule" : "New compliance rule"}
      size="xl"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={save.isPending} disabled={!name.trim() || osFamilies.length === 0 || (remediation === "run_script" && !scriptId)} onClick={() => save.mutate()}>
            Save rule
          </Button>
        </>
      }
    >
      <div className="space-y-6">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_10rem]">
          <Field label="Name" htmlFor="rname">
            <Input id="rname" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Disk encryption required" />
          </Field>
          <Field label="Severity" htmlFor="rsev">
            <Select id="rsev" value={severity} onValueChange={(v) => setSeverity(v as Severity)} options={SEVERITIES.map((s) => ({ value: s, label: titleCase(s) }))} />
          </Field>
        </div>
        <Field label="Description" htmlFor="rdesc">
          <Input id="rdesc" value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>

        <section>
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-sm font-semibold">Device is compliant when</h3>
            <Segmented
              value={match}
              onChange={setMatch}
              options={[
                { value: "all", label: "All conditions pass" },
                { value: "any", label: "Any condition passes" },
              ]}
            />
          </div>
          <div className="space-y-2">
            {conditions.map((c, i) => {
              const f = fieldOf(c.field);
              const needsValue = !NO_VALUE_OPS.includes(c.op);
              return (
                <div key={i} className="grid grid-cols-1 items-center gap-2 rounded-md border border-border bg-muted/30 p-2 sm:grid-cols-[1.3fr_1fr_1fr_auto]">
                  <Select
                    aria-label="Field"
                    value={c.field}
                    onValueChange={(v) => {
                      const nf = fieldOf(v);
                      update(i, { field: v, op: nf.ops[0] as ConditionOp, value: null });
                    }}
                    options={fields.map((x) => ({ value: x.field, label: x.label }))}
                  />
                  <Select
                    aria-label="Operator"
                    value={c.op}
                    onValueChange={(v) => update(i, { op: v as ConditionOp })}
                    options={f.ops.map((o) => ({ value: o, label: OP_LABELS[o] ?? o }))}
                  />
                  {needsValue ? (
                    <div className="relative">
                      <Input
                        aria-label="Value"
                        type={f.type === "number" ? "number" : "text"}
                        value={c.value == null ? "" : String(c.value)}
                        onChange={(e) => update(i, { value: e.target.value })}
                        placeholder={f.type === "version" ? "e.g. 10.0.19045" : f.type === "list" ? "name contains…" : "value"}
                        className={f.unit ? "pr-10" : undefined}
                      />
                      {f.unit && <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{f.unit}</span>}
                    </div>
                  ) : (
                    <span className="px-1 text-xs text-muted-foreground">No value needed</span>
                  )}
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    // A rule needs at least one condition; the space stays so rows line up.
                    className={cn(conditions.length === 1 && "invisible")}
                    disabled={conditions.length === 1}
                    onClick={() => setConditions((cs) => cs.filter((_, j) => j !== i))}
                    aria-label="Remove condition"
                    title="Remove condition"
                  >
                    <X />
                  </Button>
                </div>
              );
            })}
          </div>
          <Button
            variant="link"
            size="sm"
            className="mt-2"
            onClick={() => setConditions((cs) => [...cs, { field: fields[0]!.field, op: fields[0]!.ops[0] as ConditionOp, value: null }])}
          >
            <Plus /> Add condition
          </Button>
          <p className="mt-1 text-xs text-muted-foreground">If the agent can&apos;t determine a value (for example, encryption status in a VM), the condition fails.</p>
        </section>

        <section className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <Label>Applies to operating systems</Label>
            <div className="mt-2 flex flex-wrap gap-4">
              {ALL_OS.map((o) => (
                <label key={o} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={osFamilies.includes(o)}
                    onChange={(e) => setOsFamilies((xs) => (e.target.checked ? [...xs, o] : xs.filter((x) => x !== o)))}
                  />
                  {osLabel(o)}
                </label>
              ))}
            </div>
            {osFamilies.length === 0 && <p className="mt-1 text-xs text-destructive">Pick at least one.</p>}
          </div>
          <Field label="Applies to group" htmlFor="rgroup">
            <Select
              id="rgroup"
              value={groupId}
              onValueChange={setGroupId}
              placeholder="All groups"
              options={(groups.data?.items ?? []).map((g) => ({ value: g.id, label: g.name }))}
            />
          </Field>
        </section>

        <section>
          <h3 className="mb-2 text-sm font-semibold">When a device starts violating this rule</h3>
          <div className="space-y-3 rounded-md border border-border p-3">
            <label className="flex items-center justify-between gap-4 text-sm">
              <span>
                Open an alert <span className="text-muted-foreground">(auto-resolves when the device passes again)</span>
              </span>
              <Switch checked={alert} onCheckedChange={setAlert} aria-label="Open an alert" />
            </label>
            <label className="flex items-center justify-between gap-4 text-sm">
              <span>
                Send a <code className="text-xs">compliance.violation</code> webhook
              </span>
              <Switch checked={webhook} onCheckedChange={setWebhook} aria-label="Send webhook" />
            </label>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Field label="Tag the device" htmlFor="rtag">
                <Input id="rtag" value={tag} onChange={(e) => setTag(e.target.value)} placeholder="e.g. needs-encryption" />
              </Field>
              <Field label="Remediation" htmlFor="rrem">
                <Select
                  id="rrem"
                  value={remediation}
                  onValueChange={(v) => setRemediation(v as Remediation)}
                  options={[
                    { value: "none", label: "None" },
                    { value: "run_script", label: "Run a library script" },
                    { value: "refresh_inventory", label: "Refresh inventory" },
                    { value: "lock", label: "Lock the screen" },
                  ]}
                />
              </Field>
            </div>
            {remediation === "run_script" && (
              <Field label="Script" htmlFor="rscript">
                <Select
                  id="rscript"
                  value={scriptId}
                  onValueChange={setScriptId}
                  placeholder="Choose a script"
                  options={(scripts.data?.items ?? []).map((s) => ({ value: s.id, label: `${s.name} (${s.shell})` }))}
                />
              </Field>
            )}
          </div>
        </section>

        <label className="flex items-center gap-3 text-sm">
          <Switch checked={enabled} onCheckedChange={setEnabled} aria-label="Rule enabled" /> Rule enabled
        </label>
      </div>
    </Dialog>
  );
}

/** The devices failing a rule, with the reason each one fails. */
function Violations({ rule }: { rule: Rule }) {
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: qk.ruleViolations(rule.id),
    queryFn: () => api.compliance.violations(rule.id),
  });
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;
  if (isLoading) return <TableSkeleton rows={Math.min(rule.violating_devices, 4)} cols={4} />;
  const list = data?.items ?? [];
  if (list.length === 0) return <p className="text-sm text-muted-foreground">No device fails this rule any more.</p>;
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-card">
      <Table>
        <THead>
          <TR>
            <TH>Device</TH>
            <TH className="hidden md:table-cell">Signed in</TH>
            <TH>Why it fails</TH>
            <TH className="hidden sm:table-cell">Checked</TH>
          </TR>
        </THead>
        <TBody>
          {list.map((v) => (
            <TR key={v.device_id}>
              <TD className="whitespace-nowrap">
                <Link href={`/devices/${v.device_id}`} className="font-medium text-primary underline-offset-2 hover:underline">
                  {v.device_name}
                </Link>
                <p className="text-xs text-muted-foreground">{osLabel(v.os_family)}</p>
              </TD>
              <TD className="hidden text-sm text-muted-foreground md:table-cell">{v.user || "—"}</TD>
              <TD className="text-sm">{v.details || <span className="text-muted-foreground">No detail recorded</span>}</TD>
              <TD className="hidden whitespace-nowrap text-sm text-muted-foreground sm:table-cell" title={formatDateTime(v.evaluated_at)}>
                {relativeTime(v.evaluated_at)}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </div>
  );
}

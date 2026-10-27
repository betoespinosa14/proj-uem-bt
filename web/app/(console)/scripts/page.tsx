"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileCode2, Pencil, Plus, Trash2 } from "lucide-react";
import { api, qk, SHELLS, type Script, type Shell } from "@/lib/api";
import { useCan } from "@/lib/session";
import { relativeTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { TableSkeleton } from "@/components/ui/skeleton";
import { CodeEditor } from "@/components/ui/textarea";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

export default function ScriptsPage() {
  const can = useCan();
  const qc = useQueryClient();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.scripts, queryFn: api.scripts.list });
  const [editing, setEditing] = useState<Script | "new" | null>(null);
  const [deleting, setDeleting] = useState<Script | null>(null);
  const remove = useMutation({
    mutationFn: (id: string) => api.scripts.remove(id),
    onSuccess: () => {
      toast.success("Script deleted");
      setDeleting(null);
      void qc.invalidateQueries({ queryKey: qk.scripts });
    },
    onError: (e) => toast.error("Couldn't delete script", e),
  });

  return (
    <>
      <PageHeader
        title="Script library"
        description="Reusable PowerShell, bash and shell scripts to run on devices, on demand or on a schedule."
        actions={
          can("scripts.write") && (
            <Button onClick={() => setEditing("new")}>
              <Plus /> New script
            </Button>
          )
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={4} cols={4} />
          ) : !data?.items.length ? (
            <EmptyState icon={FileCode2} title="No scripts yet" description="Save scripts here to run them on one device, many devices, or on a schedule." />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>Name</TH>
                  <TH>Shell</TH>
                  <TH className="hidden md:table-cell">Timeout</TH>
                  <TH className="hidden md:table-cell">Updated</TH>
                  <TH className="w-24" />
                </TR>
              </THead>
              <TBody>
                {data.items.map((s) => (
                  <TR key={s.id}>
                    <TD>
                      <p className="font-medium">{s.name}</p>
                      {s.description && <p className="text-xs text-muted-foreground">{s.description}</p>}
                    </TD>
                    <TD>
                      <Badge variant="outline" className="font-mono">
                        {s.shell}
                      </Badge>
                    </TD>
                    <TD className="hidden tabular text-muted-foreground md:table-cell">{s.timeout_seconds}s</TD>
                    <TD className="hidden text-muted-foreground md:table-cell">{relativeTime(s.updated_at)}</TD>
                    <TD>
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="icon-sm" onClick={() => setEditing(s)} aria-label={`Open ${s.name}`}>
                          <Pencil />
                        </Button>
                        {can("scripts.write") && (
                          <Button variant="ghost" size="icon-sm" onClick={() => setDeleting(s)} aria-label={`Delete ${s.name}`}>
                            <Trash2 />
                          </Button>
                        )}
                      </div>
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </Card>
      )}
      {editing && <ScriptDialog script={editing === "new" ? null : editing} readOnly={!can("scripts.write")} onClose={() => setEditing(null)} />}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete ${deleting?.name}?`}
        description="Scheduled tasks that use this script will fail until they're updated. Past command history is kept."
        confirmLabel="Delete script"
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </>
  );
}

function ScriptDialog({ script, readOnly, onClose }: { script: Script | null; readOnly: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(script?.name ?? "");
  const [description, setDescription] = useState(script?.description ?? "");
  const [shell, setShell] = useState<Shell>(script?.shell ?? "bash");
  const [body, setBody] = useState(script?.body ?? "");
  const [timeout, setTimeoutSecs] = useState(script?.timeout_seconds ?? 300);
  const save = useMutation({
    mutationFn: () => {
      const b = { name, description, shell, body, timeout_seconds: timeout };
      return script ? api.scripts.update(script.id, b) : api.scripts.create(b);
    },
    onSuccess: () => {
      toast.success(script ? "Script saved" : "Script created");
      void qc.invalidateQueries({ queryKey: qk.scripts });
      onClose();
    },
    onError: (e) => toast.error("Couldn't save script", e),
  });
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={readOnly ? script?.name : script ? "Edit script" : "New script"}
      size="xl"
      footer={
        readOnly ? (
          <Button variant="outline" onClick={onClose}>
            Close
          </Button>
        ) : (
          <>
            <Button variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button loading={save.isPending} disabled={!name.trim() || !body.trim()} onClick={() => save.mutate()}>
              Save
            </Button>
          </>
        )
      }
    >
      <fieldset disabled={readOnly} className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-[1fr_10rem_9rem]">
          <Field label="Name" htmlFor="sname">
            <Input id="sname" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Clear browser caches" />
          </Field>
          <Field label="Shell" htmlFor="sshell">
            <Select id="sshell" value={shell} onValueChange={(v) => setShell(v as Shell)} options={SHELLS.map((s) => ({ value: s, label: s }))} />
          </Field>
          <Field label="Timeout (s)" htmlFor="stimeout">
            <Input id="stimeout" type="number" min={1} max={3600} value={timeout} onChange={(e) => setTimeoutSecs(Number(e.target.value))} />
          </Field>
        </div>
        <Field label="Description" htmlFor="sdesc">
          <Input id="sdesc" value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <Field label="Script" htmlFor="sbody" hint="Runs as root/SYSTEM. A non-zero exit code marks the command as failed.">
          <CodeEditor id="sbody" rows={16} value={body} onChange={(e) => setBody(e.target.value)} />
        </Field>
      </fieldset>
    </Dialog>
  );
}

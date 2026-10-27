"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plug, Plus, Send, Trash2 } from "lucide-react";
import { api, qk, WEBHOOK_EVENTS, type Webhook } from "@/lib/api";
import { relativeTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Label } from "@/components/ui/input";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

const EVENT_HELP: Record<string, string> = {
  "alert.created": "A compliance alert opened",
  "compliance.violation": "A device started failing a rule (rules with the webhook action)",
  "device.enrolled": "A new device enrolled",
  "device.offline": "A device stopped checking in",
  "command.completed": "A command or script finished",
  "audit.*": "Every audit log entry (SIEM streaming)",
};

export default function IntegrationsPage() {
  const qc = useQueryClient();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.webhooks, queryFn: api.webhooks.list });
  const [editing, setEditing] = useState<Webhook | "new" | null>(null);
  const [deleting, setDeleting] = useState<Webhook | null>(null);
  const test = useMutation({
    mutationFn: (id: string) => api.webhooks.test(id) as Promise<{ ok: boolean; status: number; error?: string }>,
    onSuccess: (r) => {
      if (r.ok) toast.success("Test delivered", `Receiver responded with HTTP ${r.status}.`);
      else toast.error("Test delivery failed", r.error ?? `Receiver responded with HTTP ${r.status}.`);
      void qc.invalidateQueries({ queryKey: qk.webhooks });
    },
    onError: (e) => toast.error("Test failed", e),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.webhooks.remove(id),
    onSuccess: () => {
      toast.success("Webhook deleted");
      setDeleting(null);
      void qc.invalidateQueries({ queryKey: qk.webhooks });
    },
    onError: (e) => toast.error("Couldn't delete webhook", e),
  });

  return (
    <>
      <PageHeader
        title="Integrations"
        description="Stream events to your SIEM (Splunk, Sentinel, Datadog) or ITSM (Jira, Freshservice) with signed webhooks."
        actions={
          <Button onClick={() => setEditing("new")}>
            <Plus /> New webhook
          </Button>
        }
      />
      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={3} cols={4} />
          ) : !data?.items.length ? (
            <EmptyState
              icon={Plug}
              title="No webhooks yet"
              description="Each delivery is a JSON POST signed with HMAC-SHA256 in the X-UEM-Signature header."
            />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>Webhook</TH>
                  <TH className="hidden md:table-cell">Events</TH>
                  <TH>Last delivery</TH>
                  <TH className="w-32" />
                </TR>
              </THead>
              <TBody>
                {data.items.map((w) => (
                  <TR key={w.id}>
                    <TD className="max-w-xs">
                      <p className="flex items-center gap-2 font-medium">
                        {w.name} {!w.enabled && <Badge variant="neutral">Disabled</Badge>}
                      </p>
                      <p className="truncate font-mono text-xs text-muted-foreground">{w.url}</p>
                    </TD>
                    <TD className="hidden md:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {w.events.map((e) => (
                          <Badge key={e} variant="outline" className="font-mono">
                            {e}
                          </Badge>
                        ))}
                      </div>
                    </TD>
                    <TD>
                      {w.last_delivery_at ? (
                        <div className="flex items-center gap-2">
                          <Badge variant={w.last_status && w.last_status < 300 ? "success" : "danger"}>{w.last_status || "error"}</Badge>
                          <span className="text-xs text-muted-foreground">{relativeTime(w.last_delivery_at)}</span>
                        </div>
                      ) : (
                        <span className="text-muted-foreground">Never</span>
                      )}
                    </TD>
                    <TD>
                      <div className="flex justify-end gap-1">
                        <Button variant="ghost" size="icon-sm" title="Send test event" aria-label={`Test ${w.name}`} onClick={() => test.mutate(w.id)}>
                          <Send />
                        </Button>
                        <Button variant="ghost" size="icon-sm" aria-label={`Edit ${w.name}`} onClick={() => setEditing(w)}>
                          <Pencil />
                        </Button>
                        <Button variant="ghost" size="icon-sm" aria-label={`Delete ${w.name}`} onClick={() => setDeleting(w)}>
                          <Trash2 />
                        </Button>
                      </div>
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </Card>
      )}
      {editing && <WebhookDialog hook={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete ${deleting?.name}?`}
        confirmLabel="Delete webhook"
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </>
  );
}

function WebhookDialog({ hook, onClose }: { hook: Webhook | null; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(hook?.name ?? "");
  const [url, setUrl] = useState(hook?.url ?? "");
  const [events, setEvents] = useState<string[]>(hook?.events ?? ["alert.created"]);
  const [enabled, setEnabled] = useState(hook?.enabled ?? true);
  const save = useMutation({
    mutationFn: () => {
      const body = { name, url, events, enabled };
      return hook ? api.webhooks.update(hook.id, body) : api.webhooks.create(body);
    },
    onSuccess: () => {
      toast.success(hook ? "Webhook saved" : "Webhook created");
      void qc.invalidateQueries({ queryKey: qk.webhooks });
      onClose();
    },
    onError: (e) => toast.error("Couldn't save webhook", e),
  });
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={hook ? "Edit webhook" : "New webhook"}
      size="lg"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={save.isPending} disabled={!name.trim() || !url.trim() || events.length === 0} onClick={() => save.mutate()}>
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Name" htmlFor="wname">
          <Input id="wname" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Splunk HEC" />
        </Field>
        <Field label="Endpoint URL" htmlFor="wurl" hint="Must be reachable from the UEM server over HTTPS (private addresses are blocked by default).">
          <Input id="wurl" type="url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://hooks.example.com/uem" />
        </Field>
        <div>
          <Label>Events</Label>
          <div className="mt-2 space-y-2">
            {WEBHOOK_EVENTS.map((e) => (
              <label key={e} className="flex items-start gap-2 text-sm">
                <Checkbox
                  className="mt-0.5"
                  checked={events.includes(e)}
                  onChange={(ev) => setEvents((xs) => (ev.target.checked ? [...xs, e] : xs.filter((x) => x !== e)))}
                />
                <span>
                  <span className="font-mono text-xs">{e}</span>
                  <span className="block text-xs text-muted-foreground">{EVENT_HELP[e]}</span>
                </span>
              </label>
            ))}
          </div>
        </div>
        {hook && (
          <Field label="Signing secret" htmlFor="wsecret" hint="Verify deliveries by computing HMAC-SHA256 of the raw body with this secret.">
            <Input id="wsecret" readOnly className="font-mono text-xs" value={hook.secret} onFocus={(e) => e.target.select()} />
          </Field>
        )}
        <label className="flex items-center gap-3 text-sm">
          <Switch checked={enabled} onCheckedChange={setEnabled} aria-label="Webhook enabled" /> Enabled
        </label>
      </div>
    </Dialog>
  );
}

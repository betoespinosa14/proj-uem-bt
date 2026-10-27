"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Apple, Check, Copy, KeyRound, Monitor, Plus, Server, Terminal } from "lucide-react";
import { api, qk, type EnrollmentToken } from "@/lib/api";
import { useCan } from "@/lib/session";
import { copyToClipboard, formatDateTime, relativeTime } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

function tokenState(t: EnrollmentToken): { label: string; variant: "success" | "neutral" | "danger" } {
  if (t.revoked) return { label: "Revoked", variant: "danger" };
  if (t.expires_at && new Date(t.expires_at).getTime() < Date.now()) return { label: "Expired", variant: "neutral" };
  if (t.max_uses != null && t.uses >= t.max_uses) return { label: "Used up", variant: "neutral" };
  return { label: "Active", variant: "success" };
}

export default function EnrollmentPage() {
  const can = useCan();
  const qc = useQueryClient();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.enrollment, queryFn: api.enrollment.list });
  const [creating, setCreating] = useState(false);
  const [showing, setShowing] = useState<EnrollmentToken | null>(null);
  const [revoking, setRevoking] = useState<EnrollmentToken | null>(null);

  const revoke = useMutation({
    mutationFn: (id: string) => api.enrollment.revoke(id),
    onSuccess: () => {
      toast.success("Token revoked");
      setRevoking(null);
      void qc.invalidateQueries({ queryKey: qk.enrollment });
    },
    onError: (e) => toast.error("Couldn't revoke token", e),
  });

  return (
    <>
      <PageHeader
        title="Enrollment"
        description="Enroll Windows, macOS and Linux devices with a one-line installer."
        actions={
          can("enrollment.write") && (
            <Button onClick={() => setCreating(true)}>
              <Plus /> New token
            </Button>
          )
        }
      />

      <div className="mb-6 grid gap-4 md:grid-cols-3">
        {[
          { icon: KeyRound, title: "1. Create a token", body: "Optionally assign a group, an expiry and a use limit." },
          { icon: Terminal, title: "2. Run the installer", body: "Paste the one-liner as admin/root on the device." },
          { icon: Monitor, title: "3. Device appears", body: "It shows up online within seconds with full inventory." },
        ].map((s) => (
          <Card key={s.title} className="flex gap-3 p-4">
            <s.icon className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
            <div>
              <p className="text-sm font-medium">{s.title}</p>
              <p className="text-sm text-muted-foreground">{s.body}</p>
            </div>
          </Card>
        ))}
      </div>

      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={3} cols={5} />
          ) : !data?.items.length ? (
            <EmptyState
              icon={KeyRound}
              title="No enrollment tokens"
              description="Create a token to get install commands for your devices."
              action={can("enrollment.write") ? <Button onClick={() => setCreating(true)}>Create token</Button> : undefined}
            />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>Token</TH>
                  <TH>Status</TH>
                  <TH className="hidden md:table-cell">Group</TH>
                  <TH>Uses</TH>
                  <TH className="hidden lg:table-cell">Expires</TH>
                  <TH className="w-48" />
                </TR>
              </THead>
              <TBody>
                {data.items.map((t) => {
                  const st = tokenState(t);
                  return (
                    <TR key={t.id}>
                      <TD>
                        <p className="font-medium">{t.description || "Untitled token"}</p>
                        <p className="font-mono text-xs text-muted-foreground">{t.token.slice(0, 12)}…</p>
                      </TD>
                      <TD>
                        <Badge variant={st.variant}>{st.label}</Badge>
                      </TD>
                      <TD className="hidden md:table-cell">{t.group_name ?? <span className="text-muted-foreground">—</span>}</TD>
                      <TD className="tabular">
                        {t.uses}
                        {t.max_uses != null && <span className="text-muted-foreground"> / {t.max_uses}</span>}
                      </TD>
                      <TD className="hidden text-muted-foreground lg:table-cell">
                        {t.expires_at ? <span title={formatDateTime(t.expires_at)}>{relativeTime(t.expires_at)}</span> : "Never"}
                      </TD>
                      <TD>
                        <div className="flex justify-end gap-2">
                          {st.label === "Active" && (
                            <Button size="sm" variant="outline" onClick={() => setShowing(t)}>
                              Install commands
                            </Button>
                          )}
                          {can("enrollment.write") && !t.revoked && (
                            <Button size="sm" variant="ghost" onClick={() => setRevoking(t)}>
                              Revoke
                            </Button>
                          )}
                        </div>
                      </TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          )}
        </Card>
      )}

      {creating && (
        <CreateToken
          onClose={() => setCreating(false)}
          onCreated={(t) => {
            setCreating(false);
            setShowing(t);
          }}
        />
      )}
      {showing && <InstallDialog token={showing} onClose={() => setShowing(null)} />}
      <ConfirmDialog
        open={!!revoking}
        onOpenChange={(o) => !o && setRevoking(null)}
        title="Revoke this token?"
        description="New enrollments with it will be refused. Devices already enrolled are not affected."
        confirmLabel="Revoke"
        loading={revoke.isPending}
        onConfirm={() => revoking && revoke.mutate(revoking.id)}
      />
    </>
  );
}

function CreateToken({ onClose, onCreated }: { onClose: () => void; onCreated: (t: EnrollmentToken) => void }) {
  const qc = useQueryClient();
  const groups = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const [description, setDescription] = useState("");
  const [groupId, setGroupId] = useState("");
  const [expiry, setExpiry] = useState("168");
  const [maxUses, setMaxUses] = useState("");
  const create = useMutation({
    mutationFn: () =>
      api.enrollment.create({
        description,
        group_id: groupId || undefined,
        expires_in_hours: expiry ? Number(expiry) : undefined,
        max_uses: maxUses ? Number(maxUses) : undefined,
      }),
    onSuccess: (t) => {
      void qc.invalidateQueries({ queryKey: qk.enrollment });
      onCreated(t);
    },
    onError: (e) => toast.error("Couldn't create token", e),
  });
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="New enrollment token"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={create.isPending} onClick={() => create.mutate()}>
            Create token
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Description" htmlFor="tdesc">
          <Input id="tdesc" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="e.g. London office laptops" />
        </Field>
        <Field label="Enroll into group" htmlFor="tgroup">
          <Select
            id="tgroup"
            value={groupId}
            onValueChange={setGroupId}
            placeholder="No group"
            options={(groups.data?.items ?? []).map((g) => ({ value: g.id, label: g.name }))}
          />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Expires" htmlFor="texp">
            <Select
              id="texp"
              value={expiry}
              onValueChange={setExpiry}
              options={[
                { value: "1", label: "In 1 hour" },
                { value: "24", label: "In 24 hours" },
                { value: "168", label: "In 7 days" },
                { value: "720", label: "In 30 days" },
                { value: "", label: "Never" },
              ]}
            />
          </Field>
          <Field label="Max devices" htmlFor="tmax" hint="Blank = unlimited">
            <Input id="tmax" type="number" min={1} value={maxUses} onChange={(e) => setMaxUses(e.target.value)} />
          </Field>
        </div>
      </div>
    </Dialog>
  );
}

function CopyBlock({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="group relative">
      <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-md border border-border bg-muted/50 p-3 pr-12 font-mono text-xs">{text}</pre>
      <Button
        size="icon-sm"
        variant="outline"
        className="absolute right-2 top-2"
        aria-label="Copy command"
        onClick={async () => {
          if (await copyToClipboard(text)) {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          } else toast.error("Couldn't copy to clipboard");
        }}
      >
        {copied ? <Check className="text-success" /> : <Copy />}
      </Button>
    </div>
  );
}

function InstallDialog({ token, onClose }: { token: EnrollmentToken; onClose: () => void }) {
  const { data, isLoading, error } = useQuery({ queryKey: ["install", token.id], queryFn: () => api.enrollment.install(token.id) });
  const rows = data
    ? [
        { icon: Monitor, os: "Windows", note: "Run in an elevated PowerShell (Run as Administrator).", cmd: data.windows },
        { icon: Apple, os: "macOS", note: "Run in Terminal; you'll be asked for an admin password.", cmd: data.macos },
        { icon: Server, os: "Linux", note: "Run as root or with sudo. Works on Debian/Ubuntu, RHEL/Fedora and derivatives.", cmd: data.linux },
      ]
    : [];
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title="Install the agent" description={token.description || undefined} size="lg">
      {error ? (
        <p className="text-sm text-destructive">Couldn&apos;t load install commands.</p>
      ) : isLoading ? (
        <Skeleton className="h-48" />
      ) : (
        <div className="space-y-5">
          {rows.map((r) => (
            <Card key={r.os} className="shadow-none">
              <CardHeader className="pb-2">
                <CardTitle className="flex items-center gap-2">
                  <r.icon className="h-4 w-4 text-muted-foreground" /> {r.os}
                </CardTitle>
                <p className="text-xs text-muted-foreground">{r.note}</p>
              </CardHeader>
              <CardContent>
                <CopyBlock text={r.cmd} />
              </CardContent>
            </Card>
          ))}
          <p className="text-xs text-muted-foreground">
            The token is a secret: anyone with it can enroll a device into your organization until it expires or is revoked.
          </p>
        </div>
      )}
    </Dialog>
  );
}

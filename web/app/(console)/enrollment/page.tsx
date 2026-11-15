"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Apple, Check, ChevronDown, Copy, KeyRound, Monitor, Plus, Server, Terminal } from "lucide-react";
import { api, qk, type EnrollmentToken } from "@/lib/api";
import { useCan } from "@/lib/session";
import { cn, copyToClipboard, formatDateTime, relativeTime, relativeTimeShort } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Skeleton, TableSkeleton } from "@/components/ui/skeleton";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { Segmented } from "@/components/ui/tabs";
import { EmptyState, ErrorState, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

function tokenState(t: EnrollmentToken): { label: string; variant: "success" | "neutral" | "danger" } {
  if (t.revoked) return { label: "Revoked", variant: "danger" };
  if (t.expires_at && new Date(t.expires_at).getTime() < Date.now()) return { label: "Expired", variant: "neutral" };
  if (t.max_uses != null && t.uses >= t.max_uses) return { label: "Used up", variant: "neutral" };
  return { label: "Active", variant: "success" };
}

const GUIDE_KEY = "enrollment-guide";

// The three-step guide: open while there are no tokens, folded away once there
// are (unless the viewer opened it, which this browser remembers).
function useGuideOpen(hasTokens: boolean): [boolean, () => void] {
  const [stored, setStored] = useState<"open" | "closed" | null>(null);
  useEffect(() => {
    try {
      const v = localStorage.getItem(GUIDE_KEY);
      if (v === "open" || v === "closed") setStored(v);
    } catch {}
  }, []);
  const open = stored ? stored === "open" : !hasTokens;
  const toggle = () => {
    const next = open ? "closed" : "open";
    setStored(next);
    try {
      localStorage.setItem(GUIDE_KEY, next);
    } catch {}
  };
  return [open, toggle];
}

const STEPS = [
  { icon: KeyRound, title: "1. Create a token", body: "Optionally assign a group, an expiry and a use limit." },
  { icon: Terminal, title: "2. Run the installer", body: "Paste the one-liner as admin/root on the device." },
  { icon: Monitor, title: "3. Device appears", body: "It shows up online within seconds with full inventory." },
];

export default function EnrollmentPage() {
  const can = useCan();
  const qc = useQueryClient();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.enrollment, queryFn: api.enrollment.list });
  const [creating, setCreating] = useState(false);
  const [showing, setShowing] = useState<EnrollmentToken | null>(null);
  const [revoking, setRevoking] = useState<EnrollmentToken | null>(null);
  const [view, setView] = useState<"active" | "all">("active");
  const all = data?.items ?? [];
  const active = all.filter((t) => tokenState(t).label === "Active");
  const shown = view === "active" ? active : all;
  // While loading, keep the guide folded so it doesn't flash open then shut.
  const [guideOpen, toggleGuide] = useGuideOpen(isLoading || all.length > 0);

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

      <div className="mb-6">
        <button
          type="button"
          onClick={toggleGuide}
          aria-expanded={guideOpen}
          aria-controls="enrollment-guide"
          className="inline-flex items-center gap-1.5 rounded-md text-sm font-medium text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <ChevronDown className={cn("h-4 w-4 transition-transform", !guideOpen && "-rotate-90")} />
          How enrollment works
        </button>
        {guideOpen && (
          <div id="enrollment-guide" className="mt-3 grid grid-cols-1 gap-4 md:grid-cols-3">
            {STEPS.map((s) => (
              <Card key={s.title} className="flex gap-3 p-4">
                <s.icon className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
                <div>
                  <p className="text-sm font-medium">{s.title}</p>
                  <p className="text-sm text-muted-foreground">{s.body}</p>
                </div>
              </Card>
            ))}
          </div>
        )}
      </div>

      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={3} cols={5} />
          ) : !all.length ? (
            <EmptyState
              icon={KeyRound}
              title="No enrollment tokens"
              description="Create a token to get install commands for your devices."
              action={can("enrollment.write") ? <Button onClick={() => setCreating(true)}>Create token</Button> : undefined}
            />
          ) : (
            <>
              <div className="flex flex-wrap items-center gap-3 border-b border-border p-3">
                <Segmented
                  value={view}
                  onChange={setView}
                  options={[
                    { value: "active", label: `Active (${active.length})` },
                    { value: "all", label: `All (${all.length})` },
                  ]}
                />
                {view === "all" && <span className="text-xs text-muted-foreground">Includes revoked, expired and used-up tokens.</span>}
              </div>
              {!shown.length ? (
                <EmptyState
                  icon={KeyRound}
                  title="No active tokens"
                  description="Every token is revoked, expired or used up. Create a new one to enroll more devices."
                  action={
                    <Button variant="outline" onClick={() => setView("all")}>
                      Show all tokens
                    </Button>
                  }
                />
              ) : (
                <Table>
                  <THead>
                    <TR>
                      <TH>Token</TH>
                      <TH className="hidden sm:table-cell">Status</TH>
                      <TH className="hidden md:table-cell">Group</TH>
                      <TH className="hidden sm:table-cell">Devices enrolled</TH>
                      <TH className="hidden lg:table-cell">Last used</TH>
                      <TH className="hidden lg:table-cell">Created</TH>
                      <TH className="hidden lg:table-cell">Expires</TH>
                      <TH className="hidden w-48 sm:table-cell" />
                    </TR>
                  </THead>
                  <TBody>
                    {shown.map((t) => {
                      const st = tokenState(t);
                      // One set of buttons: in their own column from sm up, under the name on phones.
                      const actions = (
                        <>
                          {st.label === "Active" && (
                            <Button size="sm" variant="outline" onClick={() => setShowing(t)}>
                              Install commands
                            </Button>
                          )}
                          {/* Expired, used-up and revoked tokens can't enroll anything, so they get no buttons. */}
                          {can("enrollment.write") && st.label === "Active" && (
                            <Button
                              size="sm"
                              variant="ghost"
                              className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                              onClick={() => setRevoking(t)}
                            >
                              Revoke
                            </Button>
                          )}
                        </>
                      );
                      const count =
                        t.uses > 0 ? (
                          <Link
                            href={`/devices?token=${encodeURIComponent(t.id)}`}
                            className="font-medium text-primary hover:underline"
                            title="See the devices enrolled with this token"
                          >
                            {t.uses}
                          </Link>
                        ) : (
                          t.uses
                        );
                      return (
                        <TR key={t.id}>
                          <TD>
                            <p className="font-medium">{t.description || "Untitled token"}</p>
                            <p className="font-mono text-xs text-muted-foreground">{t.token.slice(0, 12)}…</p>
                            {/* Phones: status, count and buttons stack under the name instead of their own columns. */}
                            <div className="mt-2 flex items-center gap-2 text-sm tabular sm:hidden">
                              <Badge variant={st.variant}>{st.label}</Badge>
                              <span>
                                {count}
                                <span className="text-muted-foreground">
                                  {t.max_uses != null ? ` / ${t.max_uses} devices` : ` device${t.uses === 1 ? "" : "s"} · no limit`}
                                </span>
                              </span>
                            </div>
                            <p className="mt-1 text-xs text-muted-foreground sm:hidden">
                              {t.expires_at
                                ? `${new Date(t.expires_at).getTime() < Date.now() ? "Expired" : "Expires"} ${relativeTime(t.expires_at)}`
                                : "Never expires"}
                              {" · "}
                              {t.last_used_at ? `Last used ${relativeTimeShort(t.last_used_at)}` : "Never used"}
                              {t.group_name && ` · Group: ${t.group_name}`}
                            </p>
                            <div className="mt-2 flex gap-2 sm:hidden">{actions}</div>
                          </TD>
                          <TD className="hidden sm:table-cell">
                            <Badge variant={st.variant}>{st.label}</Badge>
                          </TD>
                          <TD className="hidden md:table-cell">{t.group_name ?? <span className="text-muted-foreground">—</span>}</TD>
                          <TD className="tabular hidden whitespace-nowrap sm:table-cell">
                            {count}
                            <span className="text-muted-foreground">{t.max_uses != null ? ` / ${t.max_uses}` : " · no limit"}</span>
                          </TD>
                          <TD className="hidden whitespace-nowrap text-muted-foreground lg:table-cell">
                            {t.last_used_at ? <span title={formatDateTime(t.last_used_at)}>{relativeTimeShort(t.last_used_at)}</span> : "Never"}
                          </TD>
                          <TD className="hidden whitespace-nowrap text-muted-foreground lg:table-cell">
                            <span title={`${formatDateTime(t.created_at)}${t.created_by ? ` by ${t.created_by}` : ""}`}>
                              {relativeTimeShort(t.created_at)}
                            </span>
                            {t.created_by && <p className="max-w-[12rem] truncate text-xs">{t.created_by}</p>}
                          </TD>
                          <TD className="hidden text-muted-foreground lg:table-cell">
                            {t.expires_at ? <span title={formatDateTime(t.expires_at)}>{relativeTime(t.expires_at)}</span> : "Never"}
                          </TD>
                          <TD className="hidden sm:table-cell">
                            <div className="flex justify-end gap-2">{actions}</div>
                          </TD>
                        </TR>
                      );
                    })}
                  </TBody>
                </Table>
              )}
            </>
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
        description: description.trim(),
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
          <Button loading={create.isPending} disabled={!description.trim()} onClick={() => create.mutate()}>
            Create token
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Description" htmlFor="tdesc" hint="Required. Names the token in this list and the audit log.">
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

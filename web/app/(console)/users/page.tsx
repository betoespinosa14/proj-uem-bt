"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2, Users } from "lucide-react";
import { api, qk, ROLES, type Role, type User } from "@/lib/api";
import { useMe } from "@/lib/session";
import { relativeTime, titleCase } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

const ROLE_INFO: Record<Role, string> = {
  owner: "Full access, including organization settings and managing owners.",
  admin: "Full access except organization settings.",
  helpdesk: "View devices, run commands and scripts, resolve alerts.",
  auditor: "Read-only access to devices plus the audit log.",
};

export default function UsersPage() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.users, queryFn: api.users.list });
  const [editing, setEditing] = useState<User | "new" | null>(null);
  const [deleting, setDeleting] = useState<User | null>(null);
  const isOwner = me?.user.role === "owner";
  const remove = useMutation({
    mutationFn: (id: string) => api.users.remove(id),
    onSuccess: () => {
      toast.success("User removed");
      setDeleting(null);
      void qc.invalidateQueries({ queryKey: qk.users });
    },
    onError: (e) => toast.error("Couldn't remove user", e),
  });

  return (
    <>
      <PageHeader
        title="Users & roles"
        description="Invite teammates and control what they can do."
        actions={
          <Button onClick={() => setEditing("new")}>
            <Plus /> Add user
          </Button>
        }
      />
      <div className="mb-6 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {ROLES.map((r) => (
          <Card key={r} className="shadow-none">
            <CardContent className="pt-4">
              <p className="text-sm font-medium">{titleCase(r)}</p>
              <p className="mt-1 text-xs text-muted-foreground">{ROLE_INFO[r]}</p>
            </CardContent>
          </Card>
        ))}
      </div>
      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <Card className="overflow-hidden">
          {isLoading ? (
            <TableSkeleton rows={3} cols={4} />
          ) : !data?.items.length ? (
            <EmptyState icon={Users} title="No users" />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>User</TH>
                  <TH>Role</TH>
                  <TH className="hidden md:table-cell">Last sign-in</TH>
                  <TH className="w-24" />
                </TR>
              </THead>
              <TBody>
                {data.items.map((u) => {
                  const self = u.id === me?.user.id;
                  const locked = u.role === "owner" && !isOwner;
                  return (
                    <TR key={u.id}>
                      <TD>
                        <p className="font-medium">
                          {u.name || u.email} {self && <span className="text-xs font-normal text-muted-foreground">(you)</span>}
                        </p>
                        <p className="text-xs text-muted-foreground">{u.email}</p>
                      </TD>
                      <TD>
                        <Badge variant={u.role === "owner" ? "primary" : "outline"}>{titleCase(u.role)}</Badge>
                      </TD>
                      <TD className="hidden text-muted-foreground md:table-cell">{u.last_login_at ? relativeTime(u.last_login_at) : "Never"}</TD>
                      <TD>
                        {!locked && (
                          <div className="flex justify-end gap-1">
                            <Button variant="ghost" size="icon-sm" onClick={() => setEditing(u)} aria-label={`Edit ${u.email}`}>
                              <Pencil />
                            </Button>
                            {!self && (
                              <Button variant="ghost" size="icon-sm" onClick={() => setDeleting(u)} aria-label={`Remove ${u.email}`}>
                                <Trash2 />
                              </Button>
                            )}
                          </div>
                        )}
                      </TD>
                    </TR>
                  );
                })}
              </TBody>
            </Table>
          )}
        </Card>
      )}
      {editing && <UserDialog user={editing === "new" ? null : editing} canGrantOwner={isOwner} onClose={() => setEditing(null)} />}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Remove ${deleting?.email}?`}
        description="They are signed out immediately and can no longer access this organization."
        confirmLabel="Remove user"
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </>
  );
}

function UserDialog({ user, canGrantOwner, onClose }: { user: User | null; canGrantOwner: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const [email, setEmail] = useState(user?.email ?? "");
  const [name, setName] = useState(user?.name ?? "");
  const [role, setRole] = useState<Role>(user?.role ?? "helpdesk");
  const [password, setPassword] = useState("");
  const save = useMutation({
    mutationFn: () =>
      user
        ? api.users.update(user.id, { name, role, ...(password ? { password } : {}) })
        : api.users.create({ email, name, role, password }),
    onSuccess: () => {
      toast.success(user ? "User updated" : "User added", user ? undefined : "Share the temporary password with them securely.");
      void qc.invalidateQueries({ queryKey: qk.users });
      onClose();
    },
    onError: (e) => toast.error("Couldn't save user", e),
  });
  const roles = ROLES.filter((r) => r !== "owner" || canGrantOwner);
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={user ? "Edit user" : "Add user"}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={save.isPending} disabled={!user && (!email || password.length < 10)} onClick={() => save.mutate()}>
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Email" htmlFor="uemail">
          <Input id="uemail" type="email" value={email} disabled={!!user} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label="Name" htmlFor="uname">
          <Input id="uname" value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Role" htmlFor="urole" hint={ROLE_INFO[role]}>
          <Select id="urole" value={role} onValueChange={(v) => setRole(v as Role)} options={roles.map((r) => ({ value: r, label: titleCase(r) }))} />
        </Field>
        <Field
          label={user ? "New password" : "Temporary password"}
          htmlFor="upass"
          hint={user ? "Leave blank to keep the current password. Changing it signs the user out." : "At least 10 characters."}
        >
          <Input id="upass" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
      </div>
    </Dialog>
  );
}

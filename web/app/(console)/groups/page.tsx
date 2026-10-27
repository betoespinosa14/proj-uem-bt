"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderTree, Pencil, Plus, Trash2 } from "lucide-react";
import { api, qk, type Group } from "@/lib/api";
import { useCan } from "@/lib/session";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { ConfirmDialog, Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { TableSkeleton } from "@/components/ui/skeleton";
import { Table, TBody, TD, TH, THead, TR } from "@/components/ui/table";
import { EmptyState, ErrorState, PageHeader, TagInput } from "@/components/common";
import { toast } from "@/components/toast";

export default function GroupsPage() {
  const can = useCan();
  const qc = useQueryClient();
  const { data, isLoading, error, refetch } = useQuery({ queryKey: qk.groups, queryFn: api.groups.list });
  const [editing, setEditing] = useState<Group | "new" | null>(null);
  const [deleting, setDeleting] = useState<Group | null>(null);

  const remove = useMutation({
    mutationFn: (id: string) => api.groups.remove(id),
    onSuccess: () => {
      toast.success("Group deleted");
      setDeleting(null);
      void qc.invalidateQueries({ queryKey: qk.groups });
      void qc.invalidateQueries({ queryKey: qk.devices() });
    },
    onError: (e) => toast.error("Couldn't delete group", e),
  });

  return (
    <>
      <PageHeader
        title="Groups"
        description="Organize devices and apply group policy such as the application blocklist."
        actions={
          can("devices.write") && (
            <Button onClick={() => setEditing("new")}>
              <Plus /> New group
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
            <EmptyState
              icon={FolderTree}
              title="No groups yet"
              description="Groups let you target tasks and compliance rules, and enroll devices straight into a group."
            />
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>Name</TH>
                  <TH>Devices</TH>
                  <TH className="hidden md:table-cell">Blocked apps</TH>
                  <TH className="w-24" />
                </TR>
              </THead>
              <TBody>
                {data.items.map((g) => (
                  <TR key={g.id}>
                    <TD>
                      <p className="font-medium">{g.name}</p>
                      {g.description && <p className="text-xs text-muted-foreground">{g.description}</p>}
                    </TD>
                    <TD>
                      <Link href={`/devices?group=${g.id}`} className="tabular hover:underline">
                        {g.device_count}
                      </Link>
                    </TD>
                    <TD className="hidden md:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {g.blocked_apps.length ? (
                          g.blocked_apps.map((a) => (
                            <Badge key={a} variant="danger" className="font-mono">
                              {a}
                            </Badge>
                          ))
                        ) : (
                          <span className="text-muted-foreground">None</span>
                        )}
                      </div>
                    </TD>
                    <TD>
                      {can("devices.write") && (
                        <div className="flex justify-end gap-1">
                          <Button variant="ghost" size="icon-sm" onClick={() => setEditing(g)} aria-label={`Edit ${g.name}`}>
                            <Pencil />
                          </Button>
                          <Button variant="ghost" size="icon-sm" onClick={() => setDeleting(g)} aria-label={`Delete ${g.name}`}>
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
      {editing && <GroupDialog group={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      <ConfirmDialog
        open={!!deleting}
        onOpenChange={(o) => !o && setDeleting(null)}
        title={`Delete ${deleting?.name}?`}
        description="Devices in this group are kept but become ungrouped, and its blocklist stops applying."
        confirmLabel="Delete group"
        loading={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
      />
    </>
  );
}

function GroupDialog({ group, onClose }: { group: Group | null; onClose: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(group?.name ?? "");
  const [description, setDescription] = useState(group?.description ?? "");
  const [blocked, setBlocked] = useState<string[]>(group?.blocked_apps ?? []);
  const save = useMutation({
    mutationFn: () => {
      const body = { name, description, blocked_apps: blocked };
      return group ? api.groups.update(group.id, body) : api.groups.create(body);
    },
    onSuccess: () => {
      toast.success(group ? "Group updated" : "Group created");
      void qc.invalidateQueries({ queryKey: qk.groups });
      onClose();
    },
    onError: (e) => toast.error("Couldn't save group", e),
  });
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={group ? "Edit group" : "New group"}
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={save.isPending} disabled={!name.trim()} onClick={() => save.mutate()}>
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Name" htmlFor="gname">
          <Input id="gname" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Retail kiosks" />
        </Field>
        <Field label="Description" htmlFor="gdesc">
          <Input id="gdesc" value={description} onChange={(e) => setDescription(e.target.value)} />
        </Field>
        <Field
          label="Blocked applications"
          htmlFor="gblocked"
          hint="Process names the agent terminates on devices in this group, e.g. utorrent, steam.exe. Each block is audit-logged."
        >
          <TagInput id="gblocked" value={blocked} onChange={setBlocked} placeholder="Type a process name and press Enter" />
        </Field>
      </div>
    </Dialog>
  );
}

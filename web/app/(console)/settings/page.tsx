"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, qk, type Me } from "@/lib/api";
import { useCan, useMe } from "@/lib/session";
import { formatDate, titleCase } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Field, Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { ErrorState, KeyValues, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

export default function SettingsPage() {
  const can = useCan();
  const qc = useQueryClient();
  const { data: me } = useMe();
  const { data: org, isLoading, error, refetch } = useQuery({ queryKey: qk.org, queryFn: api.org.get });
  const [name, setName] = useState("");
  const [serverUrl, setServerUrl] = useState("");
  useEffect(() => {
    if (org) {
      setName(org.name);
      setServerUrl(org.server_url);
    }
  }, [org]);
  const save = useMutation({
    mutationFn: () => api.org.update({ name, server_url: serverUrl }),
    onSuccess: (o) => {
      qc.setQueryData(qk.org, o);
      qc.setQueryData<Me>(qk.me, (m) => (m ? { ...m, org: o } : m));
      toast.success("Settings saved");
    },
    onError: (e) => toast.error("Couldn't save settings", e),
  });
  const tracking = useMutation({
    mutationFn: (b: { usage_tracking?: boolean; idle_threshold_sec?: number }) => api.org.update(b),
    onSuccess: (o, b) => {
      qc.setQueryData(qk.org, o);
      qc.setQueryData<Me>(qk.me, (m) => (m ? { ...m, org: o } : m));
      toast.success(
        b.usage_tracking !== undefined
          ? o.usage_tracking
            ? "Usage tracking turned on"
            : "Usage tracking turned off"
          : `Idle time set to ${o.idle_threshold_sec / 60} min`,
      );
    },
    onError: (e) => toast.error("Couldn't change usage tracking", e),
  });
  const editable = can("settings.write");

  return (
    <>
      <PageHeader title="Settings" description="Organization details and your account." />
      {error ? (
        <ErrorState error={error} onRetry={refetch} />
      ) : (
        <div className="grid gap-4 lg:grid-cols-[2fr_1fr]">
          <Card>
            <CardHeader>
              <CardTitle>Organization</CardTitle>
              <CardDescription>{editable ? "Only owners can change these settings." : "Read-only: only owners can change these settings."}</CardDescription>
            </CardHeader>
            <CardContent>
              {isLoading ? (
                <Skeleton className="h-32" />
              ) : (
                <fieldset disabled={!editable} className="space-y-4">
                  <Field label="Organization name" htmlFor="oname">
                    <Input id="oname" value={name} onChange={(e) => setName(e.target.value)} />
                  </Field>
                  <Field
                    label="Public server URL"
                    htmlFor="ourl"
                    hint="The address agents use to reach this server, used in install commands. Leave blank to use the address you're browsing from."
                  >
                    <Input id="ourl" type="url" value={serverUrl} onChange={(e) => setServerUrl(e.target.value)} placeholder="https://uem.example.com" />
                  </Field>
                </fieldset>
              )}
            </CardContent>
            {editable && (
              <CardFooter className="justify-end">
                <Button loading={save.isPending} disabled={!name.trim()} onClick={() => save.mutate()}>
                  Save changes
                </Button>
              </CardFooter>
            )}
          </Card>
          <Card className="lg:order-last">
            <CardHeader>
              <CardTitle>Application usage tracking</CardTitle>
              <CardDescription>
                Records how much each user actively uses each installed app, for license and subscription audits.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {isLoading || !org ? (
                <Skeleton className="h-6 w-40" />
              ) : (
                <>
                  <label htmlFor="usage-tracking" className="flex items-center gap-3 text-sm font-medium">
                    <Switch
                      id="usage-tracking"
                      checked={org.usage_tracking}
                      disabled={!editable || tracking.isPending}
                      onCheckedChange={(v) => tracking.mutate({ usage_tracking: v })}
                    />
                    {org.usage_tracking ? "On" : "Off"}
                  </label>
                  <Field
                    label="Count as idle after"
                    htmlFor="idle-after"
                    hint="Time in front of an app counts as active until this long passes without keyboard or mouse input. Applies from now on."
                  >
                    <Select
                      id="idle-after"
                      className="w-40"
                      value={String(org.idle_threshold_sec)}
                      disabled={!editable || tracking.isPending}
                      onValueChange={(v) => tracking.mutate({ idle_threshold_sec: Number(v) })}
                      options={[1, 2, 3, 5, 10, 15].map((m) => ({ value: String(m * 60), label: `${m} minute${m === 1 ? "" : "s"}` }))}
                    />
                  </Field>
                </>
              )}
              <p className="text-xs text-muted-foreground">
                Agents record which app is in front and the time since the last keyboard or mouse input, plus which apps are open. They never record
                keystrokes, window titles or screen contents. This is still employee monitoring: inform staff before turning it on. Changes are
                recorded in the audit log, and usage data is kept for 400 days. Active time isn&apos;t available on Linux.
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Your account</CardTitle>
            </CardHeader>
            <CardContent>
              {me && (
                <KeyValues
                  items={[
                    ["Name", me.user.name],
                    ["Email", me.user.email],
                    ["Role", titleCase(me.user.role)],
                    ["Member since", formatDate(me.user.created_at)],
                    ["Organization ID", <span key="id" className="font-mono text-xs">{me.org.id}</span>],
                  ]}
                />
              )}
            </CardContent>
          </Card>
        </div>
      )}
    </>
  );
}

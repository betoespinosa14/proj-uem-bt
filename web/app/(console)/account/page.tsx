"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import QRCode from "qrcode";
import { Copy, Download, KeyRound, ShieldCheck, ShieldOff, Smartphone } from "lucide-react";
import { api, qk, type MfaSetup } from "@/lib/api";
import { useMe } from "@/lib/session";
import { formatDate } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { ErrorState, PageHeader } from "@/components/common";
import { toast } from "@/components/toast";

export default function AccountSecurityPage() {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const status = useQuery({ queryKey: qk.mfa, queryFn: api.auth.mfa.status });
  const [setup, setSetup] = useState<MfaSetup | null>(null);
  const [codes, setCodes] = useState<string[] | null>(null);
  const [askPassword, setAskPassword] = useState<"disable" | "codes" | null>(null);

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: qk.mfa });
    void qc.invalidateQueries({ queryKey: qk.me });
  };
  const start = useMutation({
    mutationFn: api.auth.mfa.setup,
    onSuccess: setSetup,
    onError: (e) => toast.error("Couldn't start the setup", e),
  });
  const forget = useMutation({
    mutationFn: api.auth.mfa.forgetBrowsers,
    onSuccess: () => {
      toast.success("Remembered browsers forgotten", "Every browser asks for a code at the next sign-in.");
      refresh();
    },
    onError: (e) => toast.error("Couldn't forget browsers", e),
  });

  const s = status.data;
  return (
    <>
      <PageHeader title="Account security" description={me ? `Sign-in settings for ${me.user.email}.` : "Your sign-in settings."} />
      {status.error ? (
        <ErrorState error={status.error} onRetry={status.refetch} />
      ) : (
        <div className="grid max-w-3xl grid-cols-1 gap-4">
          <Card>
            <CardHeader>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <CardTitle>Two-step sign-in</CardTitle>
                {s && (s.enabled ? <Badge variant="success" dot>On</Badge> : <Badge variant="neutral">Off</Badge>)}
              </div>
              <CardDescription>
                After your password, BEACON asks for a 6-digit code from an authenticator app on your phone, such as Google Authenticator or
                Microsoft Authenticator. Someone who learns your password still can&apos;t sign in without your phone.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {!s ? (
                <Skeleton className="h-24" />
              ) : s.enabled ? (
                <div className="space-y-5">
                  <dl className="grid grid-cols-1 gap-4 text-sm sm:grid-cols-3">
                    <div>
                      <dt className="text-muted-foreground">Turned on</dt>
                      <dd className="font-medium">{formatDate(s.enabled_at)}</dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Recovery codes left</dt>
                      <dd className={s.recovery_codes_left <= 3 ? "font-medium text-warning" : "font-medium"}>
                        {s.recovery_codes_left} of 10
                        {s.recovery_codes_left <= 3 && <span className="block text-xs font-normal">Make new ones soon</span>}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-muted-foreground">Remembered browsers</dt>
                      <dd className="font-medium">{s.trusted_browsers}</dd>
                    </div>
                  </dl>
                  <div className="flex flex-wrap gap-2">
                    <Button variant="outline" onClick={() => setAskPassword("codes")}>
                      <KeyRound /> New recovery codes
                    </Button>
                    <Button variant="outline" disabled={!s.trusted_browsers} loading={forget.isPending} onClick={() => forget.mutate()}>
                      Forget remembered browsers
                    </Button>
                    <Button variant="ghost" className="text-destructive" onClick={() => setAskPassword("disable")}>
                      <ShieldOff /> Turn off
                    </Button>
                  </div>
                </div>
              ) : setup ? (
                <SetupSteps
                  setup={setup}
                  onCancel={() => setSetup(null)}
                  onEnabled={(c) => {
                    setSetup(null);
                    setCodes(c);
                    refresh();
                  }}
                />
              ) : (
                <Button onClick={() => start.mutate()} loading={start.isPending}>
                  <ShieldCheck /> Turn on two-step sign-in
                </Button>
              )}
            </CardContent>
          </Card>
          <p className="text-xs text-muted-foreground">
            Lost your phone and your recovery codes? Ask another owner or admin to reset your two-step sign-in from Users.
          </p>
        </div>
      )}
      {codes && <RecoveryCodesDialog codes={codes} email={me?.user.email ?? ""} onClose={() => setCodes(null)} />}
      {askPassword && (
        <PasswordDialog
          mode={askPassword}
          onClose={() => setAskPassword(null)}
          onDone={(newCodes) => {
            setAskPassword(null);
            if (newCodes) setCodes(newCodes);
            else toast.success("Two-step sign-in turned off");
            refresh();
          }}
        />
      )}
    </>
  );
}

function SetupSteps({ setup, onCancel, onEnabled }: { setup: MfaSetup; onCancel: () => void; onEnabled: (codes: string[]) => void }) {
  const [qr, setQr] = useState<string | null>(null);
  const [code, setCode] = useState("");
  useEffect(() => {
    // Drawn in the browser, so the secret never goes to another service.
    QRCode.toDataURL(setup.uri, { margin: 1, width: 192, errorCorrectionLevel: "M" })
      .then(setQr)
      .catch(() => setQr(null));
  }, [setup.uri]);
  const enable = useMutation({
    mutationFn: () => api.auth.mfa.enable(code),
    onSuccess: (r) => onEnabled(r.recovery_codes),
    onError: () => setCode(""),
  });
  const key = setup.secret.replace(/(.{4})/g, "$1 ").trim();

  return (
    <ol className="space-y-6 text-sm">
      <li className="flex gap-3">
        <StepNumber n={1} />
        <div>
          <p className="font-medium">Install an authenticator app on your phone</p>
          <p className="text-muted-foreground">Google Authenticator, Microsoft Authenticator or any app that supports 6-digit codes.</p>
        </div>
      </li>
      <li className="flex gap-3">
        <StepNumber n={2} />
        <div className="min-w-0 space-y-3">
          <p className="font-medium">In the app, add an account and scan this QR code</p>
          <div className="inline-block rounded-lg border border-border bg-white p-2">
            {qr ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={qr} width={192} height={192} alt="QR code for your authenticator app" />
            ) : (
              <Skeleton className="h-48 w-48" />
            )}
          </div>
          <div>
            <p className="text-muted-foreground">Can&apos;t scan it? Choose &ldquo;Enter a setup key&rdquo; in the app and type:</p>
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <code className="break-all rounded bg-muted px-2 py-1 font-mono text-xs">{key}</code>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Copy setup key"
                onClick={() => navigator.clipboard.writeText(setup.secret).then(() => toast.success("Setup key copied"))}
              >
                <Copy />
              </Button>
            </div>
          </div>
        </div>
      </li>
      <li className="flex gap-3">
        <StepNumber n={3} />
        <form
          className="min-w-0 flex-1 space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            enable.mutate();
          }}
        >
          <Field
            label="Enter the 6-digit code the app shows for BEACON"
            htmlFor="setup-code"
            error={enable.error ? "That code didn't work. Use the newest code, and check that your phone's clock is set automatically." : undefined}
          >
            <Input
              id="setup-code"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={7}
              placeholder="123456"
              className="w-40 font-mono text-lg tracking-[0.3em]"
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/[^0-9 ]/g, ""))}
            />
          </Field>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" loading={enable.isPending} disabled={code.replace(/\s/g, "").length !== 6}>
              <Smartphone /> Turn on
            </Button>
            <Button variant="outline" onClick={onCancel}>
              Cancel
            </Button>
          </div>
        </form>
      </li>
    </ol>
  );
}

function StepNumber({ n }: { n: number }) {
  return (
    <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">{n}</span>
  );
}

function RecoveryCodesDialog({ codes, email, onClose }: { codes: string[]; email: string; onClose: () => void }) {
  const [saved, setSaved] = useState(false);
  const text = `BEACON recovery codes for ${email}\nEach code works once. Keep them somewhere safe, away from your phone.\n\n${codes.join("\n")}\n`;
  const download = () => {
    const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "beacon-recovery-codes.txt";
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && saved && onClose()}
      title="Save your recovery codes"
      description="If you lose your phone, each of these codes signs you in once. This is the only time they're shown."
      size="sm"
      footer={
        <Button disabled={!saved} onClick={onClose}>
          Done
        </Button>
      }
    >
      <div className="space-y-4">
        <ul className="grid grid-cols-2 gap-x-6 gap-y-1 rounded-lg bg-muted p-4 font-mono text-sm">
          {codes.map((c) => (
            <li key={c}>{c}</li>
          ))}
        </ul>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => navigator.clipboard.writeText(text).then(() => toast.success("Recovery codes copied"))}>
            <Copy /> Copy
          </Button>
          <Button variant="outline" onClick={download}>
            <Download /> Download
          </Button>
        </div>
        <label htmlFor="codes-saved" className="flex items-center gap-2 text-sm">
          <Checkbox id="codes-saved" checked={saved} onChange={(e) => setSaved(e.target.checked)} />
          I&apos;ve saved these codes somewhere safe
        </label>
      </div>
    </Dialog>
  );
}

function PasswordDialog({
  mode,
  onClose,
  onDone,
}: {
  mode: "disable" | "codes";
  onClose: () => void;
  onDone: (codes?: string[]) => void;
}) {
  const [password, setPassword] = useState("");
  const run = useMutation({
    mutationFn: async () =>
      mode === "disable" ? (await api.auth.mfa.disable(password), undefined) : (await api.auth.mfa.newRecoveryCodes(password)).recovery_codes,
    onSuccess: onDone,
  });
  const disable = mode === "disable";
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={disable ? "Turn off two-step sign-in?" : "Make new recovery codes?"}
      description={
        disable
          ? "Your password alone will sign you in again, and remembered browsers are forgotten."
          : "Your current recovery codes stop working, and you get 10 new ones."
      }
      size="sm"
      footer={
        <>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="submit"
            form="confirm-password"
            variant={disable ? "destructive" : "default"}
            loading={run.isPending}
            disabled={!password}
          >
            {disable ? "Turn off" : "Make new codes"}
          </Button>
        </>
      }
    >
      <form
        id="confirm-password"
        onSubmit={(e) => {
          e.preventDefault();
          run.mutate();
        }}
      >
        <Field
          label="Confirm with your password"
          htmlFor="confirm-pw"
          error={run.error ? (run.error.message === "that password isn't right" ? "That password isn't right." : run.error.message) : undefined}
        >
          <Input id="confirm-pw" type="password" autoComplete="current-password" autoFocus value={password} onChange={(e) => setPassword(e.target.value)} />
        </Field>
      </form>
    </Dialog>
  );
}

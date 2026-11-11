"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, errorMessage, needsCode, qk, type Me } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox, Field, Input } from "@/components/ui/input";

/** Only allow same-site relative redirects after login. */
function safeNext(next: string | null): string {
  return next && next.startsWith("/") && !next.startsWith("//") ? next : "/";
}

function LoginForm() {
  const router = useRouter();
  const params = useSearchParams();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [step, setStep] = useState<"password" | "code">("password");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const signedIn = (me: Me) => {
    qc.setQueryData(qk.me, me);
    router.replace(safeNext(params.get("next")));
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      const res = await api.auth.login({ email, password });
      if (needsCode(res)) {
        setStep("code");
        setLoading(false);
        return;
      }
      signedIn(res);
    } catch (err) {
      setError(errorMessage(err));
      setLoading(false);
    }
  };

  if (step === "code") {
    return (
      <CodeStep
        onSignedIn={signedIn}
        onBack={() => {
          setStep("password");
          setPassword("");
          setError(null);
        }}
      />
    );
  }

  return (
    <Card className="p-6">
      <h2 className="mb-1 text-base font-semibold">Sign in</h2>
      <p className="mb-5 text-sm text-muted-foreground">Use your organization account.</p>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Email" htmlFor="email">
          <Input id="email" type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <Field label="Password" htmlFor="password">
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <Button type="submit" className="w-full" loading={loading}>
          Sign in
        </Button>
      </form>
      <p className="mt-5 text-center text-sm text-muted-foreground">
        New here?{" "}
        <Link href="/signup" className="font-medium text-primary hover:underline">
          Create an organization
        </Link>
      </p>
    </Card>
  );
}

/** Second step for accounts with two-step sign-in on. */
function CodeStep({ onSignedIn, onBack }: { onSignedIn: (me: Me) => void; onBack: () => void }) {
  const [useRecovery, setUseRecovery] = useState(false);
  const [code, setCode] = useState("");
  const [remember, setRemember] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      onSignedIn(await api.auth.loginCode({ code, remember }));
    } catch (err) {
      const msg = errorMessage(err);
      setError(
        msg === "that code didn't work"
          ? useRecovery
            ? "That recovery code didn't work. Each code works only once."
            : "That code didn't work. Use the newest code in your app, and check that your phone's clock is set automatically."
          : msg,
      );
      setCode("");
      setLoading(false);
    }
  };

  return (
    <Card className="p-6">
      <h2 className="mb-1 text-base font-semibold">Two-step sign-in</h2>
      <p className="mb-5 text-sm text-muted-foreground">
        {useRecovery
          ? "Enter one of the recovery codes you saved when you turned on two-step sign-in."
          : "Open your authenticator app and enter the 6-digit code for BEACON."}
      </p>
      <form onSubmit={submit} className="space-y-4">
        {useRecovery ? (
          <Field label="Recovery code" htmlFor="code">
            <Input
              id="code"
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              placeholder="xxxxx-xxxxx"
              required
              autoFocus
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </Field>
        ) : (
          <Field label="Code" htmlFor="code">
            <Input
              id="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9 ]{6,7}"
              maxLength={7}
              placeholder="123456"
              required
              autoFocus
              className="font-mono text-lg tracking-[0.3em]"
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/[^0-9 ]/g, ""))}
            />
          </Field>
        )}
        <label htmlFor="remember" className="flex items-center gap-2 text-sm">
          <Checkbox id="remember" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
          Remember this browser for 30 days
        </label>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <Button type="submit" className="w-full" loading={loading}>
          Verify and sign in
        </Button>
      </form>
      <div className="mt-5 flex flex-wrap items-center justify-between gap-2 text-sm">
        <button type="button" className="font-medium text-primary hover:underline" onClick={onBack}>
          Back
        </button>
        <button
          type="button"
          className="font-medium text-primary hover:underline"
          onClick={() => {
            setUseRecovery(!useRecovery);
            setCode("");
            setError(null);
          }}
        >
          {useRecovery ? "Use the code from my app" : "Use a recovery code instead"}
        </button>
      </div>
    </Card>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}

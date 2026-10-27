"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api, errorMessage, qk } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Field, Input } from "@/components/ui/input";

export default function SignupPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const [form, setForm] = useState({ org_name: "", name: "", email: "", password: "" });
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value });

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    if (form.password.length < 10) {
      setError("Password must be at least 10 characters.");
      return;
    }
    setLoading(true);
    try {
      const me = await api.auth.signup(form);
      qc.setQueryData(qk.me, me);
      router.replace("/enrollment");
    } catch (err) {
      setError(errorMessage(err));
      setLoading(false);
    }
  };

  return (
    <Card className="p-6">
      <h2 className="mb-1 text-base font-semibold">Create your organization</h2>
      <p className="mb-5 text-sm text-muted-foreground">You&apos;ll be its owner and can invite your team afterwards.</p>
      <form onSubmit={submit} className="space-y-4">
        <Field label="Organization name" htmlFor="org">
          <Input id="org" required value={form.org_name} onChange={set("org_name")} placeholder="Acme Corp" />
        </Field>
        <Field label="Your name" htmlFor="name">
          <Input id="name" autoComplete="name" value={form.name} onChange={set("name")} />
        </Field>
        <Field label="Work email" htmlFor="email">
          <Input id="email" type="email" autoComplete="email" required value={form.email} onChange={set("email")} />
        </Field>
        <Field label="Password" htmlFor="password" hint="At least 10 characters.">
          <Input id="password" type="password" autoComplete="new-password" required value={form.password} onChange={set("password")} />
        </Field>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <Button type="submit" className="w-full" loading={loading}>
          Create organization
        </Button>
      </form>
      <p className="mt-5 text-center text-sm text-muted-foreground">
        Already have an account?{" "}
        <Link href="/login" className="font-medium text-primary hover:underline">
          Sign in
        </Link>
      </p>
    </Card>
  );
}

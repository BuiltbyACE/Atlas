"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, TextInput } from "@/components/form";
import { Card, ProblemNotice } from "@/components/ui";
import { acceptInvite } from "@/lib/auth/flow";
import { useMutation } from "@/lib/use-mutation";

/** Matches `minPasswordLength` in atlas-api (blueprint section 13.1). */
const MIN_PASSWORD = 12;

/**
 * Complete an invitation (`POST /api/v1/auth/invites/accept`). Go validates the
 * token, name and password; this form only collects them and confirms the two
 * password fields agree before sending. On success the person is sent to /login.
 */
export function InviteForm({ token }: { token: string }) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [mismatch, setMismatch] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const password = String(data.get("password") ?? "");
    const confirm = String(data.get("confirm") ?? "");
    if (password !== confirm) {
      setMismatch(true);
      return;
    }
    setMismatch(false);

    const result = await run(() =>
      acceptInvite({
        token,
        full_name: String(data.get("full_name") ?? "").trim(),
        password,
      }),
    );
    if (result) {
      router.replace("/login");
    }
  }

  if (!token) {
    return (
      <Card
        title="Invitation link needed"
        description="This page needs the token from your invitation email."
      >
        <p className="text-sm text-slate-600">
          Nothing to accept without a token. Ask a SafariStack system
          administrator to send you a fresh invitation.
        </p>
      </Card>
    );
  }

  return (
    <Card
      title="Set up your Atlas account"
      description="Choose your name and a password to finish joining."
    >
      <form onSubmit={onSubmit} className="space-y-4">
        {problem && (
          <ProblemNotice
            error={problem}
            title="Could not accept the invitation"
          />
        )}

        <Field label="Your full name" htmlFor="invite-name">
          <TextInput
            id="invite-name"
            name="full_name"
            autoComplete="name"
            required
          />
        </Field>

        <Field
          label="Password"
          hint={`at least ${MIN_PASSWORD} characters`}
          htmlFor="invite-password"
        >
          <TextInput
            id="invite-password"
            name="password"
            type="password"
            autoComplete="new-password"
            minLength={MIN_PASSWORD}
            required
          />
        </Field>

        <Field label="Confirm password" htmlFor="invite-confirm">
          <TextInput
            id="invite-confirm"
            name="confirm"
            type="password"
            autoComplete="new-password"
            minLength={MIN_PASSWORD}
            required
          />
        </Field>

        {mismatch && (
          <p role="alert" className="text-sm text-red-700">
            The two passwords don&rsquo;t match.
          </p>
        )}

        <Button type="submit" disabled={busy}>
          {busy ? "Creating account." : "Create account"}
        </Button>
      </form>
    </Card>
  );
}

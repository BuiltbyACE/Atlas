"use client";

import { useEffect, useState } from "react";

import { Button, Field, Select, TextInput, TextArea } from "@/components/form";
import { Icon } from "@/components/icons";
import { Card, PageHeader, ProblemNotice } from "@/components/ui";
import {
  composeEmail,
  listEmailAddresses,
  type ComposeEmailBody,
  type EmailAddress,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { useRouter } from "next/navigation";

/**
 * Compose email (§8.4). Write and send a message from a verified
 * send-as address. Returns to the inbox on success.
 */
export default function ComposePage() {
  const router = useRouter();
  const user = requireUser();

  const [addresses, setAddresses] = useState<EmailAddress[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);

  const [fromAddress, setFromAddress] = useState("");
  const [toAddresses, setToAddresses] = useState("");
  const [subject, setSubject] = useState("");
  const [bodyText, setBodyText] = useState("");

  useEffect(() => {
    let cancelled = false;
    listEmailAddresses()
      .then((items) => {
        if (!cancelled) setAddresses(items);
      })
      .catch(() => {
        if (!cancelled) setLoading(false);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Compose" description="Write a new email." />
        <div className="p-[22px] text-[12.5px] text-muted">Loading…</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader title="Compose" description="Write a new email." />
        <ProblemNotice
          error={error}
          title="Your send-as addresses could not be loaded"
        />
      </div>
    );
  }

  const fromOptions = addresses.map((a) => ({
    value: a.address,
    label: `${a.address}` + (a.is_primary ? " (primary)" : ""),
  }));

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);

    const body: ComposeEmailBody = {
      subject,
      body_text: bodyText,
      from_address: fromAddress,
      to_addresses: toAddresses
        .split(",")
        .map((a) => a.trim())
        .filter((a) => a.length > 0),
    };

    try {
      await composeEmail(body);
      router.push("/email/inbox");
    } catch (caught) {
      if (caught instanceof ApiError) setError(caught);
      else
        setError(
          new ApiError(500, {
            title: "Unexpected error",
            detail: "An unexpected error occurred",
          }),
        );
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title="Compose" description="Write a new email." />

      <form onSubmit={onSubmit} className="space-y-4">
        {error && (
          <ProblemNotice error={error} title="Your message couldn't be sent" />
        )}

        <Field label="From" htmlFor="compose-from">
          <Select
            id="compose-from"
            name="from_address"
            value={fromAddress}
            onChange={(e) => setFromAddress(e.target.value)}
            required
          >
            <option value="">Choose a send-as address</option>
            {fromOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="To" htmlFor="compose-to">
          <TextInput
            id="compose-to"
            name="to_addresses"
            value={toAddresses}
            onChange={(e) => setToAddresses(e.target.value)}
            required
            placeholder="name@company.com, name2@company.com"
          />
        </Field>

        <Field label="Subject" htmlFor="compose-subject">
          <TextInput
            id="compose-subject"
            name="subject"
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            required
          />
        </Field>

        <Field label="Message" htmlFor="compose-body">
          <TextArea
            id="compose-body"
            name="body"
            value={bodyText}
            onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) =>
              setBodyText(e.target.value)
            }
            required
            rows={12}
          />
        </Field>

        <div className="flex justify-end gap-2 pt-4">
          <Button
            type="button"
            variant="secondary"
            onClick={() => router.push("/email/inbox")}
            disabled={busy}
          >
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Sending…" : "Send"}
          </Button>
        </div>
      </form>
    </div>
  );
}

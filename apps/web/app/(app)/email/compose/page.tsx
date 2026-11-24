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
  // loadError: the address list failed. error: sending failed. They are kept apart
  // so a failed send keeps the form (and the user's text) on screen.
  const [loadError, setLoadError] = useState<ApiError | null>(null);
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
        if (cancelled) return;
        // Only verified addresses may send; default to the primary one.
        const usable = items.filter((a) => a.verified);
        setAddresses(usable);
        const preferred = usable.find((a) => a.is_primary) ?? usable[0];
        if (preferred) setFromAddress(preferred.address);
      })
      .catch((caught) => {
        // Never swallow this: an empty From list with no explanation is what
        // made compose look broken.
        if (cancelled) return;
        setLoadError(
          caught instanceof ApiError
            ? caught
            : new ApiError(500, {
                title: "Unexpected error",
                detail: "Your send-as addresses could not be loaded.",
              }),
        );
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

  if (loadError) {
    return (
      <div className="space-y-6">
        <PageHeader title="Compose" description="Write a new email." />
        <ProblemNotice
          error={loadError}
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

        {addresses.length === 0 && (
          <div
            role="status"
            className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-[12.5px] text-amber-900"
          >
            You have no verified send-as address yet, so you cannot send mail.
            Your sign-in email becomes available automatically; any other
            address must be verified by an administrator.
          </div>
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
          <Button
            type="submit"
            variant="primary"
            disabled={busy || addresses.length === 0}
          >
            {busy ? "Sending…" : "Send"}
          </Button>
        </div>
      </form>
    </div>
  );
}

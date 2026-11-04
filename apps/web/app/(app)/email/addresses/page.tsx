"use client";

import { useEffect, useState } from "react";

import { Button, ButtonLink, Field, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { Card, EmptyState, PageHeader, ProblemNotice, Table, Td, Th } from "@/components/ui";
import {
  deleteEmailAddress,
  listEmailAddresses,
  registerEmailAddress,
  setPrimaryEmailAddress,
  type EmailAddress,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";

/**
 * Email addresses (§8.4). Manage your verified “send as” addresses.
 * Each address can be used as the From when composing emails.
 */
export default function AddressesPage() {
  const [addresses, setAddresses] = useState<EmailAddress[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);
  const [newAddress, setNewAddress] = useState("");
  const [isPrimary, setIsPrimary] = useState(false);

  useEffect(() => {
    let cancelled = false;
    listEmailAddresses()
      .then((items) => {
        if (!cancelled) setAddresses(items);
      })
      .catch((caught) => {
        if (!cancelled) {
          if (caught instanceof ApiError) setError(caught);
          else setError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
        }
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
        <PageHeader title="Email addresses" description="Manage your send-as email addresses." />
        <div className="p-[22px] text-[12.5px] text-muted">Loading…</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader title="Email addresses" description="Manage your send-as email addresses." />
        <ProblemNotice error={error} title="Your email addresses could not be loaded" />
      </div>
    );
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);

    const trimmed = newAddress.trim();
    if (!trimmed) {
      setBusy(false);
      return;
    }

    try {
      await registerEmailAddress({
        address: trimmed,
        is_primary: isPrimary,
      });
      setNewAddress("");
      setIsPrimary(false);
      const updated = await listEmailAddresses();
      setAddresses(updated);
    } catch (caught) {
      if (caught instanceof ApiError) setError(caught);
      else setError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
    } finally {
      setBusy(false);
    }
  }

  async function onSetPrimary(id: string) {
    setBusy(true);
    setError(null);
    try {
      await setPrimaryEmailAddress(id);
      const updated = await listEmailAddresses();
      setAddresses(updated);
    } catch (caught) {
      if (caught instanceof ApiError) setError(caught);
      else setError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
    } finally {
      setBusy(false);
    }
  }

  async function onDelete(id: string) {
    if (!confirm("Remove this email address?")) return;
    setBusy(true);
    setError(null);
    try {
      await deleteEmailAddress(id);
      setAddresses(addresses.filter((a) => a.id !== id));
    } catch (caught) {
      if (caught instanceof ApiError) setError(caught);
      else setError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Email addresses"
        description="Manage your verified “send as” addresses. Each address can be used as the From when composing emails."
        actions={
          <ButtonLink href="/email/compose" variant="secondary">
            <Icon name="chevronLeft" className="h-4 w-4" />
            Compose
          </ButtonLink>
        }
      />

      <div className="space-y-6">
        <Card>
          <div className="px-[22px] py-[22px]">
            <h3 className="mb-[16px] text-[15px] font-semibold text-night">Add address</h3>
            <form onSubmit={onSubmit} className="flex flex-col gap-4 sm:flex-row sm:items-end">
              <Field label="Email address" htmlFor="add-address">
                <TextInput
                  id="add-address"
                  name="address"
                  value={newAddress}
                  onChange={(e) => setNewAddress(e.target.value)}
                  required
                  placeholder="you@company.com"
                />
              </Field>
              <Field label="Make primary" htmlFor="add-primary">
                <input
                  id="add-primary"
                  name="is_primary"
                  type="checkbox"
                  checked={isPrimary}
                  onChange={(e) => setIsPrimary(e.target.checked)}
                />
              </Field>
              <Button type="submit" variant="primary" disabled={busy || !newAddress.trim()}>
                {busy ? "Adding…" : "Add address"}
              </Button>
            </form>
          </div>
        </Card>

        <Card>
          <div className="px-[22px] pb-[22px]">
            <div className="flex flex-col gap-4">
              <h3 className="mb-[16px] text-[15px] font-semibold text-night">Your addresses</h3>
              {addresses.length === 0 ? (
                <div className="px-[22px] text-[12.5px] text-muted">
                  No addresses yet. Add one above to send emails from your own address.
                </div>
              ) : (
                <Table>
                  <thead>
                    <tr>
                      <Th>Address</Th>
                      <Th className="text-right">Verified</Th>
                      <Th className="text-right">Primary</Th>
                      <Th className="text-right">Action</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {addresses.map((addr) => (
                      <tr key={addr.id}>
                        <Td className="font-medium text-night">{addr.address}</Td>
                        <Td className="tabular text-right text-muted">{addr.verified ? "Yes" : "No"}</Td>
                        <Td className="tabular text-right text-muted">
                          {addr.is_primary ? (
                            "Primary"
                          ) : (
                            <button onClick={() => onSetPrimary(addr.id)}>Set primary</button>
                          )}
                        </Td>
                        <Td className="tabular text-right text-muted">
                          <button
                            onClick={() => onDelete(addr.id)}
                            className="text-[12px] text-danger hover:underline"
                          >
                            Remove
                          </button>
                        </Td>
                      </tr>
                    ))}
                  </tbody>
                </Table>
              )}
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
}

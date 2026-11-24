"use client";

import { useEffect, useState } from "react";

import { Button, ButtonLink, Field, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { Card, EmptyState, PageHeader, ProblemNotice, Table, Td, Th } from "@/components/ui";
import {
  assignEmailAddress,
  deleteEmailAddress,
  listEmailAddresses,
  registerEmailAddress,
  setPrimaryEmailAddress,
  type EmailAddress,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { getCurrentUser } from "@/lib/auth/session";

/**
 * Email addresses (§8.4). Manage your verified “send as” addresses.
 * Each address can be used as the From when composing emails.
 */
export default function AddressesPage() {
  const [addresses, setAddresses] = useState<EmailAddress[]>([]);
  const [loading, setLoading] = useState(true);
  // loadError replaces the page (nothing to show); actionError is shown beside the list.
  const [loadError, setLoadError] = useState<ApiError | null>(null);
  const [actionError, setActionError] = useState<ApiError | null>(null);
  const [loginEmail, setLoginEmail] = useState("");
  const [canAssign, setCanAssign] = useState(false);
  const [assignUser, setAssignUser] = useState("");
  const [assignAddress, setAssignAddress] = useState("");
  const [assignPrimary, setAssignPrimary] = useState(true);
  const [assigned, setAssigned] = useState("");
  const [busy, setBusy] = useState(false);
  const [newAddress, setNewAddress] = useState("");
  const [isPrimary, setIsPrimary] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getCurrentUser().then((me) => {
      if (cancelled || !me) return;
      setLoginEmail(me.email.toLowerCase());
      setCanAssign(me.permissions.includes("core.settings.manage"));
    });
    listEmailAddresses()
      .then((items) => {
        if (!cancelled) setAddresses(items);
      })
      .catch((caught) => {
        if (!cancelled) {
          if (caught instanceof ApiError) setLoadError(caught);
          else setLoadError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
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

  if (loadError) {
    return (
      <div className="space-y-6">
        <PageHeader title="Email addresses" description="Manage your send-as email addresses." />
        <ProblemNotice error={loadError} title="Your email addresses could not be loaded" />
      </div>
    );
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setActionError(null);

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
      if (caught instanceof ApiError) setActionError(caught);
      else setActionError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
    } finally {
      setBusy(false);
    }
  }

  async function onAssign(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setActionError(null);
    setAssigned("");
    try {
      const result = await assignEmailAddress({
        user_email: assignUser.trim(),
        address: assignAddress.trim(),
        make_primary: assignPrimary,
      });
      setAssigned(`${result.address} is now verified for ${assignUser.trim()}.`);
      setAssignAddress("");
      setAddresses(await listEmailAddresses());
    } catch (caught) {
      if (caught instanceof ApiError) setActionError(caught);
      else setActionError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
    } finally {
      setBusy(false);
    }
  }

  async function onSetPrimary(id: string) {
    setBusy(true);
    setActionError(null);
    try {
      await setPrimaryEmailAddress(id);
      const updated = await listEmailAddresses();
      setAddresses(updated);
    } catch (caught) {
      if (caught instanceof ApiError) setActionError(caught);
      else setActionError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
    } finally {
      setBusy(false);
    }
  }

  async function onDelete(id: string) {
    if (!confirm("Remove this email address?")) return;
    setBusy(true);
    setActionError(null);
    try {
      await deleteEmailAddress(id);
      setAddresses(addresses.filter((a) => a.id !== id));
    } catch (caught) {
      if (caught instanceof ApiError) setActionError(caught);
      else setActionError(new ApiError(500, { title: "Unexpected error", detail: "An unexpected error occurred" }));
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
        {actionError ? <ProblemNotice error={actionError} title="That did not work" /> : null}
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

        {canAssign ? (
          <Card>
            <div className="px-[22px] py-[22px]">
              <h3 className="mb-[4px] text-[15px] font-semibold text-night">Give someone an address</h3>
              <p className="mb-[16px] text-[12.5px] text-muted">
                Administrators only. The address becomes verified for that person, so they can send from it and
                see mail sent to it. Use an address on a domain verified in Resend.
              </p>
              <form onSubmit={onAssign} className="flex flex-col gap-4 sm:flex-row sm:items-end">
                <Field label="Their sign-in email" htmlFor="assign-user">
                  <TextInput
                    id="assign-user"
                    value={assignUser}
                    onChange={(e) => setAssignUser(e.target.value)}
                    required
                    placeholder="person@company.com"
                  />
                </Field>
                <Field label="Address to give" htmlFor="assign-address">
                  <TextInput
                    id="assign-address"
                    value={assignAddress}
                    onChange={(e) => setAssignAddress(e.target.value)}
                    required
                    placeholder="accounts@company.com"
                  />
                </Field>
                <Field label="Make primary" htmlFor="assign-primary">
                  <input
                    id="assign-primary"
                    type="checkbox"
                    checked={assignPrimary}
                    onChange={(e) => setAssignPrimary(e.target.checked)}
                  />
                </Field>
                <Button type="submit" variant="primary" disabled={busy || !assignUser.trim() || !assignAddress.trim()}>
                  {busy ? "Saving…" : "Give address"}
                </Button>
              </form>
              {assigned ? <p className="mt-3 text-[12.5px] text-night">{assigned}</p> : null}
            </div>
          </Card>
        ) : null}
        <Card>
          <div className="px-[22px] pb-[22px]">
            <div className="flex flex-col gap-4">
              <h3 className="mb-[16px] text-[15px] font-semibold text-night">Your addresses</h3>
              {addresses?.length === 0 ? (
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
                    {addresses?.map((addr) => (
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
                          {addr.address.toLowerCase() === loginEmail ? (
                            <span title="The address you sign in with cannot be removed">Sign-in address</span>
                          ) : (
                            <button
                              onClick={() => onDelete(addr.id)}
                              className="text-[12px] text-danger hover:underline"
                            >
                              Remove
                            </button>
                          )}
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

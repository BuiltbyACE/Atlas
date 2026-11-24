"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Button, Field, Select, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import { createInvite, listRoles, type Role } from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/**
 * Invite a new person to join Atlas (`POST /api/v1/auth/invites`,
 * gated by `core.user.manage`). Only name and email are required.
 *
 * Role grants are chosen from the live role catalogue (`GET /api/v1/roles`)
 * rather than typed as keys, so an administrator never has to remember the
 * role names. Each grant is company-wide: permissions are not scope-filtered
 * yet, so a narrower scope would change nothing.
 */
export default function NewPersonPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run } = useMutation();

  const [roles, setRoles] = useState<Role[]>([]);
  const [rolesLoading, setRolesLoading] = useState(true);
  const [rolesError, setRolesError] = useState<string | null>(null);
  const [selected, setSelected] = useState<string[]>([]);

  useEffect(() => {
    let cancelled = false;
    listRoles()
      .then((response) => {
        if (!cancelled) setRoles(response.roles);
      })
      .catch(() => {
        if (!cancelled) {
          setRolesError(
            "The role catalogue could not be loaded. You can still send the invite without a role grant.",
          );
        }
      })
      .finally(() => {
        if (!cancelled) setRolesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function toggleRole(key: string) {
    setSelected((current) =>
      current.includes(key)
        ? current.filter((roleKey) => roleKey !== key)
        : [...current, key],
    );
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);

    const created = await run(async () => {
      return createInvite({
        email: String(data.get("email") ?? "").trim(),
        full_name: String(data.get("full_name") ?? "").trim(),
        employment_type:
          String(data.get("employment_type") ?? "").trim() || undefined,
        role_grants:
          selected.length > 0
            ? selected.map((roleKey) => ({
                role_key: roleKey,
                scope_type: "company" as const,
              }))
            : undefined,
        ttl_hours:
          Number(String(data.get("ttl_hours") ?? "").trim() || "72") ||
          undefined,
      });
    });

    if (created) {
      notify(`Invited ${created.email}`);
      router.push("/people");
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
      {problem && (
        <ProblemNotice error={problem} title="The invitation couldn't be sent" />
      )}

      <Field label="Email" htmlFor="invite-email">
        <TextInput
          id="invite-email"
          name="email"
          required
          data-autofocus
          placeholder="name@company.com"
        />
      </Field>

      <Field label="Full name" htmlFor="invite-full-name">
        <TextInput
          id="invite-full-name"
          name="full_name"
          required
          placeholder="Jane Wanjiku"
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 md:grid-cols-2">
        <Field label="Employment type" htmlFor="invite-employment-type">
          <Select id="invite-employment-type" name="employment_type">
            <option value="">Not specified</option>
            <option value="full_time">Full-time</option>
            <option value="part_time">Part-time</option>
            <option value="contractor">Contractor</option>
            <option value="intern">Intern</option>
            <option value="external">External</option>
          </Select>
        </Field>
        <Field label="Expires in (hours)" htmlFor="invite-ttl-hours">
          <TextInput
            id="invite-ttl-hours"
            name="ttl_hours"
            type="number"
            defaultValue="72"
          />
          <p className="mt-1 text-[11.5px] text-muted">
            Invitation link expiry in hours.
          </p>
        </Field>
      </div>

      <div className="flex flex-col gap-1.5 text-sm">
        <p id="invite-roles-label" className="font-semibold text-night">
          Role grants
          <span className="ml-1.5 font-normal text-muted">
            {selected.length > 0
              ? `${selected.length} selected`
              : "optional"}
          </span>
        </p>
        <div
          role="group"
          aria-labelledby="invite-roles-label"
          className="max-h-64 overflow-y-auto rounded-control border border-line-strong bg-white p-1.5"
        >
          {rolesLoading && (
            <p className="px-2 py-3 text-[12.5px] text-muted">Loading roles…</p>
          )}
          {!rolesLoading && rolesError && (
            <p className="px-2 py-3 text-[12.5px] text-[#B42318]">
              {rolesError}
            </p>
          )}
          {!rolesLoading && !rolesError && roles.length === 0 && (
            <p className="px-2 py-3 text-[12.5px] text-muted">
              No roles are configured.
            </p>
          )}
          {roles.map((role) => (
            <label
              key={role.key}
              className="flex cursor-pointer items-start gap-2.5 rounded-control px-2 py-1.5 hover:bg-[#F7F9FC]"
            >
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4 shrink-0 accent-atlas"
                checked={selected.includes(role.key)}
                onChange={() => toggleRole(role.key)}
              />
              <span className="min-w-0">
                <span className="block text-[13.5px] font-medium text-night">
                  {role.name}
                </span>
                <span className="block text-[11.5px] text-muted">
                  <span className="font-mono">{role.key}</span>
                  {role.description ? ` — ${role.description}` : ""}
                </span>
              </span>
            </label>
          ))}
        </div>
        <p className="text-[11.5px] text-muted">
          Each grant applies company-wide. Permissions come from the role.
        </p>
      </div>

      <div className="flex justify-end gap-2 pt-4">
        <Button
          type="button"
          variant="secondary"
          onClick={() => router.push("/people")}
        >
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? "Inviting…" : "Invite person"}
        </Button>
      </div>
    </form>
  );
}

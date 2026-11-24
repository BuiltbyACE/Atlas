"use client";

import { useState } from "react";

import { Button, Field, TextInput } from "@/components/form";
import { Card, ProblemNotice } from "@/components/ui";
import {
  updateOrganization,
  type CrmOrganization,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/** Inline edit of an organization's registry fields (§8.1). */
export function OrganizationEditor({
  organization,
}: {
  organization: CrmOrganization;
}) {
  const { busy, problem, run, reset } = useMutation();
  const [name, setName] = useState(organization.name);
  const [industry, setIndustry] = useState(organization.industry ?? "");
  const [size, setSize] = useState(organization.size ?? "");
  const [location, setLocation] = useState(organization.location ?? "");
  const [kraPin, setKraPin] = useState(organization.kra_pin ?? "");

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return updateOrganization(organization.id, organization.version, {
        name,
        industry: industry || undefined,
        size: size || undefined,
        location: location || undefined,
        kra_pin: kraPin || undefined,
      });
    });
    if (updated) {
      reset();
    }
  }

  return (
    <Card title="Manage organization">
      {problem && (
        <ProblemNotice
          error={problem}
          title="The organization wasn't updated"
        />
      )}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Name">
          <TextInput
            name="name"
            defaultValue={organization.name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label="Industry">
            <TextInput
              name="industry"
              defaultValue={organization.industry ?? ""}
              onChange={(e) => setIndustry(e.target.value)}
              placeholder="e.g. Agriculture"
            />
          </Field>
          <Field label="Size">
            <TextInput
              name="size"
              defaultValue={organization.size ?? ""}
              onChange={(e) => setSize(e.target.value)}
              placeholder="e.g. 50-200 employees"
            />
          </Field>
        </div>
        <Field label="Location">
          <TextInput
            name="location"
            defaultValue={organization.location ?? ""}
            onChange={(e) => setLocation(e.target.value)}
            placeholder="e.g. Nairobi, Kenya"
          />
        </Field>
        <Field label="KRA PIN">
          <TextInput
            name="kra_pin"
            defaultValue={organization.kra_pin ?? ""}
            onChange={(e) => setKraPin(e.target.value)}
            placeholder="e.g. P051234567A"
          />
        </Field>
        <div className="flex items-end gap-2 pt-4">
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Saving…" : "Save"}
          </Button>
        </div>
      </form>
    </Card>
  );
}

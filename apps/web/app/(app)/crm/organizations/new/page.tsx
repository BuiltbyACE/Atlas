"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createOrganization,
  listOrganizations,
  type CrmOrganization,
} from "@/lib/api/atlas";
import { personDirectory } from "@/lib/directory";
import { useMutation } from "@/lib/use-mutation";

/**
 * New organization (`POST /api/v1/crm/organizations`, `crm.registry.manage`).
 * Opens in a drawer over the CRM page. Name is the only required field;
 * everything else is optional context for the commercial team.
 */
export default function NewOrganizationPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(true);
  const [people, setPeople] = useState<Map<string, string>>(new Map());

  useEffect(() => {
    personDirectory().then(
      (directory) => setPeople(directory),
      () => setPeople(new Map()),
    );
  }, []);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(async () => {
      return createOrganization({
        name: String(data.get("name") ?? "").trim(),
        industry: String(data.get("industry") ?? "").trim() || undefined,
        size: String(data.get("size") ?? "").trim() || undefined,
        location: String(data.get("location") ?? "").trim() || undefined,
        kra_pin: String(data.get("kra_pin") ?? "").trim() || undefined,
        status: (String(data.get("status") ?? "prospect") as
          | "prospect"
          | "client"
          | "former") || "prospect",
        owner_person_id: String(data.get("owner_person_id") ?? "").trim() ||
          undefined,
      });
    });
    if (created) {
      setOpen(false);
      reset();
      notify(`Recorded ${created.name}`);
      router.refresh();
    }
  }

  function close() {
    setOpen(false);
  }

  return (
    <>
      <Drawer
        open={open}
        onClose={close}
        title="New organization"
        description="A client or prospect company. Only the name is required; the rest can be filled in later."
      >
        {problem && (
          <ProblemNotice error={problem} title="The organization wasn't created" />
        )}
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          <Field label="Name" htmlFor="org-name">
            <TextInput
              id="org-name"
              name="name"
              required
              data-autofocus
              placeholder="Acme Limited"
            />
          </Field>

          <Field label="Industry" htmlFor="org-industry">
            <TextInput id="org-industry" name="industry" placeholder="e.g. Agriculture" />
          </Field>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Size" htmlFor="org-size">
              <TextInput id="org-size" name="size" placeholder="e.g. 200 staff" />
            </Field>
            <Field label="Location" htmlFor="org-location">
              <TextInput id="org-location" name="location" placeholder="e.g. Nairobi" />
            </Field>
          </div>

          <Field label="KRA PIN" htmlFor="org-kra-pin">
            <TextInput id="org-kra-pin" name="kra_pin" placeholder="e.g. P051234567A" />
          </Field>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Status" htmlFor="org-status">
              <Select id="org-status" name="status" defaultValue="prospect">
                <option value="prospect">Prospect</option>
                <option value="client">Client</option>
                <option value="former">Former</option>
              </Select>
            </Field>
            <Field label="Owner" htmlFor="org-owner">
              <Select id="org-owner" name="owner_person_id">
                <option value="">Unassigned</option>
                {Array.from(people.entries()).map(([id, name]) => (
                  <option key={id} value={id}>
                    {name}
                  </option>
                ))}
              </Select>
            </Field>
          </div>

          <div className="flex justify-end gap-2 pt-4">
            <Button type="button" variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Create organization"}
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}

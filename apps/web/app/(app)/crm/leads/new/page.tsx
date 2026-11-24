"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createLead,
  listLeadSources,
  listOrganizations,
  type CrmOrganization,
  type Lead,
  type LeadSource,
} from "@/lib/api/atlas";
import { personDirectory } from "@/lib/directory";
import { useMutation } from "@/lib/use-mutation";

/**
 * New lead (`POST /api/v1/crm/leads`, `crm.lead.create`). Opens in a drawer
 * over the CRM page. Name/need and a source are required; an open lead also
 * needs an owner and a dated next action, enforced by Go.
 */
export default function NewLeadPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(true);
  const [sources, setSources] = useState<LeadSource[]>([]);
  const [orgs, setOrgs] = useState<CrmOrganization[]>([]);
  const [people, setPeople] = useState<Map<string, string>>(new Map());

  useEffect(() => {
    listLeadSources().then(
      (list) => setSources(list),
      () => setSources([]),
    );
    listOrganizations({ limit: 200 }).then(
      (page) => setOrgs(page.items),
      () => setOrgs([]),
    );
    personDirectory().then(
      (directory) => setPeople(directory),
      () => setPeople(new Map()),
    );
  }, []);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const status = String(data.get("status") ?? "new") as
      | "new"
      | "assigned"
      | "contacted"
      | "qualified"
      | "nurture"
      | "disqualified"
      | "converted";
    const isOpen =
      status === "assigned" ||
      status === "contacted" ||
      status === "qualified" ||
      status === "nurture";
    const created = await run(async () => {
      return createLead({
        source_key: String(data.get("source_key") ?? "").trim() || "website_form",
        source_detail: String(data.get("source_detail") ?? "").trim() || undefined,
        organization_id:
          String(data.get("organization_id") ?? "").trim() || undefined,
        contact_id:
          String(data.get("contact_id") ?? "").trim() || undefined,
        contact_name: String(data.get("contact_name") ?? "").trim() || undefined,
        contact_phone: String(data.get("contact_phone") ?? "").trim() || undefined,
        contact_email: String(data.get("contact_email") ?? "").trim() || undefined,
        need_summary: String(data.get("need_summary") ?? "").trim() || undefined,
        estimated_value_minor: Number(
          String(data.get("estimated_value_minor") ?? "").trim() || "0",
        ) || undefined,
        currency: String(data.get("currency") ?? "KES").trim() || "KES",
        priority: (String(data.get("priority") ?? "normal") as
          | "low"
          | "normal"
          | "high"
          | "urgent") || "normal",
        owner_person_id:
          String(data.get("owner_person_id") ?? "").trim() || undefined,
        status,
        next_action: String(data.get("next_action") ?? "").trim() || undefined,
        next_action_due_at:
          String(data.get("next_action_due_at") ?? "").trim() || undefined,
        qualification_notes:
          String(data.get("qualification_notes") ?? "").trim() || undefined,
        lost_reason:
          status === "disqualified"
            ? (String(data.get("lost_reason") ?? "").trim() || undefined)
            : undefined,
      });
    });
    if (created) {
      setOpen(false);
      reset();
      notify(`Recorded ${created.ref}`);
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
        title="New lead"
        description="An enquiry on its way to qualified. A source and need are required; an open lead also needs an owner and a dated next action."
      >
        {problem && (
          <ProblemNotice error={problem} title="The lead wasn't created" />
        )}
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Source" htmlFor="lead-source">
              <Select id="lead-source" name="source_key" required>
                <option value="">Choose a source</option>
                {sources.map((s) => (
                  <option key={s.key} value={s.key}>
                    {s.label}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Source detail" htmlFor="lead-source-detail">
              <TextInput
                id="lead-source-detail"
                name="source_detail"
                placeholder="e.g. Website form - pricing page"
              />
            </Field>
          </div>

          <Field label="Need summary" htmlFor="lead-need">
            <TextInput
              id="lead-need"
              name="need_summary"
              required
              data-autofocus
              placeholder="What does the prospect need?"
            />
          </Field>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Organization" htmlFor="lead-org">
              <Select id="lead-org" name="organization_id">
                <option value="">Unassigned</option>
                {orgs.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.name}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Contact" htmlFor="lead-contact">
              <TextInput
                id="lead-contact"
                name="contact_name"
                placeholder="e.g. Jane Wanjiku"
              />
              <p className="mt-1 text-[11.5px] text-muted">
                Contact name (create the contact record separately for full details).
              </p>
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
            <Field label="Contact phone" htmlFor="lead-contact-phone">
              <TextInput
                id="lead-contact-phone"
                name="contact_phone"
                placeholder="+254 700 000 000"
              />
            </Field>
            <Field label="Contact email" htmlFor="lead-contact-email">
              <TextInput
                id="lead-contact-email"
                name="contact_email"
                placeholder="jane@acme.com"
              />
            </Field>
            <Field label="Estimated value" htmlFor="lead-value">
              <TextInput
                id="lead-value"
                name="estimated_value_minor"
                inputMode="decimal"
                placeholder="e.g. 500000"
              />
              <p className="mt-1 text-[11.5px] text-muted">Minor units (cents)</p>
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
            <Field label="Currency" htmlFor="lead-currency">
              <Select id="lead-currency" name="currency" defaultValue="KES">
                <option value="KES">KES</option>
                <option value="USD">USD</option>
                <option value="EUR">EUR</option>
                <option value="GBP">GBP</option>
              </Select>
            </Field>
            <Field label="Priority" htmlFor="lead-priority">
              <Select id="lead-priority" name="priority" defaultValue="normal">
                <option value="low">Low</option>
                <option value="normal">Normal</option>
                <option value="high">High</option>
                <option value="urgent">Urgent</option>
              </Select>
            </Field>
            <Field label="Status" htmlFor="lead-status">
              <Select id="lead-status" name="status" defaultValue="new">
                <option value="new">New</option>
                <option value="assigned">Assigned</option>
                <option value="contacted">Contacted</option>
                <option value="qualified">Qualified</option>
                <option value="nurture">Nurture</option>
                <option value="disqualified">Disqualified</option>
                <option value="converted">Converted</option>
              </Select>
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Owner" htmlFor="lead-owner">
              <Select id="lead-owner" name="owner_person_id">
                <option value="">Unassigned</option>
                {Array.from(people.entries()).map(([id, name]) => (
                  <option key={id} value={id}>
                    {name}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Next action" htmlFor="lead-next-action">
              <TextInput
                id="lead-next-action"
                name="next_action"
                placeholder="e.g. Send proposal"
              />
            </Field>
          </div>

          <Field label="Next action due" htmlFor="lead-next-action-due">
            <TextInput
              id="lead-next-action-due"
              name="next_action_due_at"
              type="datetime-local"
            />
          </Field>

          <Field label="Qualification notes" htmlFor="lead-notes">
            <TextInput
              id="lead-notes"
              name="qualification_notes"
              placeholder="Notes from qualification"
            />
          </Field>
          <div className="flex justify-end gap-2 pt-4">
            <Button type="button" variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Create lead"}
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}

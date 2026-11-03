"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createContract,
  listOrganizations,
  type CrmOrganization,
  type Contract,
} from "@/lib/api/atlas";
import { personDirectory } from "@/lib/directory";
import { useMutation } from "@/lib/use-mutation";

/**
 * New contract (`POST /api/v1/contracts`, `contracts.contract.manage`).
 * Opens in a drawer over the contracts page. Organization and type are
 * required; everything else drafts the agreement for later completion.
 */
export default function NewContractPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(true);
  const [orgs, setOrgs] = useState<CrmOrganization[]>([]);
  const [people, setPeople] = useState<Map<string, string>>(new Map());

  useEffect(() => {
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
    const created = await run(async () => {
      return createContract({
        organization_id: String(data.get("organization_id") ?? "").trim(),
        opportunity_id:
          String(data.get("opportunity_id") ?? "").trim() || undefined,
        contract_type: String(data.get("contract_type") ?? "fixed_price_project"),
        value_minor: Number(
          String(data.get("value_minor") ?? "").trim() || "0",
        ) || undefined,
        currency: String(data.get("currency") ?? "KES").trim() || "KES",
        start_at:
          String(data.get("start_at") ?? "").trim() || undefined,
        end_at: String(data.get("end_at") ?? "").trim() || undefined,
        payment_terms:
          String(data.get("payment_terms") ?? "").trim() || undefined,
        renewal_terms:
          String(data.get("renewal_terms") ?? "").trim() || undefined,
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
        title="New contract"
        description="A client agreement drafted from an accepted proposal. Organization and type are required; the rest can be completed later."
      >
        {problem && (
          <ProblemNotice error={problem} title="The contract wasn't created" />
        )}
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          <Field label="Organization" htmlFor="contract-org">
            <Select id="contract-org" name="organization_id" required>
              <option value="">Choose an organization</option>
              {orgs.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </Select>
          </Field>

          <Field label="Opportunity" htmlFor="contract-opportunity">
            <TextInput
              id="contract-opportunity"
              name="opportunity_id"
              placeholder="Opportunity ref or ID (optional)"
            />
          </Field>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Contract type" htmlFor="contract-type">
              <Select id="contract-type" name="contract_type" defaultValue="fixed_price_project">
                <option value="fixed_price_project">Fixed-price project</option>
                <option value="retainer">Retainer</option>
                <option value="support_sla">Support SLA</option>
                <option value="licence">Licence</option>
                <option value="hosting">Hosting</option>
              </Select>
            </Field>
            <Field label="Currency" htmlFor="contract-currency">
              <Select id="contract-currency" name="currency" defaultValue="KES">
                <option value="KES">KES</option>
                <option value="USD">USD</option>
                <option value="EUR">EUR</option>
              </Select>
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="Value (minor units)" htmlFor="contract-value">
              <TextInput
                id="contract-value"
                name="value_minor"
                inputMode="decimal"
                placeholder="e.g. 50000000"
              />
              <p className="mt-1 text-[11.5px] text-muted">
                Minor units (cents). Restricted to viewers without amount access.
              </p>
            </Field>
            <Field label="Start date" htmlFor="contract-start">
              <TextInput
                id="contract-start"
                name="start_at"
                type="date"
              />
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="End date" htmlFor="contract-end">
              <TextInput
                id="contract-end"
                name="end_at"
                type="date"
              />
            </Field>
            <Field label="Owner" htmlFor="contract-owner">
              <Select id="contract-owner" name="owner_person_id">
                <option value="">Unassigned</option>
                {Array.from(people.entries()).map(([id, name]) => (
                  <option key={id} value={id}>
                    {name}
                  </option>
                ))}
              </Select>
            </Field>
          </div>

          <Field label="Payment terms" htmlFor="contract-payment-terms">
            <TextInput
              id="contract-payment-terms"
              name="payment_terms"
              placeholder="e.g. Net 30"
            />
          </Field>

          <Field label="Renewal terms" htmlFor="contract-renewal-terms">
            <TextInput
              id="contract-renewal-terms"
              name="renewal_terms"
              placeholder="e.g. Auto-renew annually"
            />
          </Field>

          <div className="flex justify-end gap-2 pt-4">
            <Button type="button" variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Create contract"}
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}

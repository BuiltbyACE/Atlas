"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, Switch, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { PersonSelect } from "@/components/person-select";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createRecurringCost,
  RECURRING_CYCLES,
  updateRecurringCost,
  type FinanceRecurringCost,
  type FinanceRecurringCostBody,
} from "@/lib/api/atlas";
import type { PersonOption } from "@/lib/directory";
import { dateInput, formatMinorInput, humanise, parseMinor, today } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { FinanceCapability } from "./permissions";

/**
 * Recurring costs (`/api/v1/finance/recurring-costs`, §8.4). Hosting, domains
 * and subscriptions that renew on a cycle; the finance officer records them and
 * can retire one when it lapses. Deactivating is the same update with `active`
 * off, so the whole record is sent back and `If-Match` guards the change.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Read the currency the money field was entered in. */
function formCurrency(data: FormData, fallback: string): string {
  return field(data, "currency") || fallback;
}

/** Build the create/update body from the drawer's form. */
function recurringBody(data: FormData, fallbackCurrency: string): FinanceRecurringCostBody {
  const currency = formCurrency(data, fallbackCurrency);
  const body: FinanceRecurringCostBody = {
    vendor: field(data, "vendor"),
    amount_minor: parseMinor(field(data, "amount"), currency),
    currency,
    cycle: field(data, "cycle"),
    next_renewal_on: field(data, "next_renewal_on"),
    active: data.get("active") !== null,
  };
  const description = field(data, "description");
  if (description) body.description = description;
  const owner = field(data, "owner_person_id");
  if (owner) body.owner_person_id = owner;
  return body;
}

/**
 * The whole record as the update body. Used by the activate/deactivate toggle,
 * which is an update rather than its own route, so it must round-trip every
 * field Go stores. Only offered when the amount is visible (`amount_minor`
 * present), so a restricted viewer never sends a fabricated zero.
 */
function recurringBodyFrom(cost: FinanceRecurringCost, active: boolean): FinanceRecurringCostBody {
  const body: FinanceRecurringCostBody = {
    vendor: cost.vendor,
    amount_minor: cost.amount_minor ?? 0,
    currency: cost.currency,
    cycle: cost.cycle,
    next_renewal_on: dateInput(cost.next_renewal_on),
    active,
  };
  if (cost.description) body.description = cost.description;
  if (cost.owner_person_id) body.owner_person_id = cost.owner_person_id;
  return body;
}

/**
 * The fields shared by "new cost" and "edit cost". The amount hint follows the
 * currency field, so the figure is always read in the currency it is typed in.
 */
function RecurringCostFields({
  cost,
  people,
  me,
  defaultCurrency,
}: {
  cost?: FinanceRecurringCost;
  people: PersonOption[];
  me?: string;
  defaultCurrency: string;
}) {
  const [currency, setCurrency] = useState(cost?.currency || defaultCurrency);

  return (
    <>
      <Field label="Vendor" htmlFor="recurring-vendor">
        <TextInput
          id="recurring-vendor"
          name="vendor"
          required
          defaultValue={cost?.vendor ?? ""}
          placeholder="e.g. Safaricom, Cloudflare"
          data-autofocus
        />
      </Field>

      <Field label="Description" hint="optional" htmlFor="recurring-description">
        <TextArea
          id="recurring-description"
          name="description"
          rows={2}
          defaultValue={cost?.description ?? ""}
          placeholder="What the cost covers."
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Amount" hint={currency} htmlFor="recurring-amount">
          <TextInput
            id="recurring-amount"
            name="amount"
            required
            inputMode="decimal"
            placeholder="0.00"
            defaultValue={cost ? formatMinorInput(cost.amount_minor, cost.currency) : ""}
          />
        </Field>
        <Field label="Currency" hint="ISO code" htmlFor="recurring-currency">
          <TextInput
            id="recurring-currency"
            name="currency"
            value={currency}
            onChange={(event) => setCurrency(event.target.value.toUpperCase())}
          />
        </Field>
        <Field label="Cycle" htmlFor="recurring-cycle">
          <Select
            id="recurring-cycle"
            name="cycle"
            defaultValue={cost?.cycle ?? RECURRING_CYCLES[0]}
          >
            {RECURRING_CYCLES.map((cycle) => (
              <option key={cycle} value={cycle}>
                {humanise(cycle)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Next renewal" htmlFor="recurring-renewal">
          <TextInput
            id="recurring-renewal"
            name="next_renewal_on"
            type="date"
            required
            defaultValue={cost ? dateInput(cost.next_renewal_on) : today()}
          />
        </Field>
      </div>

      <Field label="Owner" hint="optional" htmlFor="recurring-owner">
        <PersonSelect
          id="recurring-owner"
          name="owner_person_id"
          people={people}
          me={me}
          defaultValue={cost?.owner_person_id ?? ""}
        />
      </Field>

      <Switch name="active" defaultChecked={cost?.active ?? true} label="Active" />
    </>
  );
}

/**
 * "New recurring cost" (`POST /api/v1/finance/recurring-costs`,
 * `finance.recurring_cost.create`): a button that opens the form in a drawer.
 */
export function NewRecurringCostButton({
  people,
  me,
  currency,
  capabilities,
  variant = "primary",
}: {
  people: PersonOption[];
  me?: string;
  currency: string;
  capabilities: FinanceCapability[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("finance.recurring_cost.create")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createRecurringCost(recurringBody(new FormData(event.currentTarget), currency)),
    );
    if (created) {
      setOpen(false);
      notify(`Recorded ${created.vendor}`);
      router.refresh();
    }
  }

  return (
    <>
      <Button variant={variant} size={variant === "secondary" ? "sm" : "md"} onClick={() => setOpen(true)}>
        <Icon name="plus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New recurring cost
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New recurring cost"
        description="A standing commitment. The renewal date drives the upcoming-renewals tile."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The cost wasn’t saved" />}

          <RecurringCostFields people={people} me={me} defaultCurrency={currency} />

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Add cost"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}

/**
 * The actions for one recurring-cost row: change the amount, cycle or renewal,
 * or retire/reactivate it. All of it is `PUT /api/v1/finance/recurring-costs/{id}`
 * under `finance.recurring_cost.update`, guarded by `If-Match`.
 */
export function RecurringCostActions({
  cost,
  people,
  me,
  currency,
  capabilities,
}: {
  cost: FinanceRecurringCost;
  people: PersonOption[];
  me?: string;
  currency: string;
  capabilities: FinanceCapability[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  const canManage = capabilities.includes("finance.recurring_cost.update");
  if (!canManage) return null;

  // The toggle rewrites the whole record, so it needs the amount on screen; a
  // viewer who cannot see it gets the edit drawer instead of a blind update.
  const canToggle = cost.amount_minor !== undefined;

  function close() {
    setOpen(false);
    reset();
  }

  async function onEdit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateRecurringCost(
        cost.id,
        recurringBody(new FormData(event.currentTarget), currency),
        cost.version,
      ),
    );
    if (updated) {
      close();
      notify(`Saved ${updated.vendor}`);
      router.refresh();
    }
  }

  async function toggleActive() {
    const done = await run(() =>
      updateRecurringCost(cost.id, recurringBodyFrom(cost, !cost.active), cost.version),
    );
    if (done) {
      notify(cost.active ? `${cost.vendor} deactivated` : `${cost.vendor} reactivated`);
      router.refresh();
    }
  }

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
          Edit
        </Button>
        {canToggle && (
          <Button size="sm" variant="secondary" disabled={busy} onClick={toggleActive}>
            {cost.active ? "Deactivate" : "Reactivate"}
          </Button>
        )}
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      <Drawer open={open} onClose={close} title="Edit recurring cost" description={cost.vendor}>
        <form onSubmit={onEdit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}

          <RecurringCostFields cost={cost} people={people} me={me} defaultCurrency={currency} />

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save cost"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </div>
  );
}

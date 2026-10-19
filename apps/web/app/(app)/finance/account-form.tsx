"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, Switch, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createAccount,
  FINANCE_ACCOUNT_TYPES,
  updateAccount,
  type FinanceAccount,
  type FinanceAccountBody,
} from "@/lib/api/atlas";
import { formatMinorInput, humanise, parseMinor } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { FinanceCapability } from "./permissions";

/**
 * Accounts (`/api/v1/finance/accounts`, §8.4). A cash, bank, M-Pesa or card
 * account holds the money the cash position is summed from; the finance officer
 * opens one and can rename or close it later. Every write is a `finance.account.*`
 * route, which Go authorises on its own.
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
function accountBody(data: FormData, fallbackCurrency: string): FinanceAccountBody {
  const currency = formCurrency(data, fallbackCurrency);
  return {
    name: field(data, "name"),
    account_type: field(data, "account_type"),
    currency,
    opening_balance_minor: parseMinor(field(data, "opening_balance"), currency),
    active: data.get("active") !== null,
  };
}

/**
 * The fields shared by "new account" and "edit account". Uncontrolled, so the
 * caller collects them once with `accountBody`; the amount hint follows the
 * currency field, so the figure is always read in the currency it is typed in.
 */
function AccountFields({
  account,
  defaultCurrency,
}: {
  account?: FinanceAccount;
  defaultCurrency: string;
}) {
  const [currency, setCurrency] = useState(account?.currency || defaultCurrency);

  return (
    <>
      <Field label="Name" htmlFor="account-name">
        <TextInput
          id="account-name"
          name="name"
          required
          defaultValue={account?.name ?? ""}
          placeholder="e.g. Equity business account"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Type" htmlFor="account-type">
          <Select
            id="account-type"
            name="account_type"
            defaultValue={account?.account_type ?? FINANCE_ACCOUNT_TYPES[0]}
          >
            {FINANCE_ACCOUNT_TYPES.map((type) => (
              <option key={type} value={type}>
                {humanise(type)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Currency" hint="ISO code" htmlFor="account-currency">
          <TextInput
            id="account-currency"
            name="currency"
            value={currency}
            onChange={(event) => setCurrency(event.target.value.toUpperCase())}
          />
        </Field>
      </div>

      <Field
        label="Opening balance"
        hint={currency}
        htmlFor="account-opening"
      >
        <TextInput
          id="account-opening"
          name="opening_balance"
          inputMode="decimal"
          placeholder="0.00"
          defaultValue={
            account ? formatMinorInput(account.opening_balance_minor, account.currency) : ""
          }
        />
      </Field>

      <Switch name="active" defaultChecked={account?.active ?? true} label="Active" />
    </>
  );
}

/**
 * "New account" (`POST /api/v1/finance/accounts`, `finance.account.create`): a
 * button that opens the account form in a drawer.
 */
export function NewAccountButton({
  currency,
  capabilities,
  variant = "primary",
}: {
  currency: string;
  capabilities: FinanceCapability[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("finance.account.create")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createAccount(accountBody(new FormData(event.currentTarget), currency)),
    );
    if (created) {
      setOpen(false);
      notify(`Added ${created.name}`);
      router.refresh();
    }
  }

  return (
    <>
      <Button variant={variant} size={variant === "secondary" ? "sm" : "md"} onClick={() => setOpen(true)}>
        <Icon name="plus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New account
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New account"
        description="Where cash sits. The opening balance is the starting point the cash position is built on."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The account wasn’t saved" />}

          <AccountFields defaultCurrency={currency} />

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Add account"}
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
 * The actions for one account row: rename it, move it between types, correct the
 * opening balance, or retire it. All of it is `finance.account.manage` via
 * `PUT /api/v1/finance/accounts/{id}`, guarded by `If-Match`.
 */
export function AccountActions({
  account,
  currency,
  capabilities,
}: {
  account: FinanceAccount;
  currency: string;
  capabilities: FinanceCapability[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("finance.account.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateAccount(
        account.id,
        accountBody(new FormData(event.currentTarget), currency),
        account.version,
      ),
    );
    if (updated) {
      close();
      notify(`Saved ${updated.name}`);
      router.refresh();
    }
  }

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
          Edit
        </Button>
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      <Drawer
        open={open}
        onClose={close}
        title="Edit account"
        description={account.name}
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}

          <AccountFields account={account} defaultCurrency={currency} />

          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save account"}
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

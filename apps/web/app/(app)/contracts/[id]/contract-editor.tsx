"use client";

import { useState } from "react";

import { Button, Field, Select } from "@/components/form";
import { Card, ProblemNotice } from "@/components/ui";
import {
  activateContract,
  approveContract,
  sendContract,
  setContractStatus,
  signContract,
  submitContract,
  terminateContract,
  type Contract,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/** Status changes and lifecycle actions for a contract (§8.7). */
export function ContractEditor({
  contract,
  isSigned,
}: {
  contract: Contract;
  isSigned: boolean;
}) {
  const { busy, problem, run, reset } = useMutation();
  const [status, setStatus] = useState(contract.status);

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return setContractStatus(contract.id, contract.version, {
        status: status as any,
      });
    });
    if (updated) {
      reset();
    }
  }

  async function onSign(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    const signed = await run(async () => {
      return signContract(contract.id, contract.version, {
        name: "",
        role: "",
        method: "",
      });
    });
    if (signed) {
      reset();
    }
  }

  async function onLifecycle(action: string) {
    const updated = await run(async () => {
      switch (action) {
        case "submit":
          return submitContract(contract.id, contract.version);
        case "approve":
          return approveContract(contract.id, contract.version);
        case "send":
          return sendContract(contract.id, contract.version);
        case "activate":
          return activateContract(contract.id, contract.version);
        case "terminate":
          return terminateContract(contract.id, contract.version);
        default:
          return null;
      }
    });
    if (updated) {
      reset();
    }
  }

  // Lifecycle actions available based on current status
  const lifecycleActions: { label: string; action: string; variant: string }[] =
    [];
  if (contract.status === "draft") {
    lifecycleActions.push({
      label: "Submit for review",
      action: "submit",
      variant: "primary",
    });
  } else if (contract.status === "internal_review") {
    lifecycleActions.push({
      label: "Approve",
      action: "approve",
      variant: "primary",
    });
  } else if (contract.status === "approved") {
    lifecycleActions.push({
      label: "Send to client",
      action: "send",
      variant: "primary",
    });
  } else if (contract.status === "sent") {
    lifecycleActions.push({
      label: "Sign contract",
      action: "sign",
      variant: "primary",
    });
  } else if (contract.status === "signed") {
    lifecycleActions.push({
      label: "Activate",
      action: "activate",
      variant: "primary",
    });
  } else if (contract.status === "active") {
    lifecycleActions.push({
      label: "Terminate",
      action: "terminate",
      variant: "danger",
    });
  }

  return (
    <Card title="Manage contract">
      {problem && (
        <ProblemNotice error={problem} title="The contract wasn't updated" />
      )}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Status">
          <Select
            name="status"
            value={status}
            onChange={(e) => setStatus(e.target.value as any)}
          >
            <option value="draft">Draft</option>
            <option value="sent">Sent</option>
            <option value="internal_review">Internal review</option>
            <option value="approved">Approved</option>
            <option value="signed">Signed</option>
            <option value="active">Active</option>
            <option value="renewal_due">Renewal due</option>
            <option value="terminated">Terminated</option>
          </Select>
        </Field>

        <div className="border-t border-line pt-4">
          <p className="mb-3 text-[12px] font-medium text-night">
            Lifecycle actions
          </p>
          <div className="flex flex-wrap gap-2">
            {lifecycleActions.map((action) => (
              <Button
                key={action.action}
                type="button"
                variant={action.variant as any}
                onClick={() => onLifecycle(action.action)}
                disabled={busy}
              >
                {busy ? "Processing…" : action.label}
              </Button>
            ))}
            {!isSigned && contract.status === "sent" && (
              <Button
                type="button"
                variant="primary"
                onClick={onSign}
                disabled={busy}
              >
                {busy ? "Signing…" : "Sign contract"}
              </Button>
            )}
            <Button
              type="submit"
              variant="secondary"
              disabled={busy}
              className="ml-auto"
            >
              {busy ? "Saving…" : "Save status"}
            </Button>
          </div>
        </div>
      </form>
    </Card>
  );
}

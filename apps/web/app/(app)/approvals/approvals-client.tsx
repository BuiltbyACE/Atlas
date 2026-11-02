"use client";

import { useState } from "react";

import {
  Badge,
  Card,
  EmptyState,
  ProblemNotice,
  Table,
  Td,
  Th,
  Tag,
  statusTone,
} from "@/components/ui";
import { Button, Field, Select, TextInput, TextArea } from "@/components/form";
import {
  approveApproval,
  cancelApproval,
  declineApproval,
  escalateApproval,
  requestApproval,
  type Approval,
  type ApprovalPolicy,
  type CreateApprovalBody,
} from "@/lib/api/atlas";
import { useToast } from "@/components/toast";
import { formatDay, formatMinor } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";
import { Drawer } from "@/components/drawer";
import { Icon } from "@/components/icons";

export function ApprovalsClient({
  me,
  pending,
  requested,
  policies,
}: {
  me: string;
  pending: Approval[];
  requested: Approval[];
  policies: ApprovalPolicy[];
}) {
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [activeTab, setActiveTab] = useState<"pending" | "requested">(
    "pending",
  );
  const [newApprovalOpen, setNewApprovalOpen] = useState(false);

  const items = activeTab === "pending" ? pending : requested;

  async function handleDecide(
    id: string,
    version: number,
    approve: boolean,
    reason: string,
  ) {
    const result = await run(async () => {
      return approve
        ? approveApproval(id, version, { approve: true, reason })
        : declineApproval(id, version, { approve: false, reason });
    });
    if (result) {
      notify(result.status === "approved" ? "Approved" : "Declined");
    }
  }

  async function handleEscalate(
    id: string,
    version: number,
    toRole: string,
    toPersonId: string,
    reason: string,
  ) {
    const result = await run(async () => {
      return escalateApproval(id, version, {
        to_role: toRole,
        to_person_id: toPersonId || undefined,
        reason,
      });
    });
    if (result) {
      notify("Escalated");
    }
  }

  async function handleCancel(id: string, version: number, reason: string) {
    const result = await run(async () => {
      return cancelApproval(id, version, { reason });
    });
    if (result) {
      notify("Cancelled");
    }
  }

  return (
    <div className="space-y-6">
      {problem && (
        <ProblemNotice error={problem} title="An error occurred" />
      )}

      {items.length === 0 ? (
        <EmptyState
          icon="check"
          iconTone="success"
          title={
            activeTab === "pending"
              ? "No approvals waiting for you"
              : "You haven't requested any approvals"
          }
        >
          {activeTab === "pending"
            ? "All approvals requiring your decision have been actioned."
            : "Request a new approval below."}
        </EmptyState>
      ) : (
        <Table>
          <thead>
            <tr>
              <Th>Ref</Th>
              <Th>Entity</Th>
              <Th>Status</Th>
              <Th className="text-right">Amount</Th>
              <Th>Due</Th>
              <Th>Action</Th>
            </tr>
          </thead>
          <tbody>
            {items.map((a) => (
              <tr key={a.id}>
                <Td>
                  <Tag>{a.ref}</Tag>
                </Td>
                <Td className="text-muted">
                  {a.entity_type} {a.entity_id.slice(0, 8)}
                </Td>
                <Td>
                  <Badge tone={statusTone(a.status)} dot>
                    {humanise(a.status)}
                  </Badge>
                </Td>
                <Td className="tabular text-right">
                  {formatMinor(a.amount_minor, a.currency)}
                </Td>
                <Td className="text-muted">
                  {a.due_at ? formatDay(a.due_at) : "—"}
                </Td>
                <Td>
                  {activeTab === "pending" ? (
                    <div className="flex flex-col gap-2">
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        onClick={() =>
                          handleDecide(a.id, a.version, true, "")
                        }
                      >
                        Approve
                      </Button>
                      <Button
                        type="button"
                        variant="danger"
                        size="sm"
                        onClick={() =>
                          handleDecide(a.id, a.version, false, "")
                        }
                      >
                        Decline
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() =>
                          handleEscalate(a.id, a.version, "", "", "")
                        }
                      >
                        Escalate
                      </Button>
                    </div>
                  ) : (
                    <div className="flex flex-col gap-2">
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() =>
                          handleCancel(a.id, a.version, "")
                        }
                      >
                        Cancel
                      </Button>
                    </div>
                  )}
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}

      <div className="flex justify-end pt-4">
        <Button
          type="button"
          variant="primary"
          onClick={() => setNewApprovalOpen(true)}
        >
          <Icon name="plus" className="h-4 w-4" />
          Request approval
        </Button>
      </div>

      {newApprovalOpen && (
        <NewApprovalDrawer
          open={newApprovalOpen}
          onClose={() => setNewApprovalOpen(false)}
          policies={policies}
        />
      )}
    </div>
  );
}

function humanise(s: string): string {
  return s
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
    .join(" ");
}

function NewApprovalDrawer({
  open,
  onClose,
  policies,
}: {
  open: boolean;
  onClose: () => void;
  policies: ApprovalPolicy[];
}) {
  const { busy, problem, run, reset } = useMutation();
  const [formData, setFormData] = useState<CreateApprovalBody>({
    entity_type: "",
    entity_id: "",
    amount_minor: 0,
    currency: "KES",
    approver_role: "",
    approver_id: undefined,
    approvals_required: 1,
    policy_ref: undefined,
    due_at: undefined,
  });

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const amountMinor = Number(
      String(data.get("amount_minor") ?? "0").trim() || "0",
    );
    const body: CreateApprovalBody = {
      entity_type: String(data.get("entity_type") ?? "").trim(),
      entity_id: String(data.get("entity_id") ?? "").trim(),
      amount_minor: amountMinor,
      currency: String(data.get("currency") ?? "KES").trim(),
      approver_role: String(data.get("approver_role") ?? "").trim(),
      approver_id:
        String(data.get("approver_id") ?? "").trim() || undefined,
      approvals_required:
        Number(String(data.get("approvals_required") ?? "1").trim()) || 1,
      policy_ref:
        String(data.get("policy_ref") ?? "").trim() || undefined,
      due_at:
        String(data.get("due_at") ?? "").trim() || undefined,
    };
    const created = await run(async () => {
      return requestApproval(body);
    });
    if (created) {
      onClose();
      reset();
    }
  }

  return (
    <Drawer
      open={open}
      onClose={onClose}
      title="Request approval"
      description="Request an approval from another role or person. Both CEOs are required for approvals above T2."
    >
      {problem && (
        <ProblemNotice error={problem} title="The approval wasn't created" />
      )}
      <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Entity type" htmlFor="entity-type">
          <TextInput
            id="entity-type"
            name="entity_type"
            required
            data-autofocus
            placeholder="e.g. expense, invoice, contract"
          />
        </Field>
        <Field label="Entity ID" htmlFor="entity-id">
          <TextInput
            id="entity-id"
            name="entity_id"
            required
            placeholder="UUID of the entity being approved"
          />
        </Field>
        <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
          <Field label="Amount (minor units)" htmlFor="amount-minor">
            <TextInput
              id="amount-minor"
              name="amount_minor"
              inputMode="decimal"
              type="number"
              required
            />
          </Field>
          <Field label="Currency" htmlFor="currency">
            <Select id="currency" name="currency" defaultValue="KES">
              <option value="KES">KES</option>
              <option value="USD">USD</option>
              <option value="EUR">EUR</option>
            </Select>
          </Field>
        </div>
        <Field label="Approver role" htmlFor="approver-role">
          <Select id="approver-role" name="approver_role" required>
            <option value="">Choose a role</option>
            <option value="line_manager">Line manager</option>
            <option value="domain_ceo">Domain CEO</option>
            <option value="both_ceos">Both CEOs</option>
          </Select>
        </Field>
        <Field label="Approver person (optional)" htmlFor="approver-id">
          <TextInput
            id="approver-id"
            name="approver_id"
            placeholder="Person UUID (optional)"
          />
        </Field>
        <Field label="Approvals required" htmlFor="approvals-required">
          <TextInput
            id="approvals-required"
            name="approvals_required"
            type="number"
            defaultValue="1"
          />
        </Field>
        <Field label="Policy ref" htmlFor="policy-ref">
          <TextInput
            id="policy-ref"
            name="policy_ref"
            placeholder="Optional policy reference"
          />
        </Field>
        <Field label="Due at" htmlFor="due-at">
          <TextInput id="due-at" name="due_at" type="datetime-local" />
        </Field>
        <div className="flex justify-end gap-2 pt-4">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Requesting…" : "Request approval"}
          </Button>
        </div>
      </form>
    </Drawer>
  );
}
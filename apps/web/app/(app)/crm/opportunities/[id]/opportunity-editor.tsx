"use client";

import { useState } from "react";

import { Button, Field, Select } from "@/components/form";
import { Card, ProblemNotice } from "@/components/ui";
import {
  createProposal,
  moveOpportunityStage,
  type Opportunity,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/** Stage movement and win/lose actions for an opportunity (§8.1). */
export function OpportunityEditor({
  opportunity,
}: {
  opportunity: Opportunity;
}) {
  const { busy, problem, run, reset } = useMutation();
  const [stageKey, setStageKey] = useState(opportunity.stage_key);
  const [isOpen, setIsOpen] = useState(
    opportunity.stage_key !== "won" &&
      opportunity.stage_key !== "lost" &&
      opportunity.stage_key !== "rejected",
  );

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return moveOpportunityStage(opportunity.id, opportunity.version, {
        stage_key: stageKey as any,
      });
    });
    if (updated) {
      reset();
    }
  }

  async function onWin(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return moveOpportunityStage(opportunity.id, opportunity.version, {
        stage_key: "won" as any,
      });
    });
    if (updated) {
      reset();
    }
  }

  async function onLose(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return moveOpportunityStage(opportunity.id, opportunity.version, {
        stage_key: "lost" as any,
      });
    });
    if (updated) {
      reset();
    }
  }

  async function onCreateProposal(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    const created = await run(async () => {
      return createProposal(opportunity.id, {
        scope_summary: "",
        price_minor: 0,
        currency: opportunity.currency,
      });
    });
    if (created) {
      reset();
    }
  }

  return (
    <Card title="Manage opportunity">
      {problem && (
        <ProblemNotice
          error={problem}
          title="The opportunity wasn't updated"
        />
      )}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Stage">
          <Select
            name="stage_key"
            value={stageKey}
            onChange={(e) => setStageKey(e.target.value)}
          >
            <option value="new">New</option>
            <option value="qualified">Qualified</option>
            <option value="proposal">Proposal</option>
            <option value="negotiation">Negotiation</option>
            <option value="closed_won">Closed won</option>
            <option value="closed_lost">Closed lost</option>
          </Select>
        </Field>

        {isOpen && (
          <div className="flex items-end gap-2 pt-4">
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Save stage"}
            </Button>
            <Button
              type="button"
              variant="primary"
              onClick={onWin}
              disabled={busy}
            >
              Mark as won
            </Button>
            <Button
              type="button"
              variant="danger"
              onClick={onLose}
              disabled={busy}
            >
              Mark as lost
            </Button>
            <Button
              type="button"
              variant="secondary"
              onClick={onCreateProposal}
              disabled={busy}
            >
              {busy ? "Creating…" : "Create proposal"}
            </Button>
          </div>
        )}

        {!isOpen && (
          <div className="flex items-end gap-2 pt-4">
            <Button
              type="button"
              variant="ghost"
              onClick={() => setIsOpen(true)}
            >
              Reopen
            </Button>
          </div>
        )}
      </form>
    </Card>
  );
}

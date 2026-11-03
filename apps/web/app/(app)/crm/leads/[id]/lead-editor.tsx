"use client";

import { useState } from "react";

import { Button, Field, Select } from "@/components/form";
import { Card, ProblemNotice } from "@/components/ui";
import {
  convertLead,
  setLeadStatus,
  type Lead,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/** Status change and conversion for a lead (§8.1). */
export function LeadEditor({ lead }: { lead: Lead }) {
  const { busy, problem, run, reset } = useMutation();
  const [status, setStatus] = useState(lead.status);
  const [converting, setConverting] = useState(false);

  async function onSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(async () => {
      return setLeadStatus(lead.id, lead.version, { status: status as any });
    });
    if (updated) {
      reset();
    }
  }

  async function onConvert(event: React.MouseEvent<HTMLButtonElement>) {
    event.preventDefault();
    setConverting(true);
    const converted = await run(async () => {
      return convertLead(lead.id, lead.version, {
        title: lead.need_summary ?? `Opportunity from ${lead.ref}`,
        organization_id: lead.organization_id,
      });
    });
    if (converted) {
      setConverting(false);
      reset();
    }
  }

  return (
    <Card title="Manage lead">
      {problem && (
        <ProblemNotice error={problem} title="The lead wasn't updated" />
      )}

      <form onSubmit={onSave} className="flex flex-col gap-4 px-6 py-[22px]">
        <Field label="Status">
          <Select
            name="status"
            value={status}
            onChange={(e) => setStatus(e.target.value as any)}
          >
            <option value="draft">Draft</option>
            <option value="submitted">Submitted</option>
            <option value="assigned">Assigned</option>
            <option value="contacted">Contacted</option>
            <option value="qualified">Qualified</option>
            <option value="nurture">Nurture</option>
            <option value="lost">Lost</option>
            <option value="disqualified">Disqualified</option>
          </Select>
        </Field>
        <div className="flex justify-end gap-2 pt-4">
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Saving…" : "Save status"}
          </Button>
          {lead.status !== "converted" && lead.status !== "lost" && (
            <Button
              type="button"
              variant="secondary"
              onClick={onConvert}
              disabled={converting || busy}
            >
              {converting ? "Converting…" : "Convert to opportunity"}
            </Button>
          )}
        </div>
      </form>
    </Card>
  );
}

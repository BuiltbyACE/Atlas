"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button, ButtonLink, Field, Select, TextInput, TextArea } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createFollowup,
  type Followup,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { useMutation } from "@/lib/use-mutation";

/**
 * Schedule a new follow-up (`POST /api/v1/crm/followups`).
 */
export default function NewFollowupPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [leadId, setLeadId] = useState("");
  const [opportunityId, setOpportunityId] = useState("");
  const [ownerPersonId, setOwnerPersonId] = useState("");
  const [dueAt, setDueAt] = useState("");
  const [channel, setChannel] = useState("call");
  const [reason, setReason] = useState("");

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(async () => {
      return createFollowup({
        lead_id: leadId || undefined,
        opportunity_id: opportunityId || undefined,
        owner_person_id: ownerPersonId || undefined,
        due_at: dueAt,
        channel: channel,
        reason: reason,
      });
    });

    if (created) {
      reset();
      notify(`Follow-up ${created.id.slice(0, 8)} scheduled`);
      router.push(`/crm/followups`);
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
      {problem && (
        <ProblemNotice error={problem} title="The follow-up wasn't scheduled" />
      )}

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Field label="Lead ID (optional)">
          <TextInput
            name="lead_id"
            value={leadId}
            onChange={(e) => setLeadId(e.target.value)}
            placeholder="Lead UUID"
          />
        </Field>
        <Field label="Opportunity ID (optional)">
          <TextInput
            name="opportunity_id"
            value={opportunityId}
            onChange={(e) => setOpportunityId(e.target.value)}
            placeholder="Opportunity UUID"
          />
        </Field>
        <Field label="Owner person ID">
          <TextInput
            name="owner_person_id"
            value={ownerPersonId}
            onChange={(e) => setOwnerPersonId(e.target.value)}
            placeholder="Person UUID"
            required
          />
        </Field>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label="Due at">
          <TextInput
            name="due_at"
            type="datetime-local"
            value={dueAt}
            onChange={(e) => setDueAt(e.target.value)}
            required
          />
        </Field>
        <Field label="Channel">
          <Select
            name="channel"
            value={channel}
            onChange={(e) => setChannel(e.target.value)}
            required
          >
            <option value="call">Call</option>
            <option value="email">Email</option>
            <option value="whatsapp">WhatsApp</option>
            <option value="meeting">Meeting</option>
            <option value="other">Other</option>
          </Select>
        </Field>
      </div>

      <Field label="Reason">
        <TextArea
          name="reason"
          value={reason}
          onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setReason(e.target.value)}
          rows={3}
          placeholder="Why is this follow-up needed?"
          required
        />
      </Field>

      <div className="flex justify-end gap-2 pt-4">
        <Button type="button" variant="secondary" onClick={() => router.back()}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? "Scheduling…" : "Schedule follow-up"}
        </Button>
      </div>
    </form>
  );
}

"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button, ButtonLink, Field, Select, TextInput, TextArea } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createInteraction,
  type CrmInteraction,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { useMutation } from "@/lib/use-mutation";

/**
 * Log a new interaction (`POST /api/v1/crm/interactions`).
 */
export default function NewInteractionPage() {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [interactionType, setInteractionType] = useState("call");
  const [direction, setDirection] = useState("outbound");
  const [contactId, setContactId] = useState("");
  const [leadId, setLeadId] = useState("");
  const [opportunityId, setOpportunityId] = useState("");
  const [occurredAt, setOccurredAt] = useState("");
  const [durationMinutes, setDurationMinutes] = useState("");
  const [outcome, setOutcome] = useState("");
  const [summary, setSummary] = useState("");
  const [nextAction, setNextAction] = useState("");
  const [nextActionDueAt, setNextActionDueAt] = useState("");
  const [recordingFileId, setRecordingFileId] = useState("");

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(async () => {
      return createInteraction({
        interaction_type: interactionType,
        direction: direction,
        contact_id: contactId || undefined,
        lead_id: leadId || undefined,
        opportunity_id: opportunityId || undefined,
        occurred_at: occurredAt || undefined,
        duration_minutes:
          durationMinutes ? parseInt(durationMinutes, 10) : undefined,
        outcome,
        summary: summary || undefined,
        next_action: nextAction || undefined,
        next_action_due_at: nextActionDueAt || undefined,
        recording_file_id: recordingFileId || undefined,
      });
    });

    if (created) {
      reset();
      notify(`Interaction ${created.id.slice(0, 8)} logged`);
      router.push(`/crm/interactions`);
    }
  }

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
      {problem && (
        <ProblemNotice error={problem} title="The interaction wasn't logged" />
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label="Interaction type">
          <Select
            name="interaction_type"
            value={interactionType}
            onChange={(e) => setInteractionType(e.target.value)}
            required
          >
            <option value="call">Call</option>
            <option value="email">Email</option>
            <option value="whatsapp">WhatsApp</option>
            <option value="meeting">Meeting</option>
            <option value="note">Note</option>
          </Select>
        </Field>
        <Field label="Direction">
          <Select
            name="direction"
            value={direction}
            onChange={(e) => setDirection(e.target.value)}
            required
          >
            <option value="outbound">Outbound</option>
            <option value="inbound">Inbound</option>
          </Select>
        </Field>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <Field label="Contact ID (optional)">
          <TextInput
            name="contact_id"
            value={contactId}
            onChange={(e) => setContactId(e.target.value)}
            placeholder="Contact UUID"
          />
        </Field>
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
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Field label="Occurred at">
          <TextInput
            name="occurred_at"
            type="datetime-local"
            value={occurredAt}
            onChange={(e) => setOccurredAt(e.target.value)}
          />
        </Field>
        <Field label="Duration (minutes)">
          <TextInput
            name="duration_minutes"
            type="number"
            value={durationMinutes}
            onChange={(e) => setDurationMinutes(e.target.value)}
            placeholder="Minutes"
          />
        </Field>
      </div>

      <Field label="Outcome">
        <TextInput
          name="outcome"
          value={outcome}
          onChange={(e) => setOutcome(e.target.value)}
          placeholder="e.g. Left voicemail, agreed next call"
        />
      </Field>

      <Field label="Summary">
        <TextArea
          name="summary"
          value={summary}
          onChange={(e) => setSummary(e.target.value)}
          rows={3}
          placeholder="What was discussed?"
        />
      </Field>

      <Field label="Next action">
        <TextInput
          name="next_action"
          value={nextAction}
          onChange={(e) => setNextAction(e.target.value)}
          placeholder="e.g. Send proposal"
        />
      </Field>

      <Field label="Next action due at">
        <TextInput
          name="next_action_due_at"
          type="datetime-local"
          value={nextActionDueAt}
          onChange={(e) => setNextActionDueAt(e.target.value)}
        />
      </Field>

      <Field label="Recording file ID (optional)">
        <TextInput
          name="recording_file_id"
          value={recordingFileId}
          onChange={(e) => setRecordingFileId(e.target.value)}
          placeholder="File UUID"
        />
      </Field>

      <div className="flex justify-end gap-2 pt-4">
        <Button type="button" variant="secondary" onClick={() => router.back()}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={busy}>
          {busy ? "Logging…" : "Log interaction"}
        </Button>
      </div>
    </form>
  );
}

"use client";

import { useRef, useState, type FormEvent } from "react";

import { Button, Field, Select, TextInput } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import {
  addProjectEvidence,
  addTaskEvidence,
  EVIDENCE_TYPES,
  uploadFile,
  type Evidence,
  type EvidenceBody,
  type EvidenceType,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

import { EVIDENCE_LABELS, EVIDENCE_PLACEHOLDERS } from "./board";

/**
 * Attach evidence to a task or a project (§6.5). Most proof is a reference - a
 * merged PR, a test run, a client sign-off - so that is the default; a file
 * goes through the two-step upload (Go hands out a pre-signed ticket, bytes go
 * straight to object storage, then Go is told it landed) before it is linked.
 * Go validates the type/reference pairing and refuses a duplicate reference.
 */
export function EvidenceForm({
  taskId,
  projectId,
  onAttached,
  onCancel,
}: {
  taskId?: string;
  projectId?: string;
  onAttached: (evidence: Evidence) => void;
  onCancel?: () => void;
}) {
  const { busy, problem, run } = useMutation();
  const [type, setType] = useState<EvidenceType>("pull_request");
  const fileInput = useRef<HTMLInputElement>(null);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const description = String(data.get("description") ?? "").trim();

    const attached = await run(async () => {
      let body: EvidenceBody;
      if (type === "file") {
        const file = fileInput.current?.files?.[0];
        if (!file) throw new Error("Choose a file to upload.");
        const record = await uploadFile(file, {
          entity_type: taskId ? "work.task" : "work.project",
          entity_id: taskId ?? projectId,
        });
        body = { evidence_type: "file", file_id: record.id, description };
      } else {
        body = {
          evidence_type: type,
          reference: String(data.get("reference") ?? "").trim(),
          description,
        };
      }
      return taskId
        ? addTaskEvidence(taskId, body)
        : addProjectEvidence(projectId as string, body);
    });

    if (attached) {
      form.reset();
      setType("pull_request");
      onAttached(attached);
    }
  }

  const idPrefix = taskId ? `ev-${taskId.slice(0, 8)}` : `ev-p-${projectId?.slice(0, 8)}`;

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-3">
      {problem && <ProblemNotice error={problem} title="Evidence was not attached" />}

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,180px)_minmax(0,1fr)]">
        <Field label="Type" htmlFor={`${idPrefix}-type`}>
          <Select
            id={`${idPrefix}-type`}
            value={type}
            onChange={(event) => setType(event.target.value as EvidenceType)}
          >
            {EVIDENCE_TYPES.map((value) => (
              <option key={value} value={value}>
                {EVIDENCE_LABELS[value]}
              </option>
            ))}
          </Select>
        </Field>
        {type === "file" ? (
          <Field label="File" htmlFor={`${idPrefix}-file`}>
            <input
              id={`${idPrefix}-file`}
              ref={fileInput}
              type="file"
              required
              className="block h-11 w-full text-sm text-muted file:mr-3 file:h-11 file:rounded-control file:border-0 file:bg-[#EEF1F6] file:px-3.5 file:text-sm file:font-semibold file:text-night hover:file:bg-[#E3E7EE]"
            />
          </Field>
        ) : (
          <Field label="Reference" htmlFor={`${idPrefix}-ref`}>
            <TextInput
              id={`${idPrefix}-ref`}
              name="reference"
              required
              placeholder={EVIDENCE_PLACEHOLDERS[type]}
            />
          </Field>
        )}
      </div>
      <Field label="Note" hint="optional" htmlFor={`${idPrefix}-note`}>
        <TextInput id={`${idPrefix}-note`} name="description" placeholder="What this proves" />
      </Field>

      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={busy}>
          {busy ? (type === "file" ? "Uploading…" : "Attaching…") : "Attach evidence"}
        </Button>
        {onCancel && (
          <Button type="button" size="sm" variant="secondary" onClick={onCancel} disabled={busy}>
            Cancel
          </Button>
        )}
      </div>
    </form>
  );
}

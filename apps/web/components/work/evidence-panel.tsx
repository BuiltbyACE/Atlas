"use client";

import { useRouter } from "next/navigation";
import { useRef, useState, type FormEvent } from "react";

import { Button, Field, TextInput } from "@/components/form";
import { Badge, Card, EmptyState, ProblemNotice } from "@/components/ui";
import {
  addProjectEvidence,
  addTaskEvidence,
  downloadFile,
  uploadFile,
  verifyEvidence,
  type Evidence,
} from "@/lib/api/atlas";
import { formatDateTime, humanise, shortId } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * Evidence for a task or a project (blueprint §6.5, §8.2).
 *
 * Uploading is two steps: Go hands out a pre-signed ticket (`POST /api/v1/files`)
 * and the bytes go straight to object storage, then Go is told the object is in
 * place and the branch attaches the resulting file as evidence. Bytes never pass
 * through the API. A different person verifies the evidence (separation of
 * duties, §3.3) - Go rejects self-verification with 409.
 *
 * Not everything is a file: a provider (GitHub's merged PR, ADR-018) attaches
 * evidence by `reference`, with no `file_id` to download. The subject of a row
 * is therefore rendered by `EvidenceSubject`, not assumed to be a file.
 */
export function EvidencePanel({
  taskId,
  projectId,
  evidence,
  title = "Evidence",
}: {
  taskId?: string;
  projectId?: string;
  evidence: Evidence[];
  title?: string;
}) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [verifying, setVerifying] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const file = fileInput.current?.files?.[0];
    if (!file) return;
    const data = new FormData(form);

    const attached = await run(async () => {
      const record = await uploadFile(file, {
        entity_type: taskId ? "work.task" : "work.project",
        entity_id: taskId ?? projectId,
      });
      const body = {
        file_id: record.id,
        kind: String(data.get("kind") ?? "").trim(),
        description: String(data.get("description") ?? ""),
      };
      return taskId
        ? addTaskEvidence(taskId, body)
        : addProjectEvidence(projectId as string, body);
    });

    if (attached) {
      form.reset();
      if (fileInput.current) fileInput.current.value = "";
      router.refresh();
    }
  }

  async function onVerify(id: string) {
    setVerifying(id);
    const verified = await run(() => verifyEvidence(id));
    setVerifying(null);
    if (verified) router.refresh();
  }

  async function onDownload(fileId: string) {
    const ticket = await run(() => downloadFile(fileId));
    if (ticket) window.open(ticket.url, "_blank", "noopener");
  }

  return (
    <Card
      title={title}
      description="Files and links that prove the work. A reviewer verifies them."
    >
      {problem && (
        <div className="mb-3">
          <ProblemNotice error={problem} title="Evidence change failed" />
        </div>
      )}

      {evidence.length === 0 ? (
        <EmptyState>Nothing attached yet.</EmptyState>
      ) : (
        <ul className="mb-4 space-y-2">
          {evidence.map((item) => {
            const fileId = item.file_id;
            return (
              <li
                key={item.id}
                className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="text-sm text-slate-800">
                    {item.kind ? <span className="font-medium">{item.kind} · </span> : null}
                    <EvidenceSubject item={item} />
                  </p>
                  {item.description && (
                    <p className="text-xs text-slate-500">{item.description}</p>
                  )}
                  <p className="mt-0.5 text-xs text-slate-400">
                    Added {formatDateTime(item.created_at)}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {item.verified_at ? (
                    <Badge tone="success">verified</Badge>
                  ) : (
                    <Button
                      variant="secondary"
                      disabled={busy && verifying === item.id}
                      onClick={() => onVerify(item.id)}
                    >
                      {verifying === item.id ? "Verifying…" : "Verify"}
                    </Button>
                  )}
                  {fileId && (
                    <Button variant="ghost" onClick={() => onDownload(fileId)}>
                      Download
                    </Button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}

      <form onSubmit={onSubmit} className="space-y-3 border-t border-slate-100 pt-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <Field label="File" htmlFor="evidence-file">
            <input
              id="evidence-file"
              ref={fileInput}
              type="file"
              required
              className="block w-full text-sm text-slate-600 file:mr-3 file:rounded-md file:border-0 file:bg-slate-100 file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-slate-700 hover:file:bg-slate-200"
            />
          </Field>
          <Field label="Kind" hint="optional" htmlFor="evidence-kind">
            <TextInput id="evidence-kind" name="kind" placeholder="pull_request, document…" />
          </Field>
          <Field label="Note" hint="optional" htmlFor="evidence-note">
            <TextInput id="evidence-note" name="description" />
          </Field>
        </div>
        <Button type="submit" disabled={busy}>
          {busy ? "Uploading…" : "Attach evidence"}
        </Button>
      </form>
    </Card>
  );
}

/**
 * What an evidence row points at. Uploads have a file; provider evidence has a
 * `reference` (a pull request URL, say) that is linked when it looks like one.
 */
function EvidenceSubject({ item }: { item: Evidence }) {
  if (item.file_id) {
    return (
      <span className="font-mono text-xs text-slate-500">file {shortId(item.file_id)}</span>
    );
  }
  if (item.reference) {
    return /^https?:\/\//i.test(item.reference) ? (
      <a
        href={item.reference}
        target="_blank"
        rel="noreferrer"
        className="text-xs text-sky-700 underline decoration-sky-300 underline-offset-2"
      >
        {item.reference}
      </a>
    ) : (
      <span className="font-mono text-xs text-slate-500">{item.reference}</span>
    );
  }
  return (
    <span className="text-xs text-slate-500">{humanise(item.evidence_type ?? "evidence")}</span>
  );
}

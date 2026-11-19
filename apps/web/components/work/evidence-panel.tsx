"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { Badge, Card, EmptyState, IconTile, ProblemNotice } from "@/components/ui";
import { downloadFile, verifyEvidence, type Evidence } from "@/lib/api/atlas";
import { formatDateTime, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import { EVIDENCE_LABELS } from "./board";
import { EvidenceForm } from "./evidence-form";

/**
 * Evidence for a task or a project (blueprint §6.5, §8.2).
 *
 * Proof is usually a reference - a merged PR (GitHub records those itself,
 * ADR-018), a test run, a client sign-off - or an uploaded file. A different
 * person verifies it (separation of duties, §3.3): Go refuses self-verification
 * with 409 and the reason is shown as Go gave it.
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
  const notify = useToast();
  const { busy, problem, run } = useMutation();
  const [verifying, setVerifying] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);

  async function onVerify(id: string) {
    setVerifying(id);
    const verified = await run(() => verifyEvidence(id));
    setVerifying(null);
    if (verified) {
      notify("Evidence verified");
      router.refresh();
    }
  }

  async function onDownload(fileId: string) {
    const ticket = await run(() => downloadFile(fileId));
    if (ticket) window.open(ticket.url, "_blank", "noopener");
  }

  return (
    <Card
      title={title}
      description="Links and files that prove the work. Someone other than the submitter verifies them."
      actions={
        !adding && (
          <Button size="sm" variant="secondary" onClick={() => setAdding(true)}>
            <Icon name="paperclip" className="h-4 w-4" />
            Attach evidence
          </Button>
        )
      }
    >
      {problem && (
        <div className="mb-3">
          <ProblemNotice error={problem} title="That didn’t go through" />
        </div>
      )}

      {adding && (
        <div className="mb-4 rounded-[14px] border border-line bg-[#FAFBFC] p-4">
          <EvidenceForm
            taskId={taskId}
            projectId={projectId}
            onCancel={() => setAdding(false)}
            onAttached={() => {
              setAdding(false);
              notify("Evidence attached");
              router.refresh();
            }}
          />
        </div>
      )}

      {evidence.length === 0 ? (
        !adding && (
          <EmptyState icon="paperclip" iconTone="info" title="Nothing attached yet">
            {taskId
              ? "A task moves to Done once evidence is attached: a merged pull request, a document, a test result or a client approval."
              : "Attach the documents and sign-offs that prove this project delivered."}
          </EmptyState>
        )
      ) : (
        <ul className="flex flex-col gap-2.5">
          {evidence.map((item) => {
            const fileId = item.file_id;
            const label =
              EVIDENCE_LABELS[(item.evidence_type ?? "file") as keyof typeof EVIDENCE_LABELS] ??
              humanise(item.evidence_type ?? "evidence");
            return (
              <li
                key={item.id}
                className="flex flex-wrap items-center gap-3 rounded-[14px] border border-line px-3.5 py-3"
              >
                <IconTile
                  icon={fileId ? "document" : "link"}
                  tone={item.verified_at ? "success" : "info"}
                  size="sm"
                />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-semibold text-night">
                    {label}
                    {item.kind && <span className="font-normal text-muted"> · {item.kind}</span>}
                  </p>
                  <EvidenceSubject item={item} />
                  {item.description && (
                    <p className="text-[13px] text-muted">{item.description}</p>
                  )}
                  <p className="mt-0.5 text-xs text-[#7A879C]">
                    Added {formatDateTime(item.created_at)}
                    {!item.uploader_person_id && " by an integration"}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {item.verified_at ? (
                    <Badge tone="success" dot>
                      Verified
                    </Badge>
                  ) : (
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={busy && verifying === item.id}
                      onClick={() => onVerify(item.id)}
                    >
                      {verifying === item.id ? "Verifying…" : "Verify"}
                    </Button>
                  )}
                  {fileId && (
                    <Button size="sm" variant="ghost" onClick={() => onDownload(fileId)}>
                      Download
                    </Button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}

/**
 * What an evidence row points at. Uploads have a file; provider evidence has a
 * `reference` (a pull request URL, say) that is linked when it looks like one.
 */
function EvidenceSubject({ item }: { item: Evidence }) {
  if (item.reference) {
    return /^https?:\/\//i.test(item.reference) ? (
      <a
        href={item.reference}
        target="_blank"
        rel="noreferrer"
        className="block truncate text-[13px] text-atlas-ink underline decoration-atlas/30 underline-offset-2"
      >
        {item.reference}
      </a>
    ) : (
      <p className="truncate font-mono text-xs text-muted">{item.reference}</p>
    );
  }
  if (item.file_id) return <p className="text-[13px] text-muted">Uploaded file</p>;
  return null;
}

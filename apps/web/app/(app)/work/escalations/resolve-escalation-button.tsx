"use client";

import { useRouter } from "next/navigation";

import { Button } from "@/components/form";
import { ProblemNotice } from "@/components/ui";
import { resolveEscalation } from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/**
 * Closes an escalation (`POST /api/v1/work/escalations/{id}/resolve`, gated by
 * `work.task.review`). The route takes no body and needs no precondition.
 */
export function ResolveEscalationButton({ id }: { id: string }) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();

  async function onResolve() {
    const resolved = await run(() => resolveEscalation(id));
    if (resolved) router.refresh();
  }

  return (
    <div className="flex flex-col items-start gap-2">
      <Button variant="secondary" disabled={busy} onClick={onResolve}>
        {busy ? "Resolving…" : "Resolve"}
      </Button>
      {problem && <ProblemNotice error={problem} title="Could not resolve" />}
    </div>
  );
}

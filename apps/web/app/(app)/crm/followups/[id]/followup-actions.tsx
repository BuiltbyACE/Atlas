"use client";

import { useRouter } from "next/navigation";

import { Button, Field, TextInput } from "@/components/form";
import { Card, ProblemNotice } from "@/components/ui";
import { cancelFollowup, completeFollowup } from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

/**
 * Resolve a scheduled follow-up (§8.1): mark it done with an outcome, or
 * cancel it with a reason. A successful write refreshes the server page so
 * the new status is shown.
 */
export function FollowupActions({
  followupId,
  version,
}: {
  followupId: string;
  version: number;
}) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();

  async function onComplete(outcome: string) {
    const updated = await run(() =>
      completeFollowup(followupId, version, outcome),
    );
    if (updated) {
      router.refresh();
    }
  }

  async function onCancel() {
    const outcome = window.prompt("Reason for cancelling:");
    if (outcome === null) return;
    const updated = await run(() =>
      cancelFollowup(followupId, version, outcome),
    );
    if (updated) {
      router.refresh();
    }
  }

  return (
    <Card title="Resolve follow-up">
      {problem && (
        <ProblemNotice error={problem} title="The follow-up wasn't resolved" />
      )}

      <form
        className="flex flex-col gap-3 px-6 py-[22px]"
        onSubmit={(event) => {
          event.preventDefault();
          const data = new FormData(event.currentTarget);
          void onComplete(String(data.get("outcome") ?? "Completed"));
        }}
      >
        <Field label="Outcome" htmlFor="followup-outcome">
          <TextInput
            id="followup-outcome"
            name="outcome"
            required
            placeholder="e.g. Spoke with decision maker, sent proposal"
          />
        </Field>
        <div className="flex items-end gap-2 pt-4">
          <Button type="submit" variant="primary" disabled={busy}>
            {busy ? "Completing…" : "Mark as done"}
          </Button>
          <Button
            type="button"
            variant="danger"
            onClick={onCancel}
            disabled={busy}
          >
            Cancel
          </Button>
        </div>
      </form>
    </Card>
  );
}

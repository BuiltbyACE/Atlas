"use client";

import { useCallback, useState } from "react";

import { ApiError } from "@/lib/api/client";

/** A problem shape the shared `ProblemNotice` can render. */
export interface MutationProblem {
  message: string;
  code?: string;
  requestId?: string;
}

/**
 * Tiny helper for the Phase 2 client forms: one in-flight guard and one error
 * slot, so each form only has to describe its own fields. Go owns the rules; a
 * refusal arrives as `ApiError` and is surfaced verbatim (with its `code`).
 */
export function useMutation() {
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<MutationProblem | null>(null);

  const run = useCallback(
    async <T,>(action: () => Promise<T>): Promise<T | undefined> => {
      setBusy(true);
      setProblem(null);
      try {
        return await action();
      } catch (error) {
        if (error instanceof ApiError) {
          setProblem({
            message: error.message,
            code: error.code,
            requestId: error.requestId,
          });
        } else {
          setProblem({ message: "Something went wrong. Please try again." });
        }
        return undefined;
      } finally {
        setBusy(false);
      }
    },
    [],
  );

  const reset = useCallback(() => setProblem(null), []);

  return { busy, problem, run, reset };
}

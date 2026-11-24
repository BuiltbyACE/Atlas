/**
 * Atlas web - task-board presentation config: the columns, their order and
 * colours, and evidence-type labels. Which moves are legal is Go's call
 * (`allowedTaskTransition`); the board just asks and shows the answer.
 */

import type { EvidenceType, TaskStatus } from "@/lib/api/atlas";

export interface BoardColumn {
  status: TaskStatus;
  label: string;
  dot: string;
}

/** Board column order (task lifecycle, §8.2). Cancelled tasks are not shown. */
export const BOARD_COLUMNS: BoardColumn[] = [
  { status: "todo", label: "To do", dot: "#8A97AD" },
  { status: "in_progress", label: "In progress", dot: "#1A6CF5" },
  { status: "blocked", label: "Blocked", dot: "#D92D20" },
  { status: "review", label: "In review", dot: "#D97706" },
  { status: "done", label: "Done", dot: "#16A34A" },
];

export function columnLabel(status: string): string {
  return BOARD_COLUMNS.find((column) => column.status === status)?.label ?? status;
}

export const EVIDENCE_LABELS: Record<EvidenceType, string> = {
  pull_request: "Pull request",
  url: "Link",
  test_run: "Test run",
  client_approval: "Client approval",
  git_commit: "Commit",
  payment_reference: "Payment reference",
  file: "File upload",
};

/** What the reference field asks for, per evidence type. */
export const EVIDENCE_PLACEHOLDERS: Record<EvidenceType, string> = {
  pull_request: "https://github.com/…/pull/123",
  url: "https://…",
  test_run: "Link to the CI run or test report",
  client_approval: "Link to the sign-off, or who approved and when",
  git_commit: "Commit sha or link",
  payment_reference: "Payment or invoice reference",
  file: "",
};

/** The short tail of a task ref: `PRJ-2026-007/T-0142` -> `T-0142`. */
export function shortRef(ref?: string): string | undefined {
  if (!ref) return undefined;
  const parts = ref.split("/");
  return parts[parts.length - 1];
}

/**
 * Atlas web - the typed Atlas API surface (Phase 2).
 *
 * `lib/api/client.ts` is the transport (`apiFetch`); this module is the typed
 * seam in front of it, one function per Go route. Shapes mirror the JSON views
 * in `apps/api/internal/{work,people,dashboard,files}/http.go` exactly.
 *
 * Why hand-written and not generated: `npm run generate:api` is still a
 * placeholder because `contracts/openapi/public.yaml` does not describe the
 * work/people modules yet. When those schemas land this file is replaced by
 * `lib/api/generated/` and these wrappers either move to that output or wrap it.
 * Until then the rule is simple: this is the ONLY place that knows wire shapes.
 *
 * Go owns every rule and permission (blueprint §5.2). Nothing here validates
 * business rules; it forwards the session cookie, serialises bodies, and lets
 * `ApiError` (RFC 9457 + stable `code`) carry the failure back to the UI.
 *
 * Optimistic concurrency (§6.6): every update must carry the version it read in
 * `If-Match`, formatted as the ETag Go issued (`"<version>"`). Helpers below
 * take the version and set the header for you; a stale version is a 412.
 */

import { apiFetch } from "@/lib/api/client";

// ── pagination envelope (§11) ───────────────────────────────────────────────

/** The standard keyset-paginated list envelope every list route returns. */
export interface Page<T> {
  items: T[];
  /** Opaque keyset cursor for the next page; absent on the last page. */
  next_cursor?: string;
  has_more: boolean;
  limit: number;
}

// ── auth (§6.1) ─────────────────────────────────────────────────────────────

/**
 * Refresh the current session's activity (`POST /api/v1/auth/heartbeat`). The
 * Go gate touches `last_seen_at` before the handler runs, so this is a cheap
 * 204 that keeps a signed-in tab counted as "online" in the people activity
 * view (§8.5). A service token has no session and gets a 401, which callers
 * ignore.
 */
export function heartbeat(): Promise<void> {
  return apiFetch<void>("/api/v1/auth/heartbeat", { method: "POST" });
}

// ── work: enums ─────────────────────────────────────────────────────────────

export type ProjectStatus =
  | "planning"
  | "active"
  | "paused"
  | "completed"
  | "cancelled";

export type MilestoneStatus =
  | "not_started"
  | "in_progress"
  | "blocked"
  | "completed"
  | "cancelled";

export type TaskStatus =
  | "todo"
  | "in_progress"
  | "blocked"
  | "review"
  | "done"
  | "cancelled";

export type Priority = "low" | "medium" | "high" | "urgent";

export const PROJECT_STATUSES: ProjectStatus[] = [
  "planning",
  "active",
  "paused",
  "completed",
  "cancelled",
];

export const MILESTONE_STATUSES: MilestoneStatus[] = [
  "not_started",
  "in_progress",
  "blocked",
  "completed",
  "cancelled",
];

/** Board column order (task lifecycle, §8.2). */
export const TASK_STATUSES: TaskStatus[] = [
  "todo",
  "in_progress",
  "blocked",
  "review",
  "done",
  "cancelled",
];

export const PRIORITIES: Priority[] = ["low", "medium", "high", "urgent"];

// ── work: records ───────────────────────────────────────────────────────────

export interface Project {
  id: string;
  name: string;
  description?: string;
  project_type?: string;
  status: ProjectStatus | string;
  owner_person_id?: string;
  department_id?: string;
  started_at?: string;
  due_at?: string;
  completed_at?: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface Milestone {
  id: string;
  project_id: string;
  name: string;
  description?: string;
  status: MilestoneStatus | string;
  due_at?: string;
  completed_at?: string;
  weight: number;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface Task {
  id: string;
  project_id: string;
  milestone_id?: string;
  parent_id?: string;
  title: string;
  description?: string;
  assignee_person_id?: string;
  reporter_person_id?: string;
  status: TaskStatus | string;
  priority: Priority | string;
  due_at?: string;
  started_at?: string;
  completed_at?: string;
  blocked: boolean;
  blocked_reason?: string;
  estimate_hours?: number;
  actual_hours?: number;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface Blocker {
  id: string;
  task_id: string;
  reason: string;
  created_by?: string;
  resolved_by?: string;
  resolved_at?: string;
  created_at: string;
}

export interface ProjectMember {
  person_id: string;
  role: string;
  joined_at: string;
}

export interface Evidence {
  id: string;
  task_id?: string;
  project_id?: string;
  /**
   * `file` for an upload, otherwise the provider that recorded it
   * (`pull_request`, …). Exactly one of `file_id` / `reference` is set.
   */
  evidence_type?: string;
  file_id?: string;
  reference?: string;
  uploader_person_id?: string;
  kind?: string;
  description?: string;
  verified_by_person_id?: string;
  verified_at?: string;
  created_at: string;
}

export interface Escalation {
  id: string;
  entity_type: string;
  entity_id: string;
  project_id?: string;
  level: number;
  status: "open" | "resolved" | string;
  reason?: string;
  assignee_person_id?: string;
  raised_by_person_id?: string;
  resolved_by_person_id?: string;
  resolved_at?: string;
  version: number;
  created_at: string;
  updated_at: string;
}

// ── work: request bodies ────────────────────────────────────────────────────

export interface ProjectBody {
  name: string;
  description?: string;
  project_type?: string;
  status?: ProjectStatus | string;
  owner_person_id?: string | null;
  department_id?: string | null;
  started_at?: string | null;
  due_at?: string | null;
}

export interface MilestoneBody {
  name: string;
  description?: string;
  status?: MilestoneStatus | string;
  due_at?: string | null;
  weight?: number;
}

export interface TaskBody {
  milestone_id?: string | null;
  parent_id?: string | null;
  title: string;
  description?: string;
  assignee_person_id?: string | null;
  status?: TaskStatus | string;
  priority?: Priority | string;
  due_at?: string | null;
  estimate_hours?: number | null;
  actual_hours?: number | null;
}

export interface EvidenceBody {
  file_id: string;
  kind?: string;
  description?: string;
}

// ── work: queries ───────────────────────────────────────────────────────────

export type ProjectListQuery = {
  status?: string;
  owner_person_id?: string;
  department?: string;
  limit?: number;
  cursor?: string;
};

export type TaskListQuery = {
  project_id?: string;
  milestone_id?: string;
  assignee_person_id?: string;
  status?: string;
  blocked?: "true" | "false";
  overdue?: "true";
  limit?: number;
  cursor?: string;
};

export type EscalationListQuery = {
  entity_type?: string;
  entity_id?: string;
  project_id?: string;
  status?: string;
  limit?: number;
  cursor?: string;
};

/** Set the `If-Match` precondition header from a version (§6.6). */
export function ifMatch(version: number): { "If-Match": string } {
  return { "If-Match": `"${version}"` };
}

// ── work: projects ──────────────────────────────────────────────────────────

export function listProjects(
  query: ProjectListQuery = {},
): Promise<Page<Project>> {
  return apiFetch<Page<Project>>("/api/v1/work/projects", {
    query,
    cache: "no-store",
  });
}

export function getProject(id: string): Promise<Project> {
  return apiFetch<Project>(`/api/v1/work/projects/${id}`, {
    cache: "no-store",
  });
}

export function createProject(body: ProjectBody): Promise<Project> {
  return apiFetch<Project>("/api/v1/work/projects", { method: "POST", body });
}

export function updateProject(
  id: string,
  body: ProjectBody,
  version: number,
): Promise<Project> {
  return apiFetch<Project>(`/api/v1/work/projects/${id}`, {
    method: "PUT",
    body,
    headers: ifMatch(version),
  });
}

export function listMembers(projectId: string): Promise<ProjectMember[]> {
  return apiFetch<{ items: ProjectMember[] }>(
    `/api/v1/work/projects/${projectId}/members`,
    { cache: "no-store" },
  ).then((page) => page.items);
}

export function addMember(
  projectId: string,
  body: { person_id: string; role?: string },
): Promise<ProjectMember> {
  return apiFetch<ProjectMember>(
    `/api/v1/work/projects/${projectId}/members`,
    { method: "POST", body },
  );
}

export function removeMember(
  projectId: string,
  personId: string,
): Promise<void> {
  return apiFetch<void>(
    `/api/v1/work/projects/${projectId}/members/${personId}`,
    { method: "DELETE" },
  );
}

// ── work: milestones ────────────────────────────────────────────────────────

export function listMilestones(projectId: string): Promise<Milestone[]> {
  return apiFetch<{ items: Milestone[] }>(
    `/api/v1/work/projects/${projectId}/milestones`,
    { cache: "no-store" },
  ).then((page) => page.items);
}

export function createMilestone(
  projectId: string,
  body: MilestoneBody,
): Promise<Milestone> {
  return apiFetch<Milestone>(
    `/api/v1/work/projects/${projectId}/milestones`,
    { method: "POST", body },
  );
}

export function updateMilestone(
  id: string,
  body: MilestoneBody,
  version: number,
): Promise<Milestone> {
  return apiFetch<Milestone>(`/api/v1/work/milestones/${id}`, {
    method: "PUT",
    body,
    headers: ifMatch(version),
  });
}

// ── work: tasks ─────────────────────────────────────────────────────────────

export function listTasks(query: TaskListQuery = {}): Promise<Page<Task>> {
  return apiFetch<Page<Task>>("/api/v1/work/tasks", {
    query,
    cache: "no-store",
  });
}

export function getTask(id: string): Promise<Task> {
  return apiFetch<Task>(`/api/v1/work/tasks/${id}`, { cache: "no-store" });
}

export function createTask(projectId: string, body: TaskBody): Promise<Task> {
  return apiFetch<Task>(`/api/v1/work/projects/${projectId}/tasks`, {
    method: "POST",
    body,
  });
}

export function updateTask(
  id: string,
  body: TaskBody,
  version: number,
): Promise<Task> {
  return apiFetch<Task>(`/api/v1/work/tasks/${id}`, {
    method: "PUT",
    body,
    headers: ifMatch(version),
  });
}

// ── work: blockers ──────────────────────────────────────────────────────────

export function listBlockers(
  taskId: string,
  includeResolved = false,
): Promise<Blocker[]> {
  return apiFetch<{ items: Blocker[] }>(
    `/api/v1/work/tasks/${taskId}/blockers`,
    { query: { include_resolved: String(includeResolved) }, cache: "no-store" },
  ).then((page) => page.items);
}

export function raiseBlocker(taskId: string, reason: string): Promise<Blocker> {
  return apiFetch<Blocker>(`/api/v1/work/tasks/${taskId}/blockers`, {
    method: "POST",
    body: { reason },
  });
}

export function resolveBlocker(id: string): Promise<Blocker> {
  return apiFetch<Blocker>(`/api/v1/work/blockers/${id}/resolve`, {
    method: "POST",
  });
}

// ── work: evidence ──────────────────────────────────────────────────────────

export function listTaskEvidence(taskId: string): Promise<Evidence[]> {
  return apiFetch<{ items: Evidence[] }>(
    `/api/v1/work/tasks/${taskId}/evidence`,
    { cache: "no-store" },
  ).then((page) => page.items);
}

export function listProjectEvidence(projectId: string): Promise<Evidence[]> {
  return apiFetch<{ items: Evidence[] }>(
    `/api/v1/work/projects/${projectId}/evidence`,
    { cache: "no-store" },
  ).then((page) => page.items);
}

export function addTaskEvidence(
  taskId: string,
  body: EvidenceBody,
): Promise<Evidence> {
  return apiFetch<Evidence>(`/api/v1/work/tasks/${taskId}/evidence`, {
    method: "POST",
    body,
  });
}

export function addProjectEvidence(
  projectId: string,
  body: EvidenceBody,
): Promise<Evidence> {
  return apiFetch<Evidence>(`/api/v1/work/projects/${projectId}/evidence`, {
    method: "POST",
    body,
  });
}

export function verifyEvidence(id: string): Promise<Evidence> {
  return apiFetch<Evidence>(`/api/v1/work/evidence/${id}/verify`, {
    method: "POST",
  });
}

// ── work: escalations ───────────────────────────────────────────────────────

export function listEscalations(
  query: EscalationListQuery = {},
): Promise<Page<Escalation>> {
  return apiFetch<Page<Escalation>>("/api/v1/work/escalations", {
    query,
    cache: "no-store",
  });
}

export function resolveEscalation(id: string): Promise<Escalation> {
  return apiFetch<Escalation>(`/api/v1/work/escalations/${id}/resolve`, {
    method: "POST",
  });
}

// ── people: records ─────────────────────────────────────────────────────────

export type LeaveType =
  | "annual"
  | "sick"
  | "compassionate"
  | "maternity"
  | "paternity"
  | "unpaid"
  | "other";

export const LEAVE_TYPES: LeaveType[] = [
  "annual",
  "sick",
  "compassionate",
  "maternity",
  "paternity",
  "unpaid",
  "other",
];

export const LEAVE_STATUSES = [
  "requested",
  "approved",
  "declined",
  "cancelled",
] as const;

export interface DailyReport {
  id: string;
  reporter_person_id: string;
  report_date: string;
  status: string;
  work_done?: string;
  blockers?: string;
  risks?: string;
  next_day?: string;
  hours_worked?: number;
  submitted_at?: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface LeaveRequest {
  id: string;
  person_id: string;
  leave_type: LeaveType | string;
  from_date: string;
  to_date: string;
  reason?: string;
  status: string;
  approver_person_id?: string;
  approved_at?: string;
  declined_at?: string;
  cancel_reason?: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface AvailabilityItem {
  person_id: string;
  full_name: string;
  employment_type: string;
  department_id?: string;
  status: string;
  available: boolean;
  leave?: {
    id: string;
    leave_type: string;
    from_date: string;
    to_date: string;
  };
}

export interface Availability {
  date: string;
  items: AvailabilityItem[];
}

export interface OffboardResult {
  person_id: string;
  status: string;
  end_date?: string;
  version: number;
  account_id?: string;
  account_status?: string;
  sessions_revoked: number;
  roles_revoked: number;
}

export interface DailyReportBody {
  report_date?: string;
  work_done?: string;
  blockers?: string;
  risks?: string;
  next_day?: string;
  hours_worked?: number | null;
}

export interface LeaveRequestBody {
  leave_type: string;
  from_date: string;
  to_date: string;
  reason?: string;
}

// ── people: routes ──────────────────────────────────────────────────────────

export function getTodayReport(): Promise<DailyReport> {
  return apiFetch<DailyReport>("/api/v1/daily-reports/today", {
    cache: "no-store",
  });
}

export function listDailyReports(query: {
  status?: string;
  reporter_person_id?: string;
  from_date?: string;
  to_date?: string;
  limit?: number;
  cursor?: string;
} = {}): Promise<Page<DailyReport>> {
  return apiFetch<Page<DailyReport>>("/api/v1/daily-reports", {
    query,
    cache: "no-store",
  });
}

export function submitDailyReport(body: DailyReportBody): Promise<DailyReport> {
  return apiFetch<DailyReport>("/api/v1/daily-reports", {
    method: "POST",
    body,
  });
}

export function listLeaveRequests(query: {
  status?: string;
  person_id?: string;
  from_date?: string;
  to_date?: string;
  limit?: number;
  cursor?: string;
} = {}): Promise<Page<LeaveRequest>> {
  return apiFetch<Page<LeaveRequest>>("/api/v1/people/leave-requests", {
    query,
    cache: "no-store",
  });
}

export function requestLeave(body: LeaveRequestBody): Promise<LeaveRequest> {
  return apiFetch<LeaveRequest>("/api/v1/people/leave-requests", {
    method: "POST",
    body,
  });
}

export function decideLeave(
  id: string,
  decision: "approve" | "decline",
  reason?: string,
): Promise<LeaveRequest> {
  return apiFetch<LeaveRequest>(
    `/api/v1/people/leave-requests/${id}/decide`,
    { method: "POST", body: { decision, reason } },
  );
}

export function cancelLeave(id: string, reason?: string): Promise<LeaveRequest> {
  return apiFetch<LeaveRequest>(
    `/api/v1/people/leave-requests/${id}/cancel`,
    { method: "POST", body: { reason } },
  );
}

export function getAvailability(date?: string): Promise<Availability> {
  return apiFetch<Availability>("/api/v1/people/availability", {
    query: { date },
    cache: "no-store",
  });
}

export function offboardPerson(
  id: string,
  body: { reason?: string; last_working_day?: string },
): Promise<OffboardResult> {
  return apiFetch<OffboardResult>(`/api/v1/people/${id}/offboard`, {
    method: "POST",
    body,
  });
}

// ── people: session activity (§8.5) ─────────────────────────────────────────

/** One sign-in, derived read-only from a core.sessions row. */
export interface SignInSession {
  id: string;
  person_id: string;
  full_name: string;
  /** When the person signed in. */
  started_at: string;
  /** The last heartbeat the session was seen. */
  last_active_at: string;
  /** Set only when the session was explicitly ended (signed out / revoked). */
  ended_at?: string;
  expires_at: string;
  online: boolean;
  /** Span from sign-in to its effective end (or last heartbeat while live). */
  active_seconds: number;
  ip?: string;
  user_agent?: string;
}

/** The live roster (`GET /api/v1/people/presence`). */
export interface Presence {
  generated_at: string;
  online: SignInSession[];
}

export type SessionListQuery = {
  person_id?: string;
  /** Inclusive calendar day (UTC), `YYYY-MM-DD`. */
  from_date?: string;
  /** Inclusive calendar day (UTC), `YYYY-MM-DD`. */
  to_date?: string;
  limit?: number;
  cursor?: string;
};

export function getPresence(): Promise<Presence> {
  return apiFetch<Presence>("/api/v1/people/presence", { cache: "no-store" });
}

export function listSessions(
  query: SessionListQuery = {},
): Promise<Page<SignInSession>> {
  return apiFetch<Page<SignInSession>>("/api/v1/people/sessions", {
    query,
    cache: "no-store",
  });
}

// ── files (§6.5) - evidence attaches a stored file ──────────────────────────

export interface FileRecord {
  id: string;
  filename: string;
  content_type: string;
  size_bytes: number;
  classification: string;
  status: string;
  entity_type?: string;
  entity_id?: string;
  uploaded_by?: string;
  created_at: string;
  updated_at: string;
}

export interface UploadTicket {
  file_id: string;
  upload_url: string;
  method: string;
  headers: Record<string, string>;
  expires_at: string;
}

export interface DownloadTicket {
  url: string;
  expires_at: string;
}

export interface RequestUploadBody {
  filename: string;
  content_type: string;
  size_bytes: number;
  classification?: string;
  entity_type?: string;
  entity_id?: string;
  checksum_sha256?: string;
}

export function requestUpload(body: RequestUploadBody): Promise<UploadTicket> {
  return apiFetch<UploadTicket>("/api/v1/files", { method: "POST", body });
}

export function completeUpload(id: string): Promise<FileRecord> {
  return apiFetch<FileRecord>(`/api/v1/files/${id}/complete`, {
    method: "POST",
  });
}

export function downloadFile(id: string): Promise<DownloadTicket> {
  return apiFetch<DownloadTicket>(`/api/v1/files/${id}/download`, {
    cache: "no-store",
  });
}

/**
 * Upload bytes straight to object storage with the pre-signed ticket, then tell
 * Go the object is in place. Bytes never pass through the API (§6.5).
 */
export async function uploadFile(
  file: File,
  meta: { entity_type?: string; entity_id?: string; classification?: string } = {},
): Promise<FileRecord> {
  const ticket = await requestUpload({
    filename: file.name,
    content_type: file.type || "application/octet-stream",
    size_bytes: file.size,
    classification: meta.classification,
    entity_type: meta.entity_type,
    entity_id: meta.entity_id,
  });

  const response = await fetch(ticket.upload_url, {
    method: ticket.method || "PUT",
    headers: ticket.headers,
    body: file,
  });
  if (!response.ok) {
    throw new Error(`Upload failed (${response.status}).`);
  }

  return completeUpload(ticket.file_id);
}

// ── dashboard (§8.8) ────────────────────────────────────────────────────────

export interface PeopleSummary {
  total: number;
  by_status: Record<string, number>;
}

export interface ApprovalsSummary {
  pending: number;
  escalated: number;
  overdue: number;
  pending_by_role: Record<string, number>;
}

export interface WorkSummary {
  overdue: number;
  open_escalations: number;
}

export interface AuditHighlight {
  id: string;
  occurred_at: string;
  actor_type: string;
  actor_id: string;
  action: string;
  entity_type: string;
  entity_id?: string;
}

export interface ExecutiveSummary {
  generated_at: string;
  people: PeopleSummary;
  approvals: ApprovalsSummary;
  work: WorkSummary;
  recent_audit: AuditHighlight[];
}

export function getExecutiveSummary(): Promise<ExecutiveSummary> {
  return apiFetch<ExecutiveSummary>("/api/v1/dashboard/executive", {
    cache: "no-store",
  });
}

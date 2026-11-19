import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
  statusTone,
  type Tone,
} from "@/components/ui";
import {
  approveApproval,
  cancelApproval,
  declineApproval,
  escalateApproval,
  listApprovalPolicies,
  listMyRequestedApprovals,
  listPendingApprovals,
  requestApproval,
  type Approval,
  type ApprovalPolicy,
} from "@/lib/api/atlas";
import { requireUser } from "@/lib/auth/session";
import { formatDay, formatMinor } from "@/lib/format";
import { ApprovalsClient } from "./approvals-client";

/**
 * Approvals page (`GET /api/v1/approvals/pending`, `GET /api/v1/approvals/mine`,
 * POST /api/v1/approvals, POST /api/v1/approvals/{id}/{approve,decline,escalate,cancel};
 * blueprint §3.3, §6.3). Two tabs: approvals waiting for the viewer to decide,
 * and approvals the viewer has requested. Create new requests from the drawer.
 */
export const dynamic = "force-dynamic";

export default async function ApprovalsPage() {
  const user = await requireUser();

  // Reading the queue needs core.approval.read; a direct visit by someone
  // without it must still show their own requests, not a 500. Each read is
  // settled independently for that reason.
  const [pendingResult, requestedResult, policiesResult] = await Promise.allSettled([
    listPendingApprovals(user.roles[0] ? { role: user.roles[0] } : {}),
    listMyRequestedApprovals(),
    listApprovalPolicies(),
  ]);

  const canReadQueue = pendingResult.status === "fulfilled";
  const pending = canReadQueue ? pendingResult.value.items : [];
  const requested = requestedResult.status === "fulfilled" ? requestedResult.value.items : [];
  const policies = policiesResult.status === "fulfilled" ? policiesResult.value : [];

  return (
    <div className="space-y-6">
      <PageHeader
        title="Approvals"
        description="Decide approvals waiting for you, and track approvals you have requested."
      />

      <div className="flex flex-wrap items-center gap-4">
        <a
          href="/approvals?tab=pending"
          className={`flex flex-wrap items-center gap-4 ${"bg-navy text-white"}`}
        >
          Pending for you
        </a>
        <a
          href="/approvals?tab=requested"
          className="flex flex-wrap items-center gap-4"
        >
          Your requests
        </a>
      </div>

      <ApprovalsClient
        me={user.person_id ?? ""}
        pending={pending}
        requested={requested}
        policies={policies}
        canReadQueue={canReadQueue}
      />
    </div>
  );
}
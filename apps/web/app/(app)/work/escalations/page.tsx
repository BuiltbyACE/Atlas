import Link from "next/link";

import { Badge, Card, EmptyState, PageHeader, ProblemNotice, Table, Td, Th, cx } from "@/components/ui";
import { listEscalations, type Escalation } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory } from "@/lib/directory";
import { formatDateTime, humanise, shortId } from "@/lib/format";

import { ResolveEscalationButton } from "./resolve-escalation-button";

/**
 * Escalations (blueprint §5.8). The overdue sweep raises a level-1 escalation
 * for every task past its deadline; a resolver closes it with `work.task.review`.
 * L2/L3 rungs are decision D4 and not raised yet.
 */

export const dynamic = "force-dynamic";

export default async function EscalationsPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string }>;
}) {
  const { status } = await searchParams;
  const filter = status === "resolved" || status === "all" ? status : "open";

  let escalations: Escalation[] = [];
  let error: ApiError | null = null;
  try {
    const page = await listEscalations({
      status: filter === "all" ? undefined : filter,
      limit: 100,
    });
    escalations = page.items;
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  const names = error ? new Map<string, string>() : await personDirectory();

  return (
    <div className="space-y-6">
      <PageHeader
        title="Escalations"
        description="Work past its deadline that needs a human. Raised by the Go overdue sweep."
        actions={
          <Link href="/work" className="text-sm text-atlas hover:underline">
            Back to work
          </Link>
        }
      />

      <nav className="flex flex-wrap items-center gap-2 text-sm">
        {(["open", "resolved", "all"] as const).map((value) => (
          <Link
            key={value}
            href={value === "open" ? "/work/escalations" : `/work/escalations?status=${value}`}
            className={cx(
              "rounded-full px-3 py-1 text-xs font-medium ring-1 ring-inset",
              filter === value
                ? "bg-atlas text-white ring-atlas"
                : "bg-white text-slate-600 ring-slate-200 hover:bg-slate-50",
            )}
          >
            {humanise(value)}
          </Link>
        ))}
      </nav>

      {error ? (
        <ProblemNotice error={error} title="Escalations could not be loaded" />
      ) : (
        <Card padded={false}>
          {escalations.length === 0 ? (
            <div className="px-5 py-6">
              <EmptyState>
                No {filter === "all" ? "" : humanise(filter).toLowerCase()} escalations.
              </EmptyState>
            </div>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Entity</Th>
                  <Th>Level</Th>
                  <Th>Reason</Th>
                  <Th>Assignee</Th>
                  <Th>Raised</Th>
                  <Th>Status</Th>
                  <Th />
                </tr>
              </thead>
              <tbody>
                {escalations.map((item) => (
                  <tr key={item.id}>
                    <Td>
                      {item.entity_type === "task" ? (
                        <Link href={`/work/tasks/${item.entity_id}`} className="text-atlas hover:underline">
                          task {shortId(item.entity_id)}
                        </Link>
                      ) : (
                        <span>
                          {item.entity_type} {shortId(item.entity_id)}
                        </span>
                      )}
                    </Td>
                    <Td>
                      <Badge tone={item.level > 1 ? "danger" : "warning"}>L{item.level}</Badge>
                    </Td>
                    <Td className="text-slate-600">{item.reason || "—"}</Td>
                    <Td className="text-slate-600">
                      {item.assignee_person_id
                        ? (names.get(item.assignee_person_id) ?? shortId(item.assignee_person_id))
                        : "—"}
                    </Td>
                    <Td className="whitespace-nowrap text-slate-500">
                      {formatDateTime(item.created_at)}
                    </Td>
                    <Td>
                      <Badge tone={item.status === "resolved" ? "success" : "warning"}>
                        {item.status}
                      </Badge>
                    </Td>
                    <Td>
                      {item.status !== "resolved" && (
                        <ResolveEscalationButton id={item.id} />
                      )}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      )}
    </div>
  );
}

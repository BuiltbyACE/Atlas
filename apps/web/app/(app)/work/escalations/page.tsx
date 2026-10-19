import Link from "next/link";

import { ButtonLink, buttonClass } from "@/components/form";
import { Icon } from "@/components/icons";
import {
  Card,
  ChipLink,
  Chips,
  EmptyState,
  PageHeader,
  ProblemNotice,
} from "@/components/ui";
import { listEscalations, listTasks, type Escalation, type Page } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";

import { EscalationGroups, type EscalationGroup } from "./escalation-groups";

/**
 * Escalations (blueprint §5.8, ADR-016/017; prototype 05). The Go sweeps raise
 * them: the overdue sweep for tasks past their deadline (L1 → L2 → L3), the
 * daily-report sweep for people over the miss threshold (L2). They are grouped
 * by subject so one person or task reads as one line, with names in place of
 * ids wherever the roster and the task list allow.
 */

export const dynamic = "force-dynamic";

const FILTERS = ["open", "resolved", "all"] as const;
type Filter = (typeof FILTERS)[number];

function countLabel(page: Page<Escalation> | null): string | undefined {
  if (!page) return undefined;
  return page.has_more ? `${page.items.length}+` : String(page.items.length);
}

export default async function EscalationsPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string; cursor?: string }>;
}) {
  const { status, cursor } = await searchParams;
  const filter: Filter = status === "resolved" || status === "all" ? status : "open";

  let page: Page<Escalation> | null = null;
  let openPage: Page<Escalation> | null = null;
  let error: ApiError | null = null;
  try {
    page = await listEscalations({
      status: filter === "all" ? undefined : filter,
      limit: 100,
      cursor,
    });
    openPage =
      filter === "open" && !cursor
        ? page
        : await listEscalations({ status: "open", limit: 100 }).catch(() => null);
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  const escalations = page?.items ?? [];
  const needsTasks = escalations.some((item) => item.entity_type === "task");
  const [names, taskTitles] = error
    ? [new Map<string, string>(), new Map<string, string>()]
    : await Promise.all([
        personDirectory(),
        needsTasks
          ? listTasks({ limit: 200 })
              .then((tasks) => new Map(tasks.items.map((t) => [t.id, t.title])))
              .catch(() => new Map<string, string>())
          : Promise.resolve(new Map<string, string>()),
      ]);

  const groups = groupBySubject(escalations, names, taskTitles);
  const next = page?.has_more && page.next_cursor
    ? `/work/escalations?${new URLSearchParams({
        ...(filter === "open" ? {} : { status: filter }),
        cursor: page.next_cursor,
      }).toString()}`
    : null;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Escalations"
        description="Work past its deadline, and people who keep missing reports — anything that needs a human."
        actions={
          <ButtonLink href="/work" variant="secondary">
            <Icon name="arrowLeft" />
            Back to work
          </ButtonLink>
        }
      />

      <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
        <Chips label="Filter by status">
          {FILTERS.map((value) => (
            <ChipLink
              key={value}
              href={value === "open" ? "/work/escalations" : `/work/escalations?status=${value}`}
              pressed={filter === value}
              count={
                value === "open"
                  ? countLabel(openPage)
                  : value === filter
                    ? countLabel(page)
                    : undefined
              }
            >
              {value === "open" ? "Open" : value === "resolved" ? "Resolved" : "All"}
            </ChipLink>
          ))}
        </Chips>
        <span className="inline-flex items-center gap-2 text-sm text-muted">
          <Icon name="people" className="h-4 w-4" />
          Grouped by person or task, newest first
        </span>
      </div>

      {error ? (
        <ProblemNotice error={error} title="Escalations could not be loaded" />
      ) : (
        <Card padded={false}>
          {groups.length === 0 ? (
            <div className="p-[22px]">
              <EmptyState
                icon="check"
                className="min-h-[220px]"
                title={filter === "resolved" ? "Nothing resolved yet" : "No open escalations"}
              >
                {filter === "resolved"
                  ? "Escalations you resolve appear here with who resolved them and when."
                  : "Overdue work and missed reports land here when the sweeps raise them."}
              </EmptyState>
            </div>
          ) : (
            <>
              <EscalationGroups groups={groups} names={Object.fromEntries(names)} />
              <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-[22px] py-4 text-sm text-muted">
                <span>
                  {escalations.length} escalation{escalations.length === 1 ? "" : "s"} about{" "}
                  {groups.length} {groups.length === 1 ? "subject" : "subjects"}
                  {cursor ? " on this page" : ""}
                </span>
                <span className="flex gap-2">
                  {cursor && (
                    <Link
                      href={filter === "open" ? "/work/escalations" : `/work/escalations?status=${filter}`}
                      className={buttonClass("secondary", "sm")}
                    >
                      First page
                    </Link>
                  )}
                  {next && (
                    <Link href={next} className={buttonClass("secondary", "sm")}>
                      Next page
                    </Link>
                  )}
                </span>
              </div>
            </>
          )}
        </Card>
      )}
    </div>
  );
}

function groupBySubject(
  escalations: Escalation[],
  names: Map<string, string>,
  taskTitles: Map<string, string>,
): EscalationGroup[] {
  const groups = new Map<string, EscalationGroup>();
  for (const item of escalations) {
    const key = `${item.entity_type}:${item.entity_id}`;
    let group = groups.get(key);
    if (!group) {
      const isTask = item.entity_type === "task";
      const isPerson = item.entity_type === "person";
      group = {
        key,
        entityType: item.entity_type,
        entityId: item.entity_id,
        subject: isPerson
          ? personName(names, item.entity_id)
          : isTask
            ? (taskTitles.get(item.entity_id) ?? `Task ${item.entity_id.slice(0, 8)}`)
            : `${item.entity_type} ${item.entity_id.slice(0, 8)}`,
        href: isTask ? `/work/tasks/${item.entity_id}` : isPerson ? "/daily-reports" : undefined,
        items: [],
      };
      groups.set(key, group);
    }
    group.items.push(item);
  }
  for (const group of groups.values()) {
    group.items.sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
  }
  return [...groups.values()].sort((a, b) =>
    a.items[0].created_at < b.items[0].created_at ? 1 : -1,
  );
}

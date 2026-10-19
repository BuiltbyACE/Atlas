import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import {
  ArrowLink,
  Badge,
  ChipLink,
  Chips,
  EmptyState,
  IconTile,
  PageHeader,
  ProblemNotice,
  cx,
  statusTone,
  type Tone,
} from "@/components/ui";
import { listProjects, PROJECT_STATUSES, type Project } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { personDirectory, rosterOf } from "@/lib/directory";
import { daysFromToday, formatDay, humanise, initials } from "@/lib/format";

import { NewProjectButton } from "./new-project-form";

/**
 * Projects (`GET /api/v1/work/projects`, gated by `work.project.read`;
 * prototype 02). One page of up to 200 projects is read once and the status
 * chips and sort work over it, so each chip can show its count.
 */

export const dynamic = "force-dynamic";

const SORTS = {
  target: "Target date",
  name: "Name",
  status: "Status",
} as const;
type SortKey = keyof typeof SORTS;

const OPEN_STATUSES = new Set(["planning", "active", "paused"]);

/** Monogram colours by status (prototype `.m-*`). */
function monogramClass(status: string): string {
  if (status === "planning") return "bg-navy text-sky";
  if (status === "active") return "bg-atlas text-white";
  return "bg-[#E4E8F0] text-[#3B4A63]";
}

/**
 * The one line of risk a card calls out, if any. Presentation over Go's dates
 * and statuses; nothing is decided here.
 */
function riskOf(project: Project): { text: string; tone: Tone } | null {
  if (!OPEN_STATUSES.has(project.status) || !project.due_at) return null;
  const days = daysFromToday(project.due_at);
  if (days === null) return null;
  if (days < 0) {
    return {
      text: `Target passed ${Math.abs(days)} day${Math.abs(days) === 1 ? "" : "s"} ago`,
      tone: "danger",
    };
  }
  if (days <= 7 && project.status === "planning") {
    return {
      text: `Due ${days === 0 ? "today" : `in ${days} day${days === 1 ? "" : "s"}`} and still in Planning`,
      tone: "warning",
    };
  }
  return null;
}

function sortProjects(projects: Project[], sort: SortKey): Project[] {
  const list = [...projects];
  if (sort === "name") return list.sort((a, b) => a.name.localeCompare(b.name));
  if (sort === "status") {
    const order = PROJECT_STATUSES as string[];
    return list.sort((a, b) => order.indexOf(a.status) - order.indexOf(b.status));
  }
  return list.sort((a, b) => {
    const at = a.due_at ? new Date(a.due_at).getTime() : Number.MAX_SAFE_INTEGER;
    const bt = b.due_at ? new Date(b.due_at).getTime() : Number.MAX_SAFE_INTEGER;
    return at - bt;
  });
}

function href(status?: string, sort?: SortKey): string {
  const query = new URLSearchParams();
  if (status) query.set("status", status);
  if (sort && sort !== "target") query.set("sort", sort);
  const qs = query.toString();
  return qs ? `/work?${qs}` : "/work";
}

export default async function WorkPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string; sort?: string; new?: string }>;
}) {
  const { status, sort: rawSort, new: create } = await searchParams;
  const activeStatus = PROJECT_STATUSES.find((s) => s === status);
  const sort: SortKey = rawSort && rawSort in SORTS ? (rawSort as SortKey) : "target";
  const user = await requireUser();

  let all: Project[] = [];
  let error: ApiError | null = null;
  try {
    all = (await listProjects({ limit: 200 })).items;
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  const names = error ? new Map<string, string>() : await personDirectory();
  const people = rosterOf(names);
  const shown = sortProjects(
    activeStatus ? all.filter((p) => p.status === activeStatus) : all,
    sort,
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title="Projects"
        description="Every project has an owner, a deadline and evidence when it’s done."
        actions={
          <>
            <ButtonLink href="/work/tasks" variant="secondary">
              <Icon name="board" />
              Task board
            </ButtonLink>
            <NewProjectButton people={people} me={user.person_id} autoOpen={create === "project"} />
          </>
        }
      />

      {error ? (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view projects"
              : "Projects could not be loaded"
          }
        />
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
            <Chips label="Filter by status">
              <ChipLink href={href(undefined, sort)} pressed={!activeStatus} count={all.length}>
                All
              </ChipLink>
              {PROJECT_STATUSES.map((s) => (
                <ChipLink
                  key={s}
                  href={href(s, sort)}
                  pressed={activeStatus === s}
                  count={all.filter((p) => p.status === s).length}
                >
                  {humanise(s)}
                </ChipLink>
              ))}
            </Chips>
            <nav aria-label="Sort projects" className="flex items-center gap-1 text-sm text-muted">
              <span className="mr-1.5">Sort by</span>
              {(Object.keys(SORTS) as SortKey[]).map((key) => (
                <Link
                  key={key}
                  href={href(activeStatus, key)}
                  aria-current={sort === key ? "true" : undefined}
                  className={cx(
                    "rounded-[10px] px-2.5 py-1.5 font-medium transition",
                    sort === key ? "bg-white text-night shadow-tile" : "hover:text-night",
                  )}
                >
                  {SORTS[key]}
                </Link>
              ))}
            </nav>
          </div>

          {shown.length === 0 && activeStatus ? (
            <EmptyState
              icon="folder"
              iconTone="info"
              title={`No ${humanise(activeStatus).toLowerCase()} projects`}
              className="min-h-[240px]"
            >
              Projects move here when their status changes.
            </EmptyState>
          ) : (
            <div className="grid grid-cols-[repeat(auto-fill,minmax(min(380px,100%),1fr))] gap-5">
              {shown.map((project) => (
                <ProjectCard
                  key={project.id}
                  project={project}
                  owner={project.owner_person_id ? names.get(project.owner_person_id) : undefined}
                />
              ))}
              {!activeStatus && (
                <div className="flex min-h-[280px] flex-col items-start justify-center gap-2.5 rounded-card border-[1.5px] border-dashed border-[#BCC6D8] bg-white/55 p-6">
                  <IconTile icon="plus" tone="info" size="lg" />
                  <h3 className="mt-1.5 text-lg font-semibold">Start a project</h3>
                  <p className="mb-1.5 max-w-[34ch] text-sm text-muted">
                    Give it an owner, a target date and a type. It’s audited the moment it exists.
                  </p>
                  <NewProjectButton people={people} me={user.person_id} />
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function ProjectCard({ project, owner }: { project: Project; owner?: string }) {
  const risk = riskOf(project);
  const days = daysFromToday(project.due_at);
  const fix = `/work/projects/${project.id}#details`;

  return (
    <article className="flex flex-col gap-4 rounded-card border border-line bg-white p-[22px] shadow-card">
      <div className="flex items-start gap-3.5">
        <span
          aria-hidden="true"
          className={cx(
            "grid h-[50px] w-[50px] flex-none place-items-center rounded-[14px] text-base font-bold tracking-[0.03em]",
            monogramClass(project.status),
          )}
        >
          {initials(project.name)}
        </span>
        <div className="min-w-0 flex-1">
          <h3 className="mb-2 mt-0.5 text-[19px] font-semibold leading-tight">
            <Link href={`/work/projects/${project.id}`} className="hover:text-atlas-ink">
              {project.name}
            </Link>
          </h3>
          <Badge tone={statusTone(project.status)} dot>
            {humanise(project.status)}
          </Badge>
        </div>
        {project.ref && (
          <span className="whitespace-nowrap font-mono text-xs text-muted">{project.ref}</span>
        )}
      </div>

      {risk && (
        <p
          className={cx(
            "flex items-center gap-2.5 rounded-control px-3 py-2.5 text-[13.5px] font-medium",
            risk.tone === "danger" ? "bg-[#FDECEC] text-[#B42318]" : "bg-[#FFF6E5] text-[#8A4B00]",
          )}
        >
          <Icon name="clock" className="h-4 w-4" />
          {risk.text}
        </p>
      )}

      <dl className="grid grid-cols-3 gap-3.5 border-t border-line pt-4">
        <div>
          <dt className="mb-1.5 text-[11.5px] font-semibold uppercase tracking-[0.09em] text-muted">Owner</dt>
          <dd className="flex flex-col items-start gap-0.5 text-[14.5px]">
            {owner || project.owner_person_id ? (
              <span className="line-clamp-2">{owner ?? `Person ${project.owner_person_id!.slice(0, 8)}`}</span>
            ) : (
              <>
                <span className="font-semibold text-[#B45309]">No owner</span>
                <FixLink href={fix}>Assign owner</FixLink>
              </>
            )}
          </dd>
        </div>
        <div>
          <dt className="mb-1.5 text-[11.5px] font-semibold uppercase tracking-[0.09em] text-muted">Type</dt>
          <dd className="flex flex-col items-start gap-0.5 text-[14.5px]">
            {project.project_type ? (
              <span className="line-clamp-2">{humanise(project.project_type)}</span>
            ) : (
              <>
                <span className="text-muted">Not set</span>
                <FixLink href={fix}>Set type</FixLink>
              </>
            )}
          </dd>
        </div>
        <div>
          <dt className="mb-1.5 text-[11.5px] font-semibold uppercase tracking-[0.09em] text-muted">Target</dt>
          <dd className="flex flex-col items-start gap-0.5 text-[14.5px]">
            {project.due_at ? (
              <>
                <span>{formatDay(project.due_at)}</span>
                {days !== null && OPEN_STATUSES.has(project.status) && (
                  <span className="text-[13px] text-muted">
                    {days === 0 ? "today" : days > 0 ? `in ${days} day${days === 1 ? "" : "s"}` : `${Math.abs(days)}d ago`}
                  </span>
                )}
              </>
            ) : (
              <>
                <span className="text-muted">No date</span>
                <FixLink href={fix}>Set date</FixLink>
              </>
            )}
          </dd>
        </div>
      </dl>

      <div className="flex items-center justify-between gap-2.5 border-t border-line pt-3.5">
        <ArrowLink href={`/work/tasks?project_id=${project.id}`}>Open task board</ArrowLink>
        <Link
          href={`/work/projects/${project.id}`}
          className="text-[13.5px] font-medium text-muted hover:text-night"
        >
          Project details
        </Link>
      </div>
    </article>
  );
}

function FixLink({ href, children }: { href: string; children: string }) {
  return (
    <Link
      href={href}
      className="min-h-7 text-[13.5px] font-semibold text-atlas-ink underline underline-offset-[3px]"
    >
      {children}
    </Link>
  );
}

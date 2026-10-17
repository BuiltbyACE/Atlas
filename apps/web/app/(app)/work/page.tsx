import Link from "next/link";

import { Badge, Card, EmptyState, PageHeader, ProblemNotice, Table, Td, Th, statusTone, cx } from "@/components/ui";
import { listProjects, PROJECT_STATUSES, type Project } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory } from "@/lib/directory";
import { formatDay, humanise } from "@/lib/format";

import { NewProjectForm } from "./new-project-form";

/**
 * Projects list (`GET /api/v1/work/projects`, gated by `work.project.read`).
 * Keyset-paged by Go; the status filter is a plain query param so the view stays
 * a server component.
 */

export const dynamic = "force-dynamic";

export default async function WorkPage({
  searchParams,
}: {
  searchParams: Promise<{ status?: string }>;
}) {
  const { status } = await searchParams;
  const activeStatus = PROJECT_STATUSES.find((s) => s === status);

  let page;
  let error: ApiError | null = null;
  try {
    page = await listProjects({ status: activeStatus, limit: 50 });
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  const names = error ? new Map<string, string>() : await personDirectory();

  return (
    <div className="space-y-6">
      <PageHeader
        title="Work"
        description="Every project has an owner, a deadline and evidence when done (blueprint §8.2)."
        actions={
          <Link
            href="/work/tasks"
            className="rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50"
          >
            Task board
          </Link>
        }
      />

      <nav className="flex flex-wrap items-center gap-2 text-sm">
        <StatusFilterLink status={activeStatus} active={!activeStatus} />
        {PROJECT_STATUSES.map((s) => (
          <StatusFilterLink key={s} status={s} active={activeStatus === s} />
        ))}
      </nav>

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
        <Card padded={false}>
          {page!.items.length === 0 ? (
            <div className="px-5 py-6">
              <EmptyState>
                No projects{activeStatus ? ` with status “${humanise(activeStatus)}”` : ""} yet.
              </EmptyState>
            </div>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Project</Th>
                  <Th>Status</Th>
                  <Th>Type</Th>
                  <Th>Owner</Th>
                  <Th>Target</Th>
                </tr>
              </thead>
              <tbody>
                {page!.items.map((project) => (
                  <ProjectRow
                    key={project.id}
                    project={project}
                    owner={
                      project.owner_person_id
                        ? (names.get(project.owner_person_id) ?? null)
                        : null
                    }
                  />
                ))}
              </tbody>
            </Table>
          )}
        </Card>
      )}

      <Card title="Start a project" description="Owned and audited by Go the moment it exists.">
        <NewProjectForm />
      </Card>
    </div>
  );
}

function ProjectRow({
  project,
  owner,
}: {
  project: Project;
  owner: string | null;
}) {
  return (
    <tr className="hover:bg-slate-50">
      <Td>
        <Link
          href={`/work/projects/${project.id}`}
          className="font-medium text-slate-900 hover:text-atlas"
        >
          {project.name}
        </Link>
        {project.description && (
          <p className="mt-0.5 line-clamp-1 text-xs text-slate-500">
            {project.description}
          </p>
        )}
      </Td>
      <Td>
        <Badge tone={statusTone(project.status)}>{humanise(project.status)}</Badge>
      </Td>
      <Td className="text-slate-600">
        {project.project_type ? humanise(project.project_type) : "—"}
      </Td>
      <Td className="text-slate-600">
        {owner ?? (project.owner_person_id ? project.owner_person_id.slice(0, 8) : "—")}
      </Td>
      <Td className="whitespace-nowrap text-slate-500">{formatDay(project.due_at)}</Td>
    </tr>
  );
}

function StatusFilterLink({
  status,
  active,
}: {
  status?: string;
  active: boolean;
}) {
  return (
    <Link
      href={status ? `/work?status=${status}` : "/work"}
      className={cx(
        "rounded-full px-3 py-1 text-xs font-medium ring-1 ring-inset",
        active
          ? "bg-atlas text-white ring-atlas"
          : "bg-white text-slate-600 ring-slate-200 hover:bg-slate-50",
      )}
    >
      {status ? humanise(status) : "All"}
    </Link>
  );
}

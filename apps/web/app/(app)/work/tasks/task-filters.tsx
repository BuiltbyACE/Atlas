"use client";

import { useRouter } from "next/navigation";

import { Field } from "@/components/form";

/**
 * Board filters. They are plain query params (`project_id`,
 * `assignee_person_id`, `overdue`) so the board itself stays a server component;
 * this client shell just rewrites the URL.
 */
export function TaskFilters({
  projects,
  projectId,
  assigneeId,
  overdue,
}: {
  projects: Array<{ id: string; name: string }>;
  projectId: string;
  assigneeId: string;
  overdue: boolean;
}) {
  const router = useRouter();

  function apply(params: Record<string, string | undefined>) {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
      if (value) query.set(key, value);
    }
    const qs = query.toString();
    router.push(qs ? `/work/tasks?${qs}` : "/work/tasks");
  }

  return (
    <form
      className="flex flex-wrap items-end gap-3 rounded-xl border border-slate-200 bg-white p-4 shadow-sm"
      onSubmit={(event) => {
        event.preventDefault();
        const data = new FormData(event.currentTarget);
        apply({
          project_id: String(data.get("project_id") ?? ""),
          assignee_person_id: String(data.get("assignee_person_id") ?? ""),
          overdue: data.get("overdue") ? "true" : undefined,
        });
      }}
    >
      <div className="min-w-[14rem] flex-1">
        <Field label="Project" htmlFor="filter-project">
          <select
            id="filter-project"
            name="project_id"
            defaultValue={projectId}
            className="w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm shadow-sm outline-none focus:border-atlas focus:ring-2 focus:ring-atlas/20"
          >
            <option value="">All projects</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        </Field>
      </div>

      <div className="min-w-[14rem] flex-1">
        <Field label="Assignee" hint="person UUID" htmlFor="filter-assignee">
          <input
            id="filter-assignee"
            name="assignee_person_id"
            defaultValue={assigneeId}
            placeholder="anyone"
            className="w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm shadow-sm outline-none placeholder:text-slate-400 focus:border-atlas focus:ring-2 focus:ring-atlas/20"
          />
        </Field>
      </div>

      <label className="flex items-center gap-2 pb-2 text-sm text-slate-700">
        <input
          type="checkbox"
          name="overdue"
          defaultChecked={overdue}
          className="h-4 w-4 rounded border-slate-300 text-atlas focus:ring-atlas"
        />
        Overdue only
      </label>

      <div className="flex items-center gap-2 pb-0.5">
        <button
          type="submit"
          className="rounded-md bg-atlas px-3 py-2 text-sm font-medium text-white transition hover:bg-atlas/90"
        >
          Filter
        </button>
        <button
          type="button"
          onClick={() => router.push("/work/tasks")}
          className="rounded-md px-3 py-2 text-sm font-medium text-slate-600 transition hover:bg-slate-100"
        >
          Clear
        </button>
      </div>
    </form>
  );
}

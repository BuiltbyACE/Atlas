"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useEffect } from "react";

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
  type Tone,
} from "@/components/ui";
import { Button, Field, TextInput } from "@/components/form";
import { useToast } from "@/components/toast";
import {
  createDepartment,
  listDepartments,
  type Department,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { humanise } from "@/lib/format";

/**
 * Departments (§8.5). List departments and create new ones. Go owns every
 * rule; the page renders what Go returns and routes refusals through
 * ProblemNotice.
 */
export const dynamic = "force-dynamic";

export default async function DepartmentsPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [depts, setDepts] = useState<Department[]>([]);
  const [error, setError] = useState<ApiError | null>(null);

  useEffect(() => {
    listDepartments({ limit: 200 }).then(
      (page) => setDepts(page.items),
      (err) => {
        if (err instanceof ApiError) setError(err);
        else throw err;
      },
    );
  }, []);

  const deptFilter = searchParams.get("q") ?? "";
  const filtered = depts.filter((d) =>
    d.name.toLowerCase().includes(deptFilter.toLowerCase()),
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title="Departments"
        description="Business units that people belong to. Departments help organise people and their work."
        actions={
          <>
            <Button
              type="button"
              variant="primary"
              size="sm"
              onClick={() => router.push("/people/departments/new")}
            >
              New department
            </Button>
          </>
        }
      />

      {error && (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view departments"
              : "Departments could not be loaded"
          }
        />
      )}

      <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
        <input
          type="text"
          placeholder="Filter departments by name…"
          value={deptFilter}
          onChange={(e) => {
            const qs = new URLSearchParams(searchParams);
            qs.set("q", e.target.value);
            router.push(`/people/departments?${qs.toString()}`);
          }}
          className="rounded-control border border-line-strong bg-white px-3 text-[14.5px] text-night outline-none transition focus:border-atlas focus:shadow-[0_0_0_3px_rgba(26,108,245,.15)] h-11 w-[240px]"
        />
      </div>

      <Card padded={false}>
        {filtered.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="building"
              iconTone="info"
              className="min-h-[220px]"
              title={deptFilter ? "No departments match" : "No departments yet"}
            >
              {deptFilter
                ? "Try a different search term."
                : "Create the first department to start grouping people."}
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Department</Th>
                <Th>Ref</Th>
                <Th>Members</Th>
                <Th>Created</Th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((d) => (
                <tr key={d.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <span className="font-medium">{d.name}</span>
                    </div>
                  </Td>
                  <Td className="tabular text-muted text-[13px]">
                    {d.id.slice(0, 8)}
                  </Td>
                  <Td className="tabular text-right">
                    <span className="text-muted">—</span>
                  </Td>
                  <Td className="tabular text-muted">
                    {d.created_at ? new Date(d.created_at).toLocaleDateString() : "—"}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}

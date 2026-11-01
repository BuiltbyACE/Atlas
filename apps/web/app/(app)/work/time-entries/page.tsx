"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useEffect, useCallback } from "react";

import {
  Button,
  ButtonLink,
  Field,
  Select,
  TextInput,
  TextArea,
} from "@/components/form";
import { Icon } from "@/components/icons";
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
} from "@/components/ui";
import { useToast } from "@/components/toast";
import {
  createTimeEntry,
  deleteTimeEntry,
  getTimeEntry,
  listTimeEntries,
  updateTimeEntry,
  type TimeEntry,
  type TimeEntryBody,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * My Time Entries (§8.2, decision D7). Log minutes worked against a task,
 * review your timesheet, and edit or delete entries you created.
 */
export const dynamic = "force-dynamic";

export default async function TimeEntriesPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [entries, setEntries] = useState<TimeEntry[]>([]);
  const [error, setError] = useState<ApiError | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);

  const taskFilter = searchParams.get("task_id") ?? "";
  const personFilter = searchParams.get("person_id") ?? "";

  useEffect(() => {
    const query: Record<string, string> = { limit: "200" };
    if (taskFilter) query.task_id = taskFilter;
    if (personFilter) query.person_id = personFilter;
    listTimeEntries(query).then(
      (page) => setEntries(page.items),
      (err) => {
        if (err instanceof ApiError) setError(err);
        else throw err;
      },
    );
  }, [taskFilter, personFilter]);

  const names = await personDirectory();

  const totalMinutes = entries.reduce((sum, e) => sum + e.minutes, 0);
  const totalHours = (totalMinutes / 60).toFixed(1);

  const { busy, problem, run, reset } = useMutation();

  async function onSave(entry: TimeEntryBody) {
    if (editingId) {
      const updated = await run(() =>
        updateTimeEntry(editingId, getEntryVersion(editingId), entry),
      );
      if (updated) {
        setEditingId(null);
        reset();
        router.refresh();
      }
    } else {
      const created = await run(() => createTimeEntry(entry));
      if (created) {
        reset();
        router.refresh();
      }
    }
  }

  async function onDelete(id: string, version: number) {
    if (!window.confirm("Delete this time entry?")) return;
    const deleted = await run(() => deleteTimeEntry(id, version));
    if (deleted) {
      reset();
      router.refresh();
    }
  }

  // Helper: find a stored entry's version for optimistic concurrency
  function getEntryVersion(id: string): number {
    const entry = entries.find((e) => e.id === id);
    return entry ? entry.version : 0;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="My Time Entries"
        description={`Log minutes worked against tasks. ${totalHours} hours this period.`}
        actions={
          <>
            <Button
              type="button"
              variant="primary"
              size="sm"
              onClick={() => setEditingId(null)}
            >
              <Icon name="plus" className="h-4 w-4" />
              Log time
            </Button>
          </>
        }
      />

      {error && (
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view time entries"
              : "Time entries could not be loaded"
          }
        />
      )}

      <StatGrid label="Timesheet summary">
        <Stat
          label="Total hours"
          value={totalHours}
          icon="clock"
          iconTone="info"
          hint={`${entries.length} entr${entries.length === 1 ? "y" : "ies"}`}
        />
        <Stat
          label="Entries"
          value={entries.length.toString()}
          icon="document"
          iconTone="neutral"
          hint="Time logged against tasks"
        />
      </StatGrid>

      <div className="flex flex-wrap items-center justify-between gap-x-5 gap-y-3">
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => {
              const qs = new URLSearchParams(searchParams);
              qs.delete("task_id");
              qs.delete("person_id");
              router.push(`/work/time-entries?${qs.toString()}`);
            }}
          >
            All entries
          </Button>
        </div>
      </div>

      {editingId ? (
        <TimeEntryForm
          initial={entries.find((e) => e.id === editingId) ?? undefined}
          onSave={onSave}
          onCancel={() => setEditingId(null)}
        />
      ) : (
        <Card padded={false}>
          {entries.length === 0 ? (
            <div className="p-[22px]">
              <EmptyState
                icon="clock"
                iconTone="info"
                className="min-h-[220px]"
                title="No time entries yet"
              >
                Log your first entry to start tracking time against tasks.
              </EmptyState>
            </div>
          ) : (
            <Table>
              <thead>
                <tr>
                  <Th>Date</Th>
                  <Th>Task</Th>
                  <Th className="text-right">Minutes</Th>
                  <Th className="text-right">Hours</Th>
                  <Th>Note</Th>
                  <Th className="text-right">Actions</Th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e) => (
                  <tr key={e.id}>
                    <Td className="tabular text-muted">
                      {formatDay(e.entry_date)}
                    </Td>
                    <Td className="tabular text-muted font-mono text-[13px]">
                      {e.task_id.slice(0, 8)}
                    </Td>
                    <Td className="tabular text-right">{e.minutes}</Td>
                    <Td className="tabular text-right">
                      {(e.minutes / 60).toFixed(1)}
                    </Td>
                    <Td className="tabular text-muted">
                      {e.note ?? "—"}
                    </Td>
                    <Td className="tabular text-right">
                      <Button
                        type="button"
                        variant="secondary"
                        size="sm"
                        onClick={() => setEditingId(e.id)}
                      >
                        Edit
                      </Button>
                      <Button
                        type="button"
                        variant="danger"
                        size="sm"
                        onClick={() => onDelete(e.id, e.version)}
                      >
                        Delete
                      </Button>
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

function TimeEntryForm({
  initial,
  onSave,
  onCancel,
}: {
  initial?: TimeEntry;
  onSave: (body: { task_id: string; entry_date: string; minutes: number; note?: string }) => void;
  onCancel: () => void;
}) {
  const [taskId, setTaskId] = useState(initial?.task_id ?? "");
  const [entryDate, setEntryDate] = useState(initial?.entry_date ?? "");
  const [minutes, setMinutes] = useState(initial?.minutes?.toString() ?? "");
  const [note, setNote] = useState(initial?.note ?? "");

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    onSave({
      task_id: taskId,
      entry_date: entryDate,
      minutes: parseInt(minutes, 10) || 0,
      note: note || undefined,
    });
  }

  return (
    <Card title="Log time" description="Record minutes worked against a task.">
      <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <Field label="Task ID" htmlFor="time-task-id">
            <TextInput
              id="time-task-id"
              name="task_id"
              value={taskId}
              onChange={(e) => setTaskId(e.target.value)}
              placeholder="Task UUID"
              data-autofocus
            />
          </Field>
          <Field label="Date" htmlFor="time-entry-date">
            <TextInput
              id="time-entry-date"
              name="entry_date"
              type="date"
              value={entryDate}
              onChange={(e) => setEntryDate(e.target.value)}
            />
          </Field>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <Field label="Minutes" htmlFor="time-minutes">
            <TextInput
              id="time-minutes"
              name="minutes"
              type="number"
              min="1"
              max="1440"
              value={minutes}
              onChange={(e) => setMinutes(e.target.value)}
            />
          </Field>
        </div>
        <Field label="Note (optional)" htmlFor="time-note">
          <TextArea
            id="time-note"
            name="note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            rows={3}
            placeholder="What did you work on?"
          />
        </Field>
        <div className="flex justify-end gap-2 pt-4">
          <Button type="button" variant="secondary" onClick={onCancel}>
            Cancel
          </Button>
          <Button type="submit" variant="primary">
            {initial ? "Save entry" : "Log time"}
          </Button>
        </div>
      </form>
    </Card>
  );
}

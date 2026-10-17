"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Badge, EmptyState, ProblemNotice, Table, Td, Th, statusTone } from "@/components/ui";
import {
  cancelLeave,
  decideLeave,
  requestLeave,
  LEAVE_TYPES,
  type LeaveRequest,
} from "@/lib/api/atlas";
import { formatDay, humanise, shortId, today } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * Leave requests (blueprint §8.5): file one, and decide or cancel the ones in
 * the queue. Go enforces who may do what - requesting and cancelling your own
 * leave needs `people.leave.request`, deciding needs `people.leave.manage` - and
 * a refusal comes back as a problem, not a silent failure.
 */
export function LeavePanel({ requests }: { requests: LeaveRequest[] }) {
  const router = useRouter();
  const { busy, problem, run } = useMutation();
  const [open, setOpen] = useState(false);
  const [actingOn, setActingOn] = useState<string | null>(null);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);

    const created = await run(() =>
      requestLeave({
        leave_type: String(data.get("leave_type") ?? "annual"),
        from_date: String(data.get("from_date") ?? "").trim(),
        to_date: String(data.get("to_date") ?? "").trim(),
        reason: String(data.get("reason") ?? ""),
      }),
    );

    if (created) {
      form.reset();
      setOpen(false);
      router.refresh();
    }
  }

  async function decide(id: string, decision: "approve" | "decline") {
    setActingOn(id);
    const result = await run(() => decideLeave(id, decision));
    setActingOn(null);
    if (result) router.refresh();
  }

  async function cancel(id: string) {
    setActingOn(id);
    const result = await run(() => cancelLeave(id));
    setActingOn(null);
    if (result) router.refresh();
  }

  return (
    <div className="space-y-4">
      {problem && <ProblemNotice error={problem} title="Leave change failed" />}

      {open ? (
        <form onSubmit={onSubmit} className="space-y-3 rounded-lg border border-slate-200 p-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Field label="Type" htmlFor="leave-type">
              <Select id="leave-type" name="leave_type" defaultValue="annual">
                {LEAVE_TYPES.map((type) => (
                  <option key={type} value={type}>
                    {humanise(type)}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="From" htmlFor="leave-from">
              <TextInput id="leave-from" name="from_date" type="date" required defaultValue={today()} />
            </Field>
            <Field label="To" htmlFor="leave-to">
              <TextInput id="leave-to" name="to_date" type="date" required defaultValue={today()} />
            </Field>
          </div>
          <Field label="Reason" hint="optional" htmlFor="leave-reason">
            <TextArea id="leave-reason" name="reason" rows={2} />
          </Field>
          <div className="flex items-center gap-2">
            <Button type="submit" disabled={busy}>
              {busy ? "Requesting…" : "Request leave"}
            </Button>
            <Button type="button" variant="secondary" onClick={() => setOpen(false)} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      ) : (
        <Button onClick={() => setOpen(true)}>Request leave</Button>
      )}

      {requests.length === 0 ? (
        <EmptyState>No leave requests.</EmptyState>
      ) : (
        <Table>
          <thead>
            <tr>
              <Th>Person</Th>
              <Th>Type</Th>
              <Th>Dates</Th>
              <Th>Status</Th>
              <Th>Reason</Th>
              <Th />
            </tr>
          </thead>
          <tbody>
            {requests.map((leave) => {
              const requested = leave.status === "requested";
              const cancellable =
                leave.status === "requested" || leave.status === "approved";
              const working = busy && actingOn === leave.id;
              return (
                <tr key={leave.id}>
                  <Td className="text-slate-600">{shortId(leave.person_id)}</Td>
                  <Td>
                    <Badge tone="neutral">{humanise(leave.leave_type)}</Badge>
                  </Td>
                  <Td className="whitespace-nowrap text-slate-600">
                    {formatDay(leave.from_date)} → {formatDay(leave.to_date)}
                  </Td>
                  <Td>
                    <Badge tone={statusTone(leave.status)}>{humanise(leave.status)}</Badge>
                  </Td>
                  <Td className="max-w-xs text-slate-500">
                    {leave.reason || leave.cancel_reason || "—"}
                  </Td>
                  <Td>
                    <div className="flex items-center gap-2">
                      {requested && (
                        <>
                          <Button
                            variant="secondary"
                            disabled={working}
                            onClick={() => decide(leave.id, "approve")}
                          >
                            Approve
                          </Button>
                          <Button
                            variant="danger"
                            disabled={working}
                            onClick={() => decide(leave.id, "decline")}
                          >
                            Decline
                          </Button>
                        </>
                      )}
                      {cancellable && (
                        <Button
                          variant="ghost"
                          disabled={working}
                          onClick={() => cancel(leave.id)}
                        >
                          Cancel
                        </Button>
                      )}
                    </div>
                  </Td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      )}
    </div>
  );
}

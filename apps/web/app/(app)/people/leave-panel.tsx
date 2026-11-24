"use client";

import { usePathname, useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import { Button, Field, Select, TextArea, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { Avatar, Badge, EmptyState, ProblemNotice, statusTone } from "@/components/ui";
import {
  cancelLeave,
  decideLeave,
  requestLeave,
  LEAVE_TYPES,
  type LeaveRequest,
} from "@/lib/api/atlas";
import {
  daySpan,
  formatShortDay,
  initials,
  leaveTypeLabel,
  statusLabel,
  timeAgo,
  today,
} from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * The "Request leave" button and drawer (prototype 06). Requesting needs
 * `people.leave.request`; Go rejects an overlap with leave already booked and
 * a last day before the first, and says so.
 */
export function RequestLeaveButton({
  autoOpen = false,
  variant = "primary",
}: {
  autoOpen?: boolean;
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const pathname = usePathname();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(autoOpen);
  const [from, setFrom] = useState(today());

  function close() {
    setOpen(false);
    reset();
    if (autoOpen) router.replace(pathname);
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(() =>
      requestLeave({
        leave_type: String(data.get("leave_type") ?? "annual"),
        from_date: String(data.get("from_date") ?? "").trim(),
        to_date: String(data.get("to_date") ?? "").trim(),
        reason: String(data.get("reason") ?? "").trim(),
      }),
    );
    if (created) {
      setOpen(false);
      notify("Leave request sent for approval");
      router.refresh();
    }
  }

  return (
    <>
      <Button variant={variant} size={variant === "secondary" ? "sm" : "md"} onClick={() => setOpen(true)}>
        <Icon name="calendarPlus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        Request leave
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="Request leave"
        description="Your request goes to an approver. Approved leave also excuses the daily reports for those days."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The request wasn’t sent" />}
          <Field label="Type of leave" htmlFor="leave-type">
            <Select id="leave-type" name="leave_type" defaultValue="annual" data-autofocus>
              {LEAVE_TYPES.map((type) => (
                <option key={type} value={type}>
                  {leaveTypeLabel(type)}
                </option>
              ))}
            </Select>
          </Field>
          <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
            <Field label="First day" htmlFor="leave-from">
              <TextInput
                id="leave-from"
                name="from_date"
                type="date"
                required
                value={from}
                onChange={(event) => setFrom(event.target.value)}
              />
            </Field>
            <Field label="Last day" htmlFor="leave-to">
              <TextInput id="leave-to" name="to_date" type="date" required min={from} defaultValue={from} key={from} />
            </Field>
          </div>
          <Field label="Note" hint="optional" htmlFor="leave-reason">
            <TextArea
              id="leave-reason"
              name="reason"
              rows={3}
              placeholder="Anything your approver should know, like who covers your work."
            />
          </Field>
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Sending…" : "Send request"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </>
  );
}

/**
 * The leave queue (blueprint §8.5) as request cards. Go enforces who may do
 * what - requesting and withdrawing your own leave needs `people.leave.request`,
 * deciding needs `people.leave.manage`, and nobody decides their own request -
 * so the card only offers the actions that could apply and a refusal is shown
 * as Go gave it.
 */
export function LeaveQueue({
  requests,
  names,
  me,
}: {
  requests: LeaveRequest[];
  names: Record<string, string>;
  me?: string;
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run } = useMutation();
  const [actingOn, setActingOn] = useState<string | null>(null);

  async function act(id: string, action: () => Promise<unknown>, message: string) {
    setActingOn(id);
    const result = await run(action);
    setActingOn(null);
    if (result) {
      notify(message);
      router.refresh();
    }
  }

  if (requests.length === 0) {
    return (
      <EmptyState icon="check" title="Nothing waiting for a decision" action={<RequestLeaveButton variant="secondary" />}>
        New requests land here for the approver.
      </EmptyState>
    );
  }

  return (
    <div className="flex flex-col gap-2.5">
      {problem && <ProblemNotice error={problem} title="That didn’t go through" />}
      <ul className="flex flex-col gap-2.5">
        {requests.map((leave) => {
          const mine = Boolean(me && leave.person_id === me);
          const name = mine ? "You" : (names[leave.person_id] ?? `Person ${leave.person_id.slice(0, 8)}`);
          const pending = leave.status === "requested";
          const withdrawable = mine && (pending || leave.status === "approved");
          const days = daySpan(leave.from_date, leave.to_date);
          const working = busy && actingOn === leave.id;
          return (
            <li key={leave.id} className="flex flex-col gap-2.5 rounded-[14px] border border-line p-3.5">
              <div className="flex items-center gap-2.5">
                <Avatar initials={initials(mine ? (names[leave.person_id] ?? "You") : name)} variant={mine ? "me" : "default"} />
                <span className="min-w-0">
                  <b className="block truncate text-[14.5px] font-semibold">{name}</b>
                  <small className="block text-[13px] text-muted">{leaveTypeLabel(leave.leave_type)}</small>
                </span>
                <span className="ml-auto">
                  <Badge tone={statusTone(leave.status)} dot>
                    {statusLabel(leave.status)}
                  </Badge>
                </span>
              </div>
              <span className="text-[14.5px] font-semibold">
                {leave.from_date === leave.to_date
                  ? formatShortDay(leave.from_date)
                  : `${formatShortDay(leave.from_date)} – ${formatShortDay(leave.to_date)}`}{" "}
                · {days} day{days === 1 ? "" : "s"}
              </span>
              {(leave.reason || leave.cancel_reason) && (
                <p className="text-[13.5px] text-muted">“{leave.reason || leave.cancel_reason}”</p>
              )}
              <div className="flex flex-wrap items-center justify-between gap-2.5 border-t border-dashed border-line-strong pt-2.5 text-[13px] text-muted">
                <span>
                  {pending && mine
                    ? "Waiting for an approver — you can’t approve your own request."
                    : pending
                      ? `Requested ${timeAgo(leave.created_at)}`
                      : leave.approved_at
                        ? `Approved ${timeAgo(leave.approved_at)}`
                        : leave.declined_at
                          ? `Declined ${timeAgo(leave.declined_at)}`
                          : `Updated ${timeAgo(leave.updated_at)}`}
                </span>
                <span className="flex gap-2">
                  {pending && !mine && (
                    <>
                      <Button
                        size="sm"
                        disabled={working}
                        onClick={() => act(leave.id, () => decideLeave(leave.id, "approve"), `Approved leave for ${name}`)}
                      >
                        Approve
                      </Button>
                      <Button
                        size="sm"
                        variant="danger"
                        disabled={working}
                        onClick={() => act(leave.id, () => decideLeave(leave.id, "decline"), `Declined leave for ${name}`)}
                      >
                        Decline
                      </Button>
                    </>
                  )}
                  {withdrawable && (
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={working}
                      onClick={() => act(leave.id, () => cancelLeave(leave.id), "Leave request withdrawn")}
                    >
                      Withdraw
                    </Button>
                  )}
                </span>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

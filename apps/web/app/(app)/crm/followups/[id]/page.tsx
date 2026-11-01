"use client";

import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  PageHeader,
  ProblemNotice,
  Table,
  Td,
  Th,
} from "@/components/ui";
import {
  completeFollowup,
  cancelFollowup,
  getFollowup,
  type Followup,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { personDirectory, personName } from "@/lib/directory";
import { formatDay, formatDateTime, humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

/**
 * Follow-up detail. Read one scheduled follow-up and complete or cancel it.
 */
export const dynamic = "force-dynamic";

export default async function FollowupDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const { busy, problem, run, reset } = useMutation();

  let followup: Followup;
  try {
    followup = await getFollowup(id);
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    return (
      <div className="space-y-6">
        <PageHeader title="Follow-up" />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That follow-up does not exist"
              : "The follow-up could not be loaded"
          }
        />
      </div>
    );
  }

  const names = await personDirectory();

  const isOpen =
    followup.status !== "done" &&
    followup.status !== "missed" &&
    followup.status !== "cancelled";

  async function onComplete(outcome: string) {
    const updated = await run(async () => {
      return completeFollowup(followup.id, followup.version, outcome);
    });
    if (updated) {
      reset();
    }
  }

  async function onCancel(outcome: string) {
    const updated = await run(async () => {
      return cancelFollowup(followup.id, followup.version, outcome);
    });
    if (updated) {
      reset();
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={humanise(followup.channel)}
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Badge
              tone={
                {
                  scheduled: "neutral",
                  done: "success",
                  missed: "danger",
                  cancelled: "neutral",
                }[followup.status] as import("@/components/ui").Tone
              }
            >
              {humanise(followup.status)}
            </Badge>
          </span>
        }
        description={
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span>
              Owner:{" "}
              <b className="font-semibold text-night">
                {personName(names, followup.owner_person_id) ?? "—"}
              </b>
            </span>
            <span aria-hidden="true">·</span>
            <span>Due {formatDay(followup.due_at)}</span>
            <SignalRefresh topic="crm" quiet />
          </span>
        }
        actions={
          <>
            <ButtonLink href="/crm/followups" variant="secondary">
              <Icon name="chevronLeft" />
              Back to follow-ups
            </ButtonLink>
          </>
        }
      />

      <Card title="Details">
        <Table>
          <tbody>
            <tr>
              <Th className="text-muted w-[30%]">Channel</Th>
              <Td className="tabular">{humanise(followup.channel)}</Td>
            </tr>
            <tr>
              <Th className="text-muted">Status</Th>
              <Td className="tabular">
                <Badge tone="neutral">{humanise(followup.status)}</Badge>
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Reason</Th>
              <Td className="tabular text-muted">
                {followup.reason ?? "—"}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Outcome</Th>
              <Td className="tabular text-muted">
                {followup.outcome ?? "—"}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Completed at</Th>
              <Td className="tabular text-muted">
                {followup.completed_at
                  ? formatDateTime(followup.completed_at)
                  : "—"}
              </Td>
            </tr>
          </tbody>
        </Table>
      </Card>

      {isOpen ? (
        <Card title="Resolve follow-up">
          <form className="flex flex-col gap-3 px-6 py-[22px]" onSubmit={async (e) => {
            e.preventDefault();
            const data = new FormData(e.currentTarget);
            await onComplete(String(data.get("outcome") ?? "Completed"));
          }}>
            <Field label="Outcome">
              <TextInput
                name="outcome"
                required
                placeholder="e.g. Spoke with decision maker, sent proposal"
              />
            </Field>
            <div className="flex items-end gap-2 pt-4">
              <Button type="submit" variant="primary" disabled={busy}>
                {busy ? "Completing…" : "Mark as done"}
              </Button>
              <Button
                type="button"
                variant="danger"
                onClick={async () => {
                  const outcome = window.prompt("Reason for cancelling:");
                  if (outcome !== null) {
                    await onCancel(outcome);
                  }
                }}
                disabled={busy}
              >
                Cancel
              </Button>
            </div>
          </form>
        </Card>
      ) : null}

      <Card title="Timestamps">
        <Table>
          <tbody>
            <tr>
              <Th className="text-muted">Due at</Th>
              <Td className="tabular text-muted">
                {formatDateTime(followup.due_at)}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Created</Th>
              <Td className="tabular text-muted">
                {formatDateTime(followup.created_at)}
              </Td>
            </tr>
            <tr>
              <Th className="text-muted">Version</Th>
              <Td className="tabular text-muted">{followup.version}</Td>
            </tr>
          </tbody>
        </Table>
      </Card>
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-[12px] font-medium text-night">{label}</label>
      {children}
    </div>
  );
}

function TextInput({
  name,
  value,
  onChange,
  placeholder,
  required,
}: {
  name: string;
  value?: string;
  onChange?: (e: React.ChangeEvent<HTMLInputElement>) => void;
  placeholder?: string;
  required?: boolean;
}) {
  return (
    <input
      name={name}
      value={value ?? ""}
      onChange={onChange}
      placeholder={placeholder}
      required={required}
      className="w-full rounded-control border border-line-strong bg-white px-3 text-[14.5px] text-night outline-none transition placeholder:text-[#7A879C] focus:border-atlas focus:shadow-[0_0_0_3px_rgba(26,108,245,.15)] h-11"
    />
  );
}

function Button({
  children,
  onClick,
  variant,
  disabled,
  type,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  variant?: "primary" | "secondary" | "danger" | "ghost";
  disabled?: boolean;
  type?: "submit" | "reset" | "button";
}) {
  return (
    <button
      type={type || "button"}
      onClick={onClick}
      disabled={disabled}
      className={`px-4 py-2 rounded font-medium text-sm transition ${
        variant === "primary"
          ? "bg-atlas text-white hover:bg-atlas-hover disabled:bg-atlas/50"
          : variant === "danger"
            ? "bg-[#C0352B] text-white hover:bg-[#9E2A22] disabled:bg-[#C0352B]/50"
            : "border-line-strong bg-white text-night hover:border-[#B8C3D6] hover:bg-[#FCFCFD] disabled:opacity-60"
      }`}
    >
      {children}
    </button>
  );
}

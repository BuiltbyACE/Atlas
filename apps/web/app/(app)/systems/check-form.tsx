"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import {
  Button,
  Field,
  Select,
  Switch,
  TextInput,
} from "@/components/form";
import { PersonSelect } from "@/components/person-select";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createCheck,
  CHECK_TYPES,
  setCheckActive,
  type SystemsCheck,
  type SystemsCheckBody,
  updateCheck,
} from "@/lib/api/atlas";
import { humanise } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Checks (`/api/v1/systems/checks`, §8.6). Creating a check takes
 * `systems.system.manage`; pausing/resuming and editing take
 * `systems.system.manage`. Every write is audited.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create/update body from the drawer's form. */
function checkBody(data: FormData): SystemsCheckBody {
  const regionsRaw = field(data, "regions");
  const regions = regionsRaw ? regionsRaw.split(",").map((r) => r.trim()).filter(Boolean) : [];
  const interval = field(data, "interval_seconds");
  const timeout = field(data, "timeout_seconds");
  const warn = field(data, "warn_ms");
  const fail = field(data, "fail_ms");
  const threshold = field(data, "failure_threshold");
  const heartbeat = field(data, "heartbeat_window_seconds");
  return {
    system_id: field(data, "system_id"),
    name: field(data, "name"),
    check_type: field(data, "check_type"),
    target: field(data, "target"),
    ...(interval ? { interval_seconds: Number(interval) } : {}),
    ...(timeout ? { timeout_seconds: Number(timeout) } : {}),
    ...(warn ? { warn_ms: Number(warn) } : {}),
    ...(fail ? { fail_ms: Number(fail) } : {}),
    ...(threshold ? { failure_threshold: Number(threshold) } : {}),
    ...(heartbeat ? { heartbeat_window_seconds: Number(heartbeat) } : {}),
    expect: field(data, "expect"),
    regions,
    active: data.get("active") !== null,
  };
}

/** The fields shared by "new check" and "edit check". */
function CheckFields({
  check,
  systems,
  people,
}: {
  check?: SystemsCheck;
  systems: { id: string; name: string }[];
  people: PersonOption[];
}) {
  return (
    <>
      <Field label="System" htmlFor="check-system">
        <Select id="check-system" name="system_id" required>
          <option value="" selected={!check}>
            Pick a system
          </option>
          {systems.map((s) => (
            <option key={s.id} value={s.id} selected={check?.system_id === s.id}>
              {s.name}
            </option>
          ))}
        </Select>
      </Field>

      <Field label="Name" htmlFor="check-name">
        <TextInput
          id="check-name"
          name="name"
          required
          defaultValue={check?.name ?? ""}
          placeholder="e.g. Checkout HTTPS health"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Type" htmlFor="check-type">
          <Select id="check-type" name="check_type" required>
            {CHECK_TYPES.map((type) => (
              <option key={type} value={type} selected={check?.check_type === type}>
                {humanise(type)}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Target" htmlFor="check-target" hint="URL, host:port or name to probe.">
          <TextInput
            id="check-target"
            name="target"
            required
            defaultValue={check ? "" : ""}
            placeholder="https://api.example.com/health"
          />
        </Field>
      </div>

      <Field label="Expect" htmlFor="check-expect" hint="Substring or status the probe must see.">
        <TextInput
          id="check-expect"
          name="expect"
          defaultValue={check ? "" : ""}
          placeholder="e.g. ok or 200"
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <Field label="Interval (s)" htmlFor="check-interval" hint="How often to probe.">
          <TextInput
            id="check-interval"
            name="interval_seconds"
            type="number"
            defaultValue={check ? "" : ""}
            placeholder="300"
          />
        </Field>
        <Field label="Timeout (s)" htmlFor="check-timeout" hint="Per-probe timeout.">
          <TextInput
            id="check-timeout"
            name="timeout_seconds"
            type="number"
            defaultValue={check ? "" : ""}
            placeholder="10"
          />
        </Field>
        <Field label="Failure threshold" htmlFor="check-threshold" hint="Failures before failing.">
          <TextInput
            id="check-threshold"
            name="failure_threshold"
            type="number"
            defaultValue={check ? "" : ""}
            placeholder="2"
          />
        </Field>
      </div>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <Field label="Warn (ms)" htmlFor="check-warn" hint="Warning response-time threshold.">
          <TextInput
            id="check-warn"
            name="warn_ms"
            type="number"
            defaultValue={check ? "" : ""}
            placeholder="500"
          />
        </Field>
        <Field label="Fail (ms)" htmlFor="check-fail" hint="Failure response-time threshold.">
          <TextInput
            id="check-fail"
            name="fail_ms"
            type="number"
            defaultValue={check ? "" : ""}
            placeholder="2000"
          />
        </Field>
        <Field label="Heartbeat window (s)" htmlFor="check-heartbeat" hint="Max silence before stale.">
          <TextInput
            id="check-heartbeat"
            name="heartbeat_window_seconds"
            type="number"
            defaultValue={check ? "" : ""}
            placeholder="900"
          />
        </Field>
      </div>

      <Field label="Regions" htmlFor="check-regions" hint="Comma-separated regions to probe from.">
        <TextInput
          id="check-regions"
          name="regions"
          defaultValue={check ? "" : ""}
          placeholder="us-east-1, eu-west-1"
        />
      </Field>

      <Switch name="active" defaultChecked={check?.active ?? true} label="Active" />
    </>
  );
}

/**
 * "New check" (`POST /api/v1/systems/checks`, `systems.system.manage`): a
 * button that opens the check form in a drawer.
 */
export function NewCheckButton({
  systems,
  people,
  me,
  capabilities,
  variant = "primary",
}: {
  systems: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("systems.system.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createCheck(checkBody(new FormData(event.currentTarget))),
    );
    if (created) {
      setOpen(false);
      notify(`Added check ${created.name}`);
      router.refresh();
    }
  }

  return (
    <>
      <Button
        variant={variant}
        size={variant === "secondary" ? "sm" : "md"}
        onClick={() => setOpen(true)}
      >
        <Icon name="plus" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New check
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New check"
        description="The probe that turns a system's health into a fact."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The check wasn’t saved" />}
          <CheckFields systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Add check"}
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
 * The actions for one check row: edit it, or pause/resume it. All of it is
 * `systems.system.manage`, guarded by `If-Match`.
 */
export function CheckActions({
  check,
  systems,
  people,
  me,
  capabilities,
}: {
  check: SystemsCheck;
  systems: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("systems.system.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const updated = await run(() =>
      updateCheck(check.id, check.version, checkBody(new FormData(event.currentTarget))),
    );
    if (updated) {
      close();
      notify(`Saved ${updated.name}`);
      router.refresh();
    }
  }

  async function setActive(active: boolean) {
    const updated = await run(() => setCheckActive(check.id, check.version, active));
    if (updated) {
      notify(`${updated.active ? "Resumed" : "Paused"} ${updated.name}`);
      router.refresh();
    }
  }

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setOpen(true)}>
          Edit
        </Button>
        <Button
          size="sm"
          variant={check.active ? "secondary" : "light"}
          disabled={busy}
          onClick={() => void setActive(!check.active)}
        >
          {check.active ? "Pause" : "Resume"}
        </Button>
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      <Drawer
        open={open}
        onClose={close}
        title="Edit check"
        description={check.name}
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The change wasn’t saved" />}
          <CheckFields check={check} systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Saving…" : "Save check"}
            </Button>
            <Button type="button" variant="secondary" onClick={close} disabled={busy}>
              Cancel
            </Button>
          </div>
        </form>
      </Drawer>
    </div>
  );
}

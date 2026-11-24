"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import {
  Button,
  Field,
  TextInput,
} from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  createMetricSnapshot,
  type MetricSnapshotBody,
} from "@/lib/api/atlas";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Intel metric snapshots (§8.8). Creating takes `intel.metric.manage`.
 * Every write is audited with the actor's name.
 */

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the create body from the drawer's form. */
function metricSnapshotBody(data: FormData): MetricSnapshotBody {
  return {
    metric_name: field(data, "metric_name"),
    metric_value: field(data, "metric_value"),
    ...(field(data, "as_of") ? { as_of: field(data, "as_of") } : {}),
    source: field(data, "source"),
  };
}

/** The fields for "new metric snapshot". */
function MetricSnapshotFields() {
  return (
    <>
      <Field label="Metric name" htmlFor="snapshot-metric-name">
        <TextInput
          id="snapshot-metric-name"
          name="metric_name"
          required
          defaultValue=""
          placeholder="e.g. cash_position_minor"
          data-autofocus
        />
      </Field>

      <Field label="Metric value" htmlFor="snapshot-metric-value">
        <TextInput
          id="snapshot-metric-value"
          name="metric_value"
          required
          defaultValue=""
          placeholder="e.g. 12500000"
        />
      </Field>

      <Field label="As of" htmlFor="snapshot-as-of" hint="YYYY-MM-DD.">
        <TextInput
          id="snapshot-as-of"
          name="as_of"
          type="date"
          defaultValue=""
        />
      </Field>

      <Field label="Source" htmlFor="snapshot-source" hint="Where this figure came from, e.g. finance.overview.">
        <TextInput
          id="snapshot-source"
          name="source"
          defaultValue=""
          placeholder="finance.overview"
        />
      </Field>
    </>
  );
}

/**
 * "New metric snapshot" (`POST /api/v1/intel/metric-snapshots`,
 * `intel.metric.manage`): a button that opens the metric snapshot form in a
 * drawer.
 */
export function NewMetricSnapshotButton({
  people,
  me,
  capabilities,
  variant = "primary",
}: {
  people: PersonOption[];
  me?: string;
  capabilities: string[];
  variant?: "primary" | "secondary";
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [open, setOpen] = useState(false);

  if (!capabilities.includes("intel.metric.manage")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() =>
      createMetricSnapshot(metricSnapshotBody(new FormData(event.currentTarget))),
    );
    if (created) {
      setOpen(false);
      notify(`Created metric ${created.metric_name}`);
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
        <Icon name="signal" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New metric snapshot
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New metric snapshot"
        description="Record a point-in-time KPI figure."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The metric snapshot wasn’t created" />}
          <MetricSnapshotFields />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Creating…" : "Create snapshot"}
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

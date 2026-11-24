"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Drawer } from "@/components/drawer";
import {
  Button,
  Field,
  Select,
  Switch,
  TextArea,
  TextInput,
} from "@/components/form";
import { PersonSelect } from "@/components/person-select";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { cx, ProblemNotice } from "@/components/ui";
import {
  addIncidentNote,
  createIncidentAction,
  createIncidentAction as _unusedCreateAction,
  INCIDENT_ACTION_STATUSES,
  INCIDENT_SEVERITIES,
  INCIDENT_SOURCES,
  openIncident,
  reviewIncident,
  stepIncident,
  type SystemsIncident,
  updateIncidentAction,
} from "@/lib/api/atlas";
import { humanise, today } from "@/lib/format";
import { useMutation } from "@/lib/use-mutation";

import type { PersonOption } from "@/lib/directory";

/**
 * Incidents (`/api/v1/systems/incidents`, §8.6). Opening takes
 * `systems.incident.create`; lifecycle steps, notes and follow-up actions take
 * `systems.incident.manage`. Every transition is audited and pushed to the
 * systems board and the dashboard.
 */

_unusedCreateAction;

/** A text field, trimmed; `""` when the field is absent. */
function field(data: FormData, key: string): string {
  return String(data.get(key) ?? "").trim();
}

/** Build the open-incident body from the drawer's form. */
function incidentBody(data: FormData) {
  const systemsRaw = data.getAll("system_ids").map(String).filter((v) => v.trim());
  return {
    title: field(data, "title"),
    severity: field(data, "severity"),
    system_ids: systemsRaw,
    detected_at: field(data, "detected_at") || today(),
    detected_source: field(data, "detected_source") || "manual",
    commander_person_id: field(data, "commander_person_id"),
    client_impact: field(data, "client_impact"),
  };
}

/** The fields shared by "new incident". */
function IncidentFields({
  systems,
  people,
}: {
  systems: { id: string; name: string }[];
  people: PersonOption[];
}) {
  return (
    <>
      <Field label="Title" htmlFor="incident-title">
        <TextInput
          id="incident-title"
          name="title"
          required
          placeholder="e.g. Checkout returns 500 for all clients"
          data-autofocus
        />
      </Field>

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <Field label="Severity" htmlFor="incident-severity">
          <Select id="incident-severity" name="severity" required>
            {INCIDENT_SEVERITIES.map((sev) => (
              <option key={sev} value={sev}>
                {sev.toUpperCase()}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Detected source" htmlFor="incident-source">
          <Select id="incident-source" name="detected_source">
            {INCIDENT_SOURCES.map((src) => (
              <option key={src} value={src} selected={src === "manual"}>
                {humanise(src)}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      <Field label="Systems" htmlFor="incident-systems" hint="One or more systems this incident touched.">
        <Select id="incident-systems" name="system_ids" multiple size={4}>
          {systems.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </Select>
      </Field>

      <Field label="Detected at" htmlFor="incident-detected" hint="Defaults to now.">
        <TextInput
          id="incident-detected"
          name="detected_at"
          type="datetime-local"
          defaultValue={today()}
        />
      </Field>

      <Field label="Commander" htmlFor="incident-commander" hint="Person leading the response.">
        <PersonSelect
          id="incident-commander"
          name="commander_person_id"
          people={people}
          emptyLabel="Unassigned"
        />
      </Field>

      <Field label="Client impact" htmlFor="incident-impact">
        <TextArea
          id="incident-impact"
          name="client_impact"
          placeholder="What clients experienced, if anything."
        />
      </Field>
    </>
  );
}

/**
 * "New incident" (`POST /api/v1/systems/incidents`, `systems.incident.create`):
 * a button that opens the incident form in a drawer.
 */
export function NewIncidentButton({
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

  if (!capabilities.includes("systems.incident.create")) return null;

  function close() {
    setOpen(false);
    reset();
  }

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const created = await run(() => openIncident(incidentBody(new FormData(event.currentTarget))));
    if (created) {
      setOpen(false);
      notify(`Opened incident ${created.ref}`);
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
        <Icon name="alert" className={variant === "secondary" ? "h-4 w-4" : undefined} />
        New incident
      </Button>
      <Drawer
        open={open}
        onClose={close}
        title="New incident"
        description="What broke, how severe, and which systems it touched."
      >
        <form onSubmit={onSubmit} className="flex flex-col gap-4 px-6 py-[22px]">
          {problem && <ProblemNotice error={problem} title="The incident wasn’t opened" />}
          <IncidentFields systems={systems} people={people} />
          <div className="flex flex-wrap gap-2.5">
            <Button type="submit" disabled={busy}>
              {busy ? "Opening…" : "Open incident"}
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
 * The actions for one incident row: lifecycle steps (acknowledge, mitigate,
 * resolve, close), the post-incident review, timeline notes and follow-up
 * actions. All of it is `systems.incident.manage`, guarded by `If-Match`.
 */
export function IncidentActions({
  incident,
  systems,
  people,
  me,
  capabilities,
}: {
  incident: SystemsIncident;
  systems: { id: string; name: string }[];
  people: PersonOption[];
  me?: string;
  capabilities: string[];
}) {
  const router = useRouter();
  const notify = useToast();
  const { busy, problem, run, reset } = useMutation();
  const [step, setStep] = useState<"acknowledge" | "mitigate" | "resolve" | "close" | "review" | "note" | "action" | null>(null);
  const [reviewOpen, setReviewOpen] = useState(false);
  const [noteOpen, setNoteOpen] = useState(false);
  const [actionOpen, setActionOpen] = useState(false);

  if (!capabilities.includes("systems.incident.manage")) return null;

  function close() {
    setStep(null);
    reset();
  }

  async function doStep(stepName: "acknowledge" | "mitigate" | "resolve" | "close", note: string) {
    const updated = await run(() => stepIncident(incident.id, stepName, incident.version, note || undefined));
    if (updated) {
      close();
      notify(`${humanise(stepName)}d incident ${incident.ref}`);
      router.refresh();
    }
  }

  async function onReview(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const updated = await run(() =>
      reviewIncident(incident.id, incident.version, {
        root_cause: field(data, "root_cause"),
        summary: field(data, "summary") || undefined,
      }),
    );
    if (updated) {
      setReviewOpen(false);
      notify(`Review recorded for ${incident.ref}`);
      router.refresh();
    }
  }

  async function onNote(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(() => addIncidentNote(incident.id, field(data, "note")));
    if (created) {
      setNoteOpen(false);
      notify("Note added");
      router.refresh();
    }
  }

  async function onAction(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const created = await run(() =>
      createIncidentAction(incident.id, {
        title: field(data, "title"),
        owner_person_id: field(data, "owner_person_id"),
        due_on: field(data, "due_on"),
        status: field(data, "status"),
        task_id: field(data, "task_id"),
      }),
    );
    if (created) {
      setActionOpen(false);
      notify("Action added");
      router.refresh();
    }
  }

  const canAcknowledge = incident.status === "open";
  const canMitigate = incident.status === "acknowledged";
  const canResolve = incident.status === "mitigated";
  const canClose = incident.status === "resolved";
  const canReview = incident.review_required && incident.status === "resolved";

  const nothing = !canAcknowledge && !canMitigate && !canResolve && !canClose && !canReview;

  return (
    <div className="flex flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-1.5">
        {canAcknowledge && (
          <Button size="sm" disabled={busy} onClick={() => doStep("acknowledge", "")}>
            Acknowledge
          </Button>
        )}
        {canMitigate && (
          <Button size="sm" disabled={busy} onClick={() => doStep("mitigate", "")}>
            Mitigate
          </Button>
        )}
        {canResolve && (
          <Button size="sm" disabled={busy} onClick={() => setStep("resolve")}>
            Resolve
          </Button>
        )}
        {canClose && (
          <Button size="sm" disabled={busy} onClick={() => doStep("close", "")}>
            Close
          </Button>
        )}
        {canReview && (
          <Button size="sm" variant="secondary" disabled={busy} onClick={() => setReviewOpen(true)}>
            Review
          </Button>
        )}
        {!nothing && (
          <Button size="sm" variant="secondary" disabled={busy} onClick={() => setStep("note")}>
            Note
          </Button>
        )}
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => setStep("action")}>
          Action
        </Button>
      </div>

      {problem && (
        <span className={cx("max-w-[240px] text-[12.5px] font-medium text-[#B42318]")}>
          {problem.message}
        </span>
      )}

      {step === "resolve" && (
        <Drawer open={step === "resolve"} onClose={() => setStep(null)} title="Resolve incident" description={incident.ref}>
          <form onSubmit={(e) => { e.preventDefault(); void doStep("resolve", field(new FormData(e.currentTarget), "note")); }} className="flex flex-col gap-4 px-6 py-[22px]">
            {problem && <ProblemNotice error={problem} title="The incident wasn’t resolved" />}
            <Field label="Note" htmlFor="resolve-note">
              <TextArea id="resolve-note" name="note" placeholder="How was it resolved?" />
            </Field>
            <div className="flex flex-wrap gap-2.5">
              <Button type="submit" disabled={busy}>{busy ? "Resolving…" : "Resolve incident"}</Button>
              <Button type="button" variant="secondary" onClick={() => setStep(null)} disabled={busy}>Cancel</Button>
            </div>
          </form>
        </Drawer>
      )}

      {reviewOpen && (
        <Drawer open={reviewOpen} onClose={() => setReviewOpen(false)} title="Post-incident review" description={incident.ref}>
          <form onSubmit={onReview} className="flex flex-col gap-4 px-6 py-[22px]">
            {problem && <ProblemNotice error={problem} title="The review wasn’t recorded" />}
            <Field label="Root cause" htmlFor="review-root-cause">
              <TextArea id="review-root-cause" name="root_cause" placeholder="What went wrong and why." required />
            </Field>
            <Field label="Summary" htmlFor="review-summary">
              <TextArea id="review-summary" name="summary" placeholder="What happened, in one or two sentences." />
            </Field>
            <div className="flex flex-wrap gap-2.5">
              <Button type="submit" disabled={busy}>{busy ? "Recording…" : "Record review"}</Button>
              <Button type="button" variant="secondary" onClick={() => setReviewOpen(false)} disabled={busy}>Cancel</Button>
            </div>
          </form>
        </Drawer>
      )}

      {step === "note" && (
        <Drawer open={noteOpen} onClose={() => setNoteOpen(false)} title="Add timeline note" description={incident.ref}>
          <form onSubmit={onNote} className="flex flex-col gap-4 px-6 py-[22px]">
            {problem && <ProblemNotice error={problem} title="The note wasn’t added" />}
            <Field label="Note" htmlFor="note-text">
              <TextArea id="note-text" name="note" placeholder="What happened and when." required />
            </Field>
            <div className="flex flex-wrap gap-2.5">
              <Button type="submit" disabled={busy}>{busy ? "Adding…" : "Add note"}</Button>
              <Button type="button" variant="secondary" onClick={() => setNoteOpen(false)} disabled={busy}>Cancel</Button>
            </div>
          </form>
        </Drawer>
      )}

      {step === "action" && (
        <Drawer open={actionOpen} onClose={() => setActionOpen(false)} title="Add follow-up action" description={incident.ref}>
          <form onSubmit={onAction} className="flex flex-col gap-4 px-6 py-[22px]">
            {problem && <ProblemNotice error={problem} title="The action wasn’t added" />}
            <Field label="Title" htmlFor="action-title">
              <TextInput id="action-title" name="title" placeholder="What to do next." required />
            </Field>
            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
              <Field label="Owner" htmlFor="action-owner">
                <PersonSelect id="action-owner" name="owner_person_id" people={people} emptyLabel="Unassigned" />
              </Field>
              <Field label="Due on" htmlFor="action-due">
                <TextInput id="action-due" name="due_on" type="date" placeholder="YYYY-MM-DD" />
              </Field>
              <Field label="Status" htmlFor="action-status">
                <Select id="action-status" name="status">
                  {INCIDENT_ACTION_STATUSES.map((s) => (
                    <option key={s} value={s} selected={s === "todo"}>
                      {humanise(s)}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
            <Field label="Task id" htmlFor="action-task" hint="Optional work task this action links to.">
              <TextInput id="action-task" name="task_id" placeholder="Task id (UUID)" />
            </Field>
            <div className="flex flex-wrap gap-2.5">
              <Button type="submit" disabled={busy}>{busy ? "Adding…" : "Add action"}</Button>
              <Button type="button" variant="secondary" onClick={() => setActionOpen(false)} disabled={busy}>Cancel</Button>
            </div>
          </form>
        </Drawer>
      )}
    </div>
  );
}

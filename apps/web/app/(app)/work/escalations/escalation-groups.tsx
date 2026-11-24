"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/form";
import { Icon } from "@/components/icons";
import { useToast } from "@/components/toast";
import { Avatar, Badge, IconTile, cx, type Tone } from "@/components/ui";
import { resolveEscalation, type Escalation } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { formatDateTime, initials, timeAgo } from "@/lib/format";

/** One subject's escalations: a person who keeps missing reports, a late task. */
export interface EscalationGroup {
  key: string;
  entityType: string;
  entityId: string;
  /** A display name: the person's name or the task's title. */
  subject: string;
  href?: string;
  /** Newest first. */
  items: Escalation[];
}

function levelTone(level: number): Tone {
  if (level >= 3) return "navy";
  if (level === 2) return "danger";
  return "warning";
}

/**
 * Escalations grouped by what they are about (prototype 05). A group shows the
 * newest reason, the highest open rung and who it is with; expanding it lists
 * each escalation. Resolving is `POST /api/v1/work/escalations/{id}/resolve`
 * (`work.task.review`), one call per escalation; Go refuses what the viewer
 * may not resolve and the reason is shown.
 */
export function EscalationGroups({
  groups,
  names,
}: {
  groups: EscalationGroup[];
  names: Record<string, string>;
}) {
  const router = useRouter();
  const notify = useToast();
  const [expanded, setExpanded] = useState<Set<string>>(
    () => new Set(groups.length === 1 ? [groups[0].key] : []),
  );
  const [busy, setBusy] = useState<string | null>(null);

  function toggle(key: string) {
    setExpanded((set) => {
      const next = new Set(set);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  async function resolve(key: string, ids: string[], subject: string) {
    setBusy(key);
    let done = 0;
    let refusal: string | null = null;
    for (const id of ids) {
      try {
        await resolveEscalation(id);
        done += 1;
      } catch (error) {
        refusal = error instanceof ApiError ? error.message : "Atlas couldn’t be reached.";
        break;
      }
    }
    setBusy(null);
    if (done > 0) {
      notify(`Resolved ${done} escalation${done === 1 ? "" : "s"} for ${subject}`);
      router.refresh();
    }
    if (refusal) notify(refusal, "error");
  }

  function assigneeLabel(item: Escalation): string {
    if (item.assignee_person_id) {
      return names[item.assignee_person_id] ?? `Person ${item.assignee_person_id.slice(0, 8)}`;
    }
    return item.level >= 3 ? "Both CEOs" : "Unassigned";
  }

  return (
    <div className="overflow-x-auto scroll-quiet">
      <div className="min-w-[1040px]">
        <div
          aria-hidden="true"
          className="grid grid-cols-[minmax(260px,1.35fr)_minmax(220px,1.3fr)_64px_minmax(160px,.9fr)_minmax(160px,.9fr)_150px] items-center gap-4 rounded-t-card border-b border-line bg-[#FBFBFC] px-[22px] py-3 text-xs font-semibold uppercase tracking-[0.08em] text-muted"
        >
          <span>About</span>
          <span>Reason</span>
          <span>Level</span>
          <span>With</span>
          <span>Latest</span>
          <span />
        </div>

        {groups.map((group) => {
          const latest = group.items[0];
          const open = group.items.filter((item) => item.status === "open");
          const level = Math.max(...(open.length ? open : group.items).map((item) => item.level));
          const isOpen = expanded.has(group.key);
          const isPerson = group.entityType === "person";
          const working = busy === group.key;

          return (
            <div key={group.key} className="border-t border-line first-of-type:border-t-0">
              <div className="grid grid-cols-[minmax(260px,1.35fr)_minmax(220px,1.3fr)_64px_minmax(160px,.9fr)_minmax(160px,.9fr)_150px] items-center gap-4 px-[22px] py-3.5">
                <button
                  type="button"
                  aria-expanded={isOpen}
                  onClick={() => toggle(group.key)}
                  className="flex min-h-11 items-center gap-3 text-left"
                >
                  <Icon
                    name="chevronRight"
                    className={cx("text-muted transition-transform", isOpen && "rotate-90")}
                  />
                  {isPerson ? (
                    <Avatar initials={initials(group.subject)} />
                  ) : (
                    <IconTile icon="board" tone="info" size="sm" round />
                  )}
                  <span className="min-w-0">
                    <b className="block truncate text-[15px] font-semibold">{group.subject}</b>
                    <small className="block text-[13px] text-muted">
                      {open.length > 0
                        ? `${open.length} open${group.items.length > 1 ? ` · raised ${group.items.length}×` : ""}`
                        : `All ${group.items.length} resolved`}
                    </small>
                  </span>
                </button>
                <span className="text-[14.5px]">{latest.reason || "—"}</span>
                <span>
                  <Badge tone={levelTone(level)}>L{level}</Badge>
                </span>
                <span className="inline-flex min-w-0 items-center gap-2 text-[14.5px]">
                  <Avatar initials={initials(assigneeLabel(latest))} size="sm" />
                  <span className="truncate">{assigneeLabel(latest)}</span>
                </span>
                <span className="text-sm">
                  {formatDateTime(latest.created_at)}
                  <small className="block text-[12.5px] text-muted">{timeAgo(latest.created_at)}</small>
                </span>
                <span className="flex justify-end">
                  {open.length > 0 ? (
                    <Button
                      size="sm"
                      variant="secondary"
                      disabled={working}
                      onClick={() =>
                        resolve(
                          group.key,
                          open.map((item) => item.id),
                          group.subject,
                        )
                      }
                    >
                      {working ? "Resolving…" : open.length > 1 ? `Resolve all ${open.length}` : "Resolve"}
                    </Button>
                  ) : (
                    <Badge tone="success" dot>
                      Resolved
                    </Badge>
                  )}
                </span>
              </div>

              {isOpen && (
                <ul className="border-t border-dashed border-line-strong bg-[#FAFBFC]">
                  {group.items.map((item, index) => (
                    <li
                      key={item.id}
                      className="grid grid-cols-[minmax(260px,1.35fr)_minmax(220px,1.3fr)_64px_minmax(160px,.9fr)_minmax(160px,.9fr)_150px] items-center gap-4 border-t border-line px-[22px] py-2.5 first:border-t-0"
                    >
                      <span className="flex items-center gap-2.5 pl-[30px] text-sm">
                        <span className="font-mono text-xs text-muted">#{group.items.length - index}</span>
                        {group.href ? (
                          <Link href={group.href} className="text-atlas-ink hover:underline">
                            {isPerson ? "Missed daily reports" : "Open the task"}
                          </Link>
                        ) : (
                          "Raised by the sweep"
                        )}
                      </span>
                      <span className="text-sm text-muted">{item.reason || "—"}</span>
                      <span>
                        <Badge tone={levelTone(item.level)}>L{item.level}</Badge>
                      </span>
                      <span>
                        <Badge tone={item.status === "open" ? "warning" : "success"} dot>
                          {item.status === "open" ? "Open" : "Resolved"}
                        </Badge>
                        {item.resolved_at && (
                          <small className="mt-1 block text-xs text-muted">
                            {item.resolved_by_person_id
                              ? `by ${names[item.resolved_by_person_id] ?? "a person"}`
                              : "by the sweep"}{" "}
                            · {timeAgo(item.resolved_at)}
                          </small>
                        )}
                      </span>
                      <span className="text-sm">{formatDateTime(item.created_at)}</span>
                      <span className="flex justify-end">
                        {item.status === "open" && (
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={working}
                            onClick={() => resolve(group.key, [item.id], group.subject)}
                          >
                            Resolve
                          </Button>
                        )}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

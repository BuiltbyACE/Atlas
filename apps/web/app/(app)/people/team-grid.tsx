"use client";

import { useMemo, useState } from "react";

import { Button } from "@/components/form";
import { Icon } from "@/components/icons";
import { Avatar, Badge, ChipButton, Chips, EmptyState, cx } from "@/components/ui";
import type { AvailabilityItem } from "@/lib/api/atlas";
import { formatShortDay, humanise, initials, leaveTypeLabel } from "@/lib/format";

const PAGE = 24;

type Filter = "all" | "available" | "away";

/**
 * The day's roster as person tiles (prototype 06 "Team"): who is in, who is on
 * approved leave and who is away for another reason. Go decides `available`
 * per person; this only filters and searches the list it returned.
 */
export function TeamGrid({ items, me }: { items: AvailabilityItem[]; me?: string }) {
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const [showAll, setShowAll] = useState(false);

  const counts = useMemo(
    () => ({
      all: items.length,
      available: items.filter((item) => item.available).length,
      away: items.filter((item) => !item.available).length,
    }),
    [items],
  );

  const list = useMemo(() => {
    const q = query.trim().toLowerCase();
    return items
      .filter((item) =>
        filter === "available" ? item.available : filter === "away" ? !item.available : true,
      )
      .filter((item) => !q || item.full_name.toLowerCase().includes(q))
      .sort((a, b) => {
        // The viewer first, then the people who are away, then by name.
        if (a.person_id === me) return -1;
        if (b.person_id === me) return 1;
        if (a.available !== b.available) return a.available ? 1 : -1;
        return a.full_name.localeCompare(b.full_name);
      });
  }, [items, filter, query, me]);

  const shown = showAll ? list : list.slice(0, PAGE);

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3 px-[22px] pb-4">
        <Chips label="Filter people">
          <ChipButton pressed={filter === "all"} count={counts.all} onClick={() => setFilter("all")}>
            Everyone
          </ChipButton>
          <ChipButton pressed={filter === "available"} count={counts.available} onClick={() => setFilter("available")}>
            Available
          </ChipButton>
          <ChipButton pressed={filter === "away"} count={counts.away} onClick={() => setFilter("away")}>
            Away
          </ChipButton>
        </Chips>
        <label className="flex h-10 min-w-[220px] items-center gap-2 rounded-control border border-line-strong bg-white px-3 text-muted focus-within:border-atlas focus-within:shadow-[0_0_0_3px_rgba(26,108,245,.15)]">
          <Icon name="search" className="h-4 w-4" />
          <span className="sr-only">Find a person</span>
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Find a person"
            className="min-w-0 flex-1 bg-transparent text-[14.5px] text-night outline-none placeholder:text-[#7A879C]"
          />
        </label>
      </div>

      {shown.length === 0 ? (
        <div className="px-[22px] pb-1.5">
          <EmptyState
            icon={filter === "away" ? "check" : "search"}
            iconTone={filter === "away" ? "success" : "info"}
            className="min-h-[200px]"
            title={
              query
                ? `No one matches “${query}”`
                : filter === "away"
                  ? "Nobody is away on this day"
                  : "No people on record"
            }
          >
            {query
              ? "Try a different name."
              : filter === "away"
                ? "People on approved leave, or not active on this day, show up here."
                : undefined}
          </EmptyState>
        </div>
      ) : (
        <ul className="grid grid-cols-[repeat(auto-fill,minmax(min(240px,100%),1fr))] gap-3 px-[22px] pb-1.5">
          {shown.map((item) => {
            const isMe = item.person_id === me;
            const onLeave = Boolean(item.leave);
            return (
              <li
                key={item.person_id}
                className={cx(
                  "flex items-center gap-3 rounded-[14px] border px-3.5 py-3",
                  isMe ? "border-[#BFD3FF] bg-[#F7FAFF]" : "border-line bg-white",
                )}
              >
                <span className="relative flex-none">
                  <Avatar initials={initials(item.full_name)} size="lg" variant={isMe ? "me" : "default"} />
                  <span
                    aria-hidden="true"
                    className={cx(
                      "absolute -bottom-px -right-px h-3 w-3 rounded-full border-2 border-white",
                      item.available ? "bg-[#22A55A]" : onLeave ? "bg-atlas" : "bg-[#F5A524]",
                    )}
                  />
                </span>
                <span className="min-w-0 flex-1">
                  <b className="block truncate text-[14.5px] font-semibold">
                    {item.full_name}
                    {isMe && <span className="font-normal text-muted"> (you)</span>}
                  </b>
                  <small className="block truncate text-[12.5px] text-muted">
                    {item.leave
                      ? `${leaveTypeLabel(item.leave.leave_type)} until ${formatShortDay(item.leave.to_date)}`
                      : humanise(item.employment_type)}
                  </small>
                </span>
                {item.available ? (
                  <Badge tone="success" dot>In</Badge>
                ) : onLeave ? (
                  <Badge tone="info" dot>On leave</Badge>
                ) : (
                  <Badge tone="warning" dot>{humanise(item.status)}</Badge>
                )}
              </li>
            );
          })}
        </ul>
      )}

      <div className="flex flex-wrap items-center justify-between gap-3 px-[22px] pb-5 pt-4 text-sm text-muted">
        <span>
          Showing {shown.length} of {list.length === items.length ? items.length : `${list.length} matching`}{" "}
          {items.length === 1 ? "person" : "people"}
        </span>
        {list.length > PAGE && (
          <Button size="sm" variant="secondary" onClick={() => setShowAll((value) => !value)}>
            {showAll ? "Show fewer" : `Show all ${list.length}`}
          </Button>
        )}
      </div>
    </>
  );
}

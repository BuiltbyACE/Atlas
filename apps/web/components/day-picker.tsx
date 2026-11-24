"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { Icon } from "@/components/icons";
import { Badge } from "@/components/ui";
import { addDays, formatLongDay } from "@/lib/format";

/**
 * A day stepper (prototype `.av-date`): previous / next buttons around a date
 * field, the day in words, and a way back to today. The day lives in the URL
 * (`?date=YYYY-MM-DD`) so the server page reads it and the view is shareable.
 */
export function DayPicker({ day, today }: { day: string; today: string }) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();

  function go(next: string) {
    const query = new URLSearchParams(params.toString());
    if (next === today) query.delete("date");
    else query.set("date", next);
    const qs = query.toString();
    router.push(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  }

  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-3">
      <div
        role="group"
        aria-label="Choose day"
        className="inline-flex items-center gap-1 rounded-[14px] border border-line-strong bg-white p-1"
      >
        <button
          type="button"
          aria-label="Previous day"
          onClick={() => go(addDays(day, -1))}
          className="grid h-10 w-10 place-items-center rounded-[10px] text-night transition hover:bg-[#F1F4FA]"
        >
          <Icon name="chevronLeft" />
        </button>
        <label className="sr-only" htmlFor="day-picker">
          Day
        </label>
        <input
          id="day-picker"
          type="date"
          value={day}
          onChange={(event) => {
            if (event.target.value) go(event.target.value);
          }}
          className="h-10 bg-transparent px-2 text-[14.5px] text-night outline-none"
        />
        <button
          type="button"
          aria-label="Next day"
          onClick={() => go(addDays(day, 1))}
          className="grid h-10 w-10 place-items-center rounded-[10px] text-night transition hover:bg-[#F1F4FA]"
        >
          <Icon name="chevronRight" />
        </button>
      </div>
      <span className="inline-flex items-center gap-2.5 text-[15px] font-semibold">
        {formatLongDay(day)}
        {day === today ? (
          <Badge tone="navy">Today</Badge>
        ) : (
          <button
            type="button"
            onClick={() => go(today)}
            className="rounded-[10px] px-2.5 py-1.5 text-[13.5px] font-semibold text-atlas-ink hover:bg-[#EEF3FF]"
          >
            Back to today
          </button>
        )}
      </span>
    </div>
  );
}

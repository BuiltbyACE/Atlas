"use client";

import { useEffect, useState } from "react";

function partOfDay(hour: number): string {
  if (hour < 12) return "Good morning";
  if (hour < 17) return "Good afternoon";
  return "Good evening";
}

/**
 * The hero's date line and greeting, in the viewer's own clock (a server
 * render would greet in the server's timezone). Until the browser has a clock
 * it renders a neutral "Welcome back", so the markup matches on hydration.
 */
export function Greeting({ firstName }: { firstName: string }) {
  const [now, setNow] = useState<Date | null>(null);

  useEffect(() => {
    setNow(new Date());
    const timer = window.setInterval(() => setNow(new Date()), 60_000);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <>
      <p className="min-h-[1.2em] text-[12.5px] font-semibold uppercase tracking-[0.12em] text-navy-soft">
        {now
          ? `${now.toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long", year: "numeric" })} · ${now.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })}`
          : " "}
      </p>
      <h1 className="mt-2.5 text-[30px] font-semibold leading-tight tracking-[-0.02em] text-white sm:text-[40px]">
        {now ? partOfDay(now.getHours()) : "Welcome back"}, {firstName}
      </h1>
    </>
  );
}

import type { Metadata } from "next";
import Image from "next/image";
import { redirect } from "next/navigation";

import { getCurrentUser } from "@/lib/auth/session";
import inkSplash from "@/public/brand/ink-splash.jpg";

import { LoginFlow } from "./login-flow";

export const metadata: Metadata = {
  title: "Sign in · Atlas",
  description: "Sign in to Atlas, the SafariStack Company Operating System.",
};

/** Reads the session cookie, so it is always rendered per request. */
export const dynamic = "force-dynamic";

/**
 * Atlas sign-in (docs/FRONTEND.md §2).
 *
 * Authentication is owned by Go (ADR-008); this page only decides whether to
 * show the journey or skip it for someone already signed in. The journey
 * itself is the `LoginFlow` client component, which talks JSON to `/api/*`.
 *
 * Layout: the ink splash is sized from the form's own box rather than the
 * viewport, so the form always sits on navy whatever the screen or step. The
 * splash's fully solid core is ~40% x 56% of the artwork; at 165% of the form's
 * height the form fits inside it, and a halo in the same navy covers any edge
 * case. `mix-blend-multiply` melts the artwork's white into the milk page.
 */
export default async function LoginPage() {
  if (await getCurrentUser()) {
    redirect("/dashboard");
  }

  return (
    <main className="relative flex min-h-svh flex-col items-center justify-center overflow-hidden bg-milk px-5 py-20 [@media(max-height:560px)]:py-10">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_at_50%_42%,theme(colors.milk.50)_0%,theme(colors.milk.DEFAULT)_55%,theme(colors.milk.200)_100%)]"
      />

      <div className="relative w-full max-w-[22.5rem] sm:max-w-[25rem] xl:max-w-[27rem]">
        <Image
          src={inkSplash}
          alt=""
          aria-hidden="true"
          priority
          sizes="(min-width: 1280px) 1700px, (min-width: 640px) 1500px, 1200px"
          className="pointer-events-none absolute left-1/2 top-1/2 aspect-[1672/941] h-[165%] w-auto max-w-none -translate-x-1/2 -translate-y-1/2 select-none mix-blend-multiply"
        />
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -inset-x-6 -inset-y-4 rounded-[3rem] bg-ink blur-2xl"
        />
        <div className="relative py-4 text-white">
          <LoginFlow />
        </div>
      </div>
    </main>
  );
}

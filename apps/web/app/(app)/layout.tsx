import Link from "next/link";
import type { ReactNode } from "react";

import { SignOutButton } from "@/components/sign-out-button";
import { requireUser } from "@/lib/auth/session";

/**
 * The authenticated shell is always request-scoped: it reads the session
 * cookie (Phase 1 placeholder via `requireUser`) and therefore must never be
 * statically prerendered.
 */
export const dynamic = "force-dynamic";

/** Modules from blueprint section 4 (capability map). */
const NAVIGATION = [
  { label: "Dashboard", href: "/dashboard" },
  { label: "CRM", href: "/crm" },
  { label: "Work", href: "/work" },
  { label: "Contracts", href: "/contracts" },
  { label: "Finance", href: "/finance" },
  { label: "People", href: "/people" },
  { label: "Systems", href: "/systems" },
  { label: "Units", href: "/units" },
  { label: "Intel", href: "/intel" },
  { label: "Settings", href: "/settings" },
] as const;

export default async function AppLayout({ children }: { children: ReactNode }) {
  // The real check is enforced by Go on every request; this only decides
  // whether to render the shell or bounce to /login.
  const user = await requireUser();

  return (
    <div className="flex min-h-screen bg-milk">
      <aside className="flex w-60 shrink-0 flex-col border-r border-slate-200 bg-white">
        <div className="border-b border-slate-200 px-5 py-4">
          <Link
            href="/dashboard"
            className="text-base font-semibold tracking-tight"
          >
            Atlas
          </Link>
          <p className="text-xs text-slate-500">SafariStack Company OS</p>
        </div>

        <nav className="flex-1 space-y-0.5 overflow-y-auto px-3 py-4">
          {NAVIGATION.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="block rounded-md px-3 py-2 text-sm text-slate-700 hover:bg-slate-100"
            >
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="space-y-3 border-t border-slate-200 px-5 py-4 text-xs text-slate-500">
          <div>
            <div className="truncate font-medium text-slate-700">{user.name}</div>
            <div className="truncate">{user.email}</div>
          </div>
          <SignOutButton />
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="border-b border-slate-200 bg-white px-6 py-3 text-xs font-medium uppercase tracking-wide text-slate-500">
          Phase 1 placeholder shell
        </header>
        <main className="flex-1 px-6 py-6">{children}</main>
      </div>
    </div>
  );
}

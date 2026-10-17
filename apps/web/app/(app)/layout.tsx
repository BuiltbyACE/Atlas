import Link from "next/link";
import type { ReactNode } from "react";

import { SignOutButton } from "@/components/sign-out-button";
import { requireUser } from "@/lib/auth/session";

/**
 * The authenticated shell is always request-scoped: it reads the session
 * cookie and therefore must never be statically prerendered.
 */
export const dynamic = "force-dynamic";

interface NavChild {
  label: string;
  href: string;
}

interface NavItem {
  label: string;
  href: string;
  /** `false` renders as a muted label: the module has no page yet. */
  ready?: boolean;
  children?: NavChild[];
}

/**
 * Modules from blueprint §4 (capability map). Phase 2 built out Work and People;
 * the remaining modules are listed but marked "soon" rather than linking to a
 * 404, so the shell never promises a page that does not exist.
 */
const NAVIGATION: NavItem[] = [
  { label: "Dashboard", href: "/dashboard", ready: true },
  {
    label: "Work",
    href: "/work",
    ready: true,
    children: [
      { label: "Projects", href: "/work" },
      { label: "Task board", href: "/work/tasks" },
      { label: "Workload", href: "/work/workload" },
      { label: "Escalations", href: "/work/escalations" },
    ],
  },
  {
    label: "People",
    href: "/people",
    ready: true,
    children: [
      { label: "Availability & leave", href: "/people" },
      { label: "Daily reports", href: "/daily-reports" },
    ],
  },
  { label: "CRM", href: "/crm" },
  { label: "Contracts", href: "/contracts" },
  { label: "Finance", href: "/finance" },
  { label: "Systems", href: "/systems" },
  { label: "Units", href: "/units" },
  { label: "Intel", href: "/intel" },
  { label: "Settings", href: "/settings", ready: true },
];

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

        <nav className="flex-1 space-y-1 overflow-y-auto px-3 py-4">
          {NAVIGATION.map((item) =>
            item.ready ? (
              <div key={item.href}>
                <Link
                  href={item.href}
                  className="block rounded-md px-3 py-2 text-sm font-medium text-slate-800 hover:bg-slate-100"
                >
                  {item.label}
                </Link>
                {item.children && (
                  <div className="mt-0.5 space-y-0.5 border-l border-slate-200 pl-3">
                    {item.children.map((child) => (
                      <Link
                        key={child.href}
                        href={child.href}
                        className="block rounded-md px-3 py-1.5 text-sm text-slate-600 hover:bg-slate-100"
                      >
                        {child.label}
                      </Link>
                    ))}
                  </div>
                )}
              </div>
            ) : (
              <div
                key={item.href}
                className="flex items-center justify-between rounded-md px-3 py-2 text-sm text-slate-400"
                title="Not built yet"
              >
                <span>{item.label}</span>
                <span className="rounded-full bg-slate-100 px-1.5 text-[10px] font-medium uppercase text-slate-400">
                  soon
                </span>
              </div>
            ),
          )}
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
          Company operating system
        </header>
        <main className="flex-1 px-6 py-6">{children}</main>
      </div>
    </div>
  );
}

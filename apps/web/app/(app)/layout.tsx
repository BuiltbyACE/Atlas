import type { ReactNode } from "react";

import { GooFilter } from "@/components/blob-button";
import { PresenceHeartbeat } from "@/components/presence-heartbeat";
import { Sidebar } from "@/components/shell/sidebar";
import { TopBar } from "@/components/shell/top-bar";
import { ToastProvider } from "@/components/toast";
import { listEscalations } from "@/lib/api/atlas";
import { requireUser } from "@/lib/auth/session";

/**
 * The authenticated shell is always request-scoped: it reads the session
 * cookie and therefore must never be statically prerendered.
 */
export const dynamic = "force-dynamic";

/**
 * The open-escalations count for the sidebar badge. One keyset page is enough
 * to say "99+"; a viewer who cannot read escalations simply gets no badge.
 */
async function openEscalationsLabel(): Promise<string | undefined> {
  try {
    const page = await listEscalations({ status: "open", limit: 100 });
    if (page.items.length === 0) return undefined;
    return page.has_more || page.items.length > 99 ? "99+" : String(page.items.length);
  } catch {
    return undefined;
  }
}

/**
 * The workspace shell (prototype `.ax`): the navy sidebar on desktop, a sticky
 * top bar, and the page. Below `lg` the sidebar moves into a drawer opened from
 * the top bar.
 */
export default async function AppLayout({ children }: { children: ReactNode }) {
  // The real check is enforced by Go on every request; this only decides
  // whether to render the shell or bounce to /login.
  const user = await requireUser();
  const escalations = await openEscalationsLabel();

  return (
    <ToastProvider>
      <div className="app-shell min-h-screen bg-canvas text-[15px] leading-[1.45] text-night lg:grid lg:grid-cols-[264px_minmax(0,1fr)]">
        <PresenceHeartbeat />
        <GooFilter />
        <div className="sticky top-0 hidden h-screen lg:block">
          <Sidebar userName={user.name} userEmail={user.email} escalations={escalations} />
        </div>

        <div className="flex min-w-0 flex-col">
          <TopBar
            userId={user.id}
            userName={user.name}
            userEmail={user.email}
            escalations={escalations}
          />
          <main className="mx-auto w-full max-w-[1360px] px-4 pb-14 pt-5 sm:px-8 sm:pt-7">
            {children}
          </main>
        </div>
      </div>
    </ToastProvider>
  );
}

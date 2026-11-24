import type { ReactNode } from "react";

import { GooFilter } from "@/components/blob-button";
import { PresenceHeartbeat } from "@/components/presence-heartbeat";
import { AppShell } from "@/components/shell/app-shell";
import { loadNavigation } from "@/components/shell/navigation";
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
 *
 * The destinations come from Go (`GET /api/v1/me/navigation`), already filtered
 * by the caller's effective permissions. `loadNavigation` falls back to a
 * dashboard-and-settings map if that endpoint is unavailable, so a degraded
 * response can hide controls but never offer an unauthorised one.
 */
export default async function AppLayout({ children }: { children: ReactNode }) {
  // The real check is enforced by Go on every request; this only decides
  // whether to render the shell or bounce to /login.
  const user = await requireUser();
  const model = await loadNavigation();

  // Only fetch the count when Go put a badge-bearing destination in the map.
  const wantsEscalations = model.items.some((item) => item.badge === "escalations");
  const escalations = wantsEscalations ? await openEscalationsLabel() : undefined;

  return (
    <ToastProvider>
      <PresenceHeartbeat />
      <GooFilter />
      <AppShell
        user={{ id: user.id, name: user.name, email: user.email }}
        model={model}
        escalations={escalations}
      >
        {children}
      </AppShell>
    </ToastProvider>
  );
}

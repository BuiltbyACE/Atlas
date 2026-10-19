"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Icon } from "@/components/icons";
import { ApiError } from "@/lib/api/client";
import { logout } from "@/lib/auth/flow";
import { disconnectSocket } from "@/lib/realtime/socket";

/**
 * Ends the current session (`POST /api/v1/auth/logout`); Go revokes it and
 * clears the HttpOnly cookie. A 401 means the session was already gone, which
 * is the outcome we wanted, so it also goes to /login. The realtime socket is
 * dropped too, so it does not keep retrying with the old session's token.
 *
 * `compact` renders the sidebar's icon button.
 */
export function SignOutButton({ compact = false }: { compact?: boolean }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);

  async function signOut() {
    if (busy) return;
    setBusy(true);
    setFailed(false);
    try {
      await logout();
    } catch (error) {
      if (!(error instanceof ApiError && error.status === 401)) {
        setFailed(true);
        setBusy(false);
        return;
      }
    }
    disconnectSocket();
    // /login renders per request, so it sees the cleared cookie.
    router.replace("/login");
  }

  if (compact) {
    return (
      <button
        type="button"
        onClick={signOut}
        disabled={busy}
        aria-label={failed ? "Couldn’t sign out. Try again" : "Sign out"}
        title={failed ? "Couldn’t sign out. Try again." : "Sign out"}
        className={
          "ml-auto grid h-10 w-10 flex-none place-items-center rounded-[10px] transition hover:bg-white/10 hover:text-white disabled:cursor-progress disabled:opacity-60 " +
          (failed ? "text-[#FFB3A5]" : "text-[#B9CAE6]")
        }
      >
        <Icon name="signOut" />
      </button>
    );
  }

  return (
    <div>
      <button
        type="button"
        onClick={signOut}
        disabled={busy}
        className="w-full rounded-md border border-slate-200 px-3 py-1.5 text-left text-xs font-medium text-slate-700 transition hover:bg-milk-100 disabled:cursor-progress disabled:opacity-60"
      >
        {busy ? "Signing out…" : "Sign out"}
      </button>
      {failed && (
        <p role="alert" className="mt-1 text-xs text-red-700">
          Couldn’t sign out. Try again.
        </p>
      )}
    </div>
  );
}

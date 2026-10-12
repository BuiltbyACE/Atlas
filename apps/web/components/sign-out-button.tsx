"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { ApiError } from "@/lib/api/client";
import { logout } from "@/lib/auth/flow";

/**
 * Ends the current session (`POST /api/v1/auth/logout`); Go revokes it and
 * clears the HttpOnly cookie. A 401 means the session was already gone, which
 * is the outcome we wanted, so it also goes to /login.
 */
export function SignOutButton() {
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
    // /login renders per request, so it sees the cleared cookie.
    router.replace("/login");
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

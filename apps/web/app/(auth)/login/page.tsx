import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Sign in · Atlas",
  description: "Sign in to Atlas, the SafariStack Company Operating System.",
};

/**
 * Phase 1 placeholder: Atlas login shell.
 *
 * This is intentionally a shell with NO business logic. The real flow (Phase 1)
 * will be a client component that:
 *   1. POSTs credentials to `/api/v1/auth/login` (Go) with the CSRF token,
 *   2. calls `/api/v1/auth/mfa/verify` when the account requires TOTP,
 *   3. and only then receives the session cookie.
 *
 * Authentication is owned by Go; Next.js never validates credentials itself
 * (blueprint sections 5.2 and 5.7, ADR-008).
 */
export default function LoginPage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-100 px-4 py-12">
      <div className="w-full max-w-sm rounded-xl border border-slate-200 bg-white p-8 shadow-sm">
        <h1 className="text-xl font-semibold tracking-tight">Atlas</h1>
        <p className="mt-1 text-sm text-slate-500">
          SafariStack Company Operating System
        </p>

        <div className="mt-4 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          <strong className="font-semibold">Phase 1 placeholder.</strong>{" "}
          Authentication is not wired up yet. This form is a shell only; it will
          POST to <code className="font-mono">/api/v1/auth/login</code>.
        </div>

        {/*
          `method="post"` points straight at the Go endpoint for documentation
          purposes only. Phase 1 replaces this with a client component that uses
          the generated API client (lib/api/client.ts).
        */}
        <form
          className="mt-6 space-y-4"
          action="/api/v1/auth/login"
          method="post"
        >
          <div>
            <label
              htmlFor="email"
              className="block text-sm font-medium text-slate-700"
            >
              Email
            </label>
            <input
              id="email"
              name="email"
              type="email"
              autoComplete="username"
              required
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-slate-500 focus:outline-none"
            />
          </div>

          <div>
            <label
              htmlFor="password"
              className="block text-sm font-medium text-slate-700"
            >
              Password
            </label>
            <input
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              required
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-slate-500 focus:outline-none"
            />
          </div>

          <div>
            <label
              htmlFor="totp"
              className="block text-sm font-medium text-slate-700"
            >
              Authenticator code
            </label>
            <input
              id="totp"
              name="totp"
              inputMode="numeric"
              pattern="[0-9]*"
              maxLength={6}
              autoComplete="one-time-code"
              placeholder="6-digit code"
              className="mt-1 w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-slate-500 focus:outline-none"
            />
            <p className="mt-1 text-xs text-slate-500">
              TOTP is mandatory for privileged roles (section 13.1).
            </p>
          </div>

          <button
            type="submit"
            className="w-full rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700"
          >
            Sign in
          </button>
        </form>
      </div>
    </main>
  );
}

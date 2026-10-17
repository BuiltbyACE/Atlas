import type { Metadata } from "next";

import { InviteForm } from "./invite-form";

export const metadata: Metadata = {
  title: "Accept your invitation · Atlas",
  description: "Finish setting up your Atlas account.",
};

/** Reads the invitation token from the query string, so it is request-scoped. */
export const dynamic = "force-dynamic";

/**
 * Invitation acceptance (docs/FRONTEND.md section 2). Public: the invitee has
 * no Atlas account yet. The token is issued by an administrator
 * (`POST /api/v1/auth/invites`) and carried here as `?token=...`.
 */
export default async function InvitePage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string | string[] }>;
}) {
  const params = await searchParams;
  const raw = params.token;
  const token = (Array.isArray(raw) ? raw[0] : raw)?.trim() ?? "";

  return (
    <main className="flex min-h-svh items-center justify-center bg-milk px-5 py-16">
      <div className="w-full max-w-md">
        <InviteForm token={token} />
      </div>
    </main>
  );
}

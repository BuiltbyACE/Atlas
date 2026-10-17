import type { Metadata } from "next";

import { Card, PageHeader } from "@/components/ui";

import { SessionsPanel } from "./sessions-panel";

export const metadata: Metadata = {
  title: "Settings · Atlas",
  description: "Your Atlas account and the company policy that governs it.",
};

/**
 * Account settings. Session management is live (the backend has always had
 * it); company policy is still a placeholder because the policy store and its
 * values (blueprint section 6.7, decisions D4/D5) are not decided yet.
 */
export default function SettingsPage() {
  return (
    <div className="max-w-3xl space-y-6">
      <PageHeader
        title="Settings"
        description="Your account and the company policy that governs Atlas."
      />

      <SessionsPanel />

      <Card
        title="Company policy"
        description="Configuration, not code (blueprint section 6.7)."
      >
        <p className="text-sm text-slate-600">
          Approval thresholds, escalation windows and retention policies will be
          edited here once the policy store is implemented. Changes will require
          both CEOs.
        </p>
      </Card>
    </div>
  );
}

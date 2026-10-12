const CARDS = [
  {
    title: "Staff status",
    description:
      "Who has reported in, who is blocked and who is missing a daily report.",
    placeholder: "Waiting for people.daily_report signals.",
  },
  {
    title: "Pending approvals",
    description: "Approvals waiting for you, with amounts and separation of duties.",
    placeholder: "Waiting for approvals API.",
  },
  {
    title: "Open escalations",
    description: "Unresolved L1-L3 items and how long they have been open.",
    placeholder: "Waiting for core.escalation signals.",
  },
  {
    title: "Recent audit highlights",
    description: "Sensitive or unusual changes worth a CEO glance.",
    placeholder: "Waiting for audit stream.",
  },
] as const;

/**
 * Executive dashboard v1 (Phase 1) - shell only.
 *
 * Real data will come from the Go API (`/api/v1/reports/executive/daily`,
 * `/api/v1/approvals`, `/api/v1/me`, ...). This page is a static shell.
 *
 * Live behaviour: the browser joins the `exec:dashboard` Phoenix topic (CEOs
 * only). Per ADR-009 and blueprint section 5.2, that channel pushes *signals*
 * (entity id + change type), never payloads - so the client refetches the real
 * data from Go. Wire this up with `useExecDashboard` from
 * `@/lib/realtime/socket` in a client child component during Phase 1.
 */
export default function DashboardPage() {
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-xl font-semibold tracking-tight">
          Executive dashboard v1 (Phase 1)
        </h1>
        <p className="mt-1 text-sm text-slate-500">
          Staff status, pending approvals, open escalations and recent audit
          highlights - computed by Atlas, not compiled by hand.
        </p>
      </header>

      <div className="rounded-md border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-800">
        Live updates will arrive over the <code className="font-mono">exec:dashboard</code>{" "}
        Phoenix channel as signals only (entity id + change type). The dashboard
        then refetches from the Go API, which enforces permissions (ADR-009).
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {CARDS.map((card) => (
          <section
            key={card.title}
            className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm"
          >
            <h2 className="text-sm font-semibold text-slate-900">{card.title}</h2>
            <p className="mt-1 text-sm text-slate-500">{card.description}</p>
            <div className="mt-4 rounded-md border border-dashed border-slate-300 bg-slate-50 px-3 py-6 text-center text-xs text-slate-400">
              {card.placeholder}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}

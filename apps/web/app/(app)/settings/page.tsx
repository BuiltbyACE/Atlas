export default function SettingsPage() {
  return (
    <div className="max-w-2xl space-y-6">
      <header>
        <h1 className="text-xl font-semibold tracking-tight">Settings</h1>
        <p className="mt-1 text-sm text-slate-500">
          Phase 1 placeholder. Policy changes will require both CEOs.
        </p>
      </header>

      <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-900">Your account</h2>
        <ul className="mt-3 space-y-2 text-sm text-slate-600">
          <li>Profile and contact details</li>
          <li>Notifications and quiet hours</li>
          <li>Sessions and sign out everywhere</li>
          <li>MFA and recovery codes</li>
        </ul>
        <p className="mt-4 text-xs text-slate-400">
          Not wired up yet. Backed by settings and notifications APIs in Phase 1.
        </p>
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-900">
          Company policy
        </h2>
        <p className="mt-2 text-sm text-slate-600">
          Approval thresholds, escalation windows and retention policies live
          here once the policy store is implemented. These are configuration,
          not code (blueprint section 6.7).
        </p>
      </section>
    </div>
  );
}

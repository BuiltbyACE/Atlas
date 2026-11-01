import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Stat,
  StatGrid,
  Table,
  Td,
  Th,
} from "@/components/ui";
import { getScorecard } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { formatMinor, formatMinorCompact } from "@/lib/format";

/**
 * Company Scorecard (§8.9). Cross-module roll-up of company health —
 * financial, operational, commercial, systems and people — for executive
 * review.
 */
export default async function ScorecardPage() {
  let scorecard: Awaited<ReturnType<typeof getScorecard>> | null = null;
  let scorecardError: ApiError | null = null;

  try {
    scorecard = await getScorecard();
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    scorecardError = error;
  }

  if (scorecardError) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Company Scorecard"
          description="Financial, operational, commercial, systems and people health at a glance."
        />
        <ProblemNotice
          error={scorecardError}
          title={
            scorecardError.status === 403
              ? "You do not have permission to view the company scorecard"
              : "Company scorecard could not be loaded"
          }
        />
      </div>
    );
  }

  if (!scorecard) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Company Scorecard"
          description="No data available."
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Company Scorecard"
        description="Financial, operational, commercial, systems and people health at a glance."
      />

      <Card
        title="Financial"
        description="Cash, receivables, expenses and budgets (§8.4)."
      >
        <StatGrid label="Financial health">
          <Stat
            label="Cash on hand"
            value={formatMinorCompact(scorecard.financial.cash_minor)}
            icon="card"
            iconTone="success"
            hint="Across all accounts"
          />
          <Stat
            label="Receivables outstanding"
            value={formatMinorCompact(scorecard.financial.receivables_outstanding_minor)}
            icon="clock"
            iconTone={
              scorecard.financial.receivables_outstanding_minor > 0
                ? "warning"
                : "neutral"
            }
            hint="Money owed by clients"
          />
          <Stat
            label="Overdue invoices"
            value={scorecard.financial.overdue_invoice_count.toString()}
            icon="alert"
            iconTone="danger"
            hint={`${formatMinorCompact(scorecard.financial.receivables_overdue_minor)} overdue`}
          />
          <Stat
            label="Expenses (30 days)"
            value={formatMinorCompact(scorecard.financial.expenses_30d_minor)}
            icon="sliders"
            iconTone="neutral"
            hint="Spending over the trailing month"
          />
          <Stat
            label="Pending expenses"
            value={scorecard.financial.pending_expenses.toString()}
            icon="clipboard"
            iconTone="warning"
            hint="Awaiting approval"
          />
          <Stat
            label="Budgets over threshold"
            value={scorecard.financial.budgets_over_threshold.toString()}
            icon="alert"
            iconTone="warning"
            hint="Above 90% of approved amount"
          />
        </StatGrid>
      </Card>

      <Card
        title="Operational"
        description="Projects, tasks and escalations (§5.8, §8.7)."
      >
        <StatGrid label="Operational health">
          <Stat
            label="Active projects"
            value={scorecard.operational.active_projects.toString()}
            icon="folder"
            iconTone="info"
            hint={`${scorecard.operational.planning_projects} planning`}
          />
          <Stat
            label="Overdue tasks"
            value={scorecard.operational.overdue_tasks.toString()}
            icon="alert"
            iconTone="danger"
            hint="Past deadline, not done"
          />
          <Stat
            label="Open escalations"
            value={scorecard.operational.open_escalations.toString()}
            icon="shieldCheck"
            iconTone="warning"
            hint="Needs executive attention"
          />
          <Stat
            label="Tasks completed (30d)"
            value={scorecard.operational.completed_tasks_30d.toString()}
            icon="check"
            iconTone="success"
            hint="Trailing month"
          />
        </StatGrid>
      </Card>

      <Card
        title="Commercial"
        description="Pipeline, opportunities and leads (§8.1)."
      >
        <StatGrid label="Commercial health">
          <Stat
            label="Open pipeline"
            value={formatMinorCompact(scorecard.commercial.pipeline_value_minor)}
            icon="pie"
            iconTone="info"
            hint={`${scorecard.commercial.open_opportunities} open opportunities`}
          />
          <Stat
            label="Won (30 days)"
            value={scorecard.commercial.won_opportunities_30d.toString()}
            icon="check"
            iconTone="success"
            hint={`${scorecard.commercial.lost_opportunities_30d} lost`}
          />
          <Stat
            label="Open leads"
            value={scorecard.commercial.open_leads.toString()}
            icon="people"
            iconTone="accent"
            hint={`${scorecard.commercial.qualified_leads_30d} qualified (30d)`}
          />
        </StatGrid>
      </Card>

      <Card title="Systems" description="Register, checks, incidents and availability (§8.6).">
        <StatGrid label="Systems health">
          <Stat
            label="Systems registered"
            value={scorecard.systems.systems_total.toString()}
            icon="server"
            iconTone="info"
            hint={`${scorecard.systems.systems_tier_1} T1 · ${scorecard.systems.systems_tier_2} T2 · ${scorecard.systems.systems_tier_3} T3`}
          />
          <Stat
            label="Failing checks"
            value={scorecard.systems.checks_failing.toString()}
            icon="alert"
            iconTone="danger"
            hint={`${scorecard.systems.checks_warning} warning · ${scorecard.systems.checks_unprobed} unprobed`}
          />
          <Stat
            label="Open incidents"
            value={scorecard.systems.open_incidents.toString()}
            icon="bell"
            iconTone="warning"
            hint={`${scorecard.systems.open_sev_1} sev-1 · ${scorecard.systems.open_sev_2} sev-2`}
          />
          <Stat
            label="Availability"
            value={(scorecard.systems.availability_bp / 100).toFixed(2) + "%"}
            icon="shieldCheck"
            iconTone="success"
            hint="Rolling 90 days"
          />
        </StatGrid>
      </Card>

      <Card title="People" description="Headcount, attendance and leave (§8.5).">
        <StatGrid label="People health">
          <Stat
            label="Headcount"
            value={scorecard.people.headcount.toString()}
            icon="people"
            iconTone="info"
            hint={`${scorecard.people.active} active · ${scorecard.people.on_leave} on leave`}
          />
          <Stat
            label="Onboarding"
            value={scorecard.people.onboarding.toString()}
            icon="plus"
            iconTone="accent"
            hint="New hires not yet active"
          />
          <Stat
            label="Missed reports (30d)"
            value={scorecard.people.missed_reports_30d.toString()}
            icon="clipboard"
            iconTone="warning"
            hint="Daily reports not filed"
          />
          <Stat
            label="Approved leave today"
            value={scorecard.people.approved_leave_today.toString()}
            icon="calendar"
            iconTone="neutral"
            hint="Out of office today"
          />
        </StatGrid>
      </Card>

      {scorecard.financial.budgets_over_threshold > 0 ||
      scorecard.systems.checks_failing > 0 ||
      scorecard.systems.open_incidents > 0 ||
      scorecard.operational.overdue_tasks > 0 ||
      scorecard.operational.open_escalations > 0 ? (
        <EmptyState
          icon="alert"
          iconTone="warning"
          title="Items need attention"
        >
          One or more scorecard metrics are outside healthy thresholds. Review
          the cards above and follow up on the flagged items.
        </EmptyState>
      ) : null}
    </div>
  );
}

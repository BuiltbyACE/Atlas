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
  type Tone,
} from "@/components/ui";
import { getDailyBrief } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { formatDay, formatMinor, humanise } from "@/lib/format";

/**
 * Daily Executive Brief (§8.8). Shows yesterday's results, items needing
 * attention, today's upcoming items, and decisions awaiting the CEOs.
 */
export default async function DailyBriefPage() {
  let brief: Awaited<ReturnType<typeof getDailyBrief>> | null = null;
  let briefError: ApiError | null = null;

  try {
    brief = await getDailyBrief();
  } catch (error) {
    if (!(error instanceof ApiError)) throw error;
    briefError = error;
  }

  if (briefError) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Daily Brief"
          description="Yesterday's results, items needing attention, and today's upcoming items."
        />
        <ProblemNotice
          error={briefError}
          title={
            briefError.status === 403
              ? "You do not have permission to view the daily brief"
              : "Daily brief could not be loaded"
          }
        />
      </div>
    );
  }

  if (!brief) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Daily Brief"
          description="No data available."
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Daily Brief"
        description={new Date(brief!.for_date).toLocaleDateString(undefined, {
          weekday: "long",
          year: "numeric",
          month: "long",
          day: "numeric",
        })}
      />

      <StatGrid label="Yesterday's Results">
        <Stat
          label="Deals Won"
          value={brief.yesterday.deals_won.toString()}
          icon="pie"
          iconTone="success"
          hint={`${brief.yesterday.deals_lost} lost`}
        />
        <Stat
          label="Cash Received"
          value={formatMinor(brief.yesterday.payments_received_minor)}
          icon="card"
          iconTone="success"
          hint="Received yesterday"
        />
        <Stat
          label="Tasks Completed"
          value={brief.yesterday.tasks_completed.toString()}
          icon="check"
          iconTone="success"
          hint="Completed yesterday"
        />
        <Stat
          label="Incidents"
          value={brief.yesterday.incidents_opened.toString()}
          icon="alert"
          iconTone="warning"
          hint={`${brief.yesterday.incidents_resolved} resolved`}
        />
      </StatGrid>

      <Card
        title="Needs Attention"
        description="Items requiring executive attention."
      >
        <Table>
          <thead>
            <tr>
              <Th>Category</Th>
              <Th className="text-right">Count</Th>
              <Th className="text-right">Value</Th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <Td>Overdue Tasks</Td>
              <Td className="tabular text-right">
                {brief.needs_attention.overdue_tasks}
              </Td>
              <Td className="text-muted">--</Td>
            </tr>
            <tr>
              <Td>Overdue Invoices</Td>
              <Td className="tabular text-right">
                {brief.needs_attention.overdue_invoices}
              </Td>
              <Td className="tabular text-right">
                {formatMinor(brief.needs_attention.overdue_invoices_minor)}
              </Td>
            </tr>
            <tr>
              <Td>Missed Daily Reports</Td>
              <Td className="tabular text-right">
                {brief.needs_attention.missed_daily_reports}
              </Td>
              <Td className="text-muted">--</Td>
            </tr>
            <tr>
              <Td>Expiring Certificates</Td>
              <Td className="tabular text-right">
                {brief.needs_attention.expiring_certificates}
              </Td>
              <Td className="text-muted">--</Td>
            </tr>
            <tr>
              <Td>Expiring Domains</Td>
              <Td className="tabular text-right">
                {brief.needs_attention.expiring_domains}
              </Td>
              <Td className="text-muted">--</Td>
            </tr>
            <tr>
              <Td>Open Risks</Td>
              <Td className="tabular text-right">
                {brief.needs_attention.open_risks}
              </Td>
              <Td className="text-muted">--</Td>
            </tr>
          </tbody>
        </Table>
      </Card>

      <Card title="Today's Items" description="Items due today.">
        <Table>
          <thead>
            <tr>
              <Th>Type</Th>
              <Th>Description</Th>
              <Th className="text-right">Due</Th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <Td>Follow-ups</Td>
              <Td className="tabular text-right">
                {brief.today.follow_ups_due}
              </Td>
              <Td className="text-muted">Today</Td>
            </tr>
            <tr>
              <Td>Milestones</Td>
              <Td className="tabular text-right">{brief.today.milestones_due}</Td>
              <Td className="text-muted">Today</Td>
            </tr>
            <tr>
              <Td>People Absent</Td>
              <Td className="tabular text-right">{brief.today.people_absent}</Td>
              <Td className="text-muted">Today</Td>
            </tr>
          </tbody>
        </Table>
      </Card>

      {brief.decisions.length > 0 ? (
        <Card
          title="Decisions Awaiting"
          description="Decisions awaiting your input."
        >
          <Table>
            <thead>
              <tr>
                <Th>Ref</Th>
                <Th>Question</Th>
                <Th>Outcome</Th>
                <Th className="text-right">Due</Th>
              </tr>
            </thead>
            <tbody>
              {brief.decisions.map((d) => {
                const tone: Tone =
                  d.outcome === "approved" ? "success" : "neutral";
                return (
                  <tr key={d.id}>
                    <Td>{d.ref}</Td>
                    <Td>{d.question}</Td>
                    <Td>
                      <Badge tone={tone}>
                        {humanise(d.outcome)}
                      </Badge>
                    </Td>
                    <Td className="tabular text-right">
                      {d.due_on ? formatDay(d.due_on) : "—"}
                    </Td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </Card>
      ) : (
        <EmptyState
          icon="check"
          iconTone="success"
          title="No decisions awaiting"
        >
          All open decisions have been resolved.
        </EmptyState>
      )}
    </div>
  );
}

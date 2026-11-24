import { notFound } from "next/navigation";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import {
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Table,
  Td,
  Th,
} from "@/components/ui";
import { listEmailThreads, type ThreadListItem } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDay, humanise } from "@/lib/format";

/**
 * Email inbox (§8.4). List of email threads, newest first.
 * Each thread links to its conversation view.
 */
export default async function EmailInboxPage() {
  const user = await requireUser();

  let threads: ThreadListItem[] = [];
  let error: ApiError | null = null;
  try {
    threads = await listEmailThreads();
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Inbox"
          description="Your email threads, newest first."
        />
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view email"
              : "Your inbox could not be loaded"
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Inbox"
        description="Your email threads, newest first."
        actions={
          <ButtonLink href="/email/compose" variant="primary">
            <Icon name="plus" className="h-4 w-4" />
            Compose
          </ButtonLink>
        }
      />

      {threads.length === 0 ? (
        <div className="p-[22px]">
          <EmptyState
            icon="document"
            iconTone="info"
            className="min-h-[240px]"
            title="No email threads"
          >
            Your inbox is empty. Compose your first message or wait for
            inbound mail to arrive.
          </EmptyState>
        </div>
      ) : (
        <Card>
          <Table>
            <thead>
              <tr>
                <Th>Thread</Th>
                <Th>From</Th>
                <Th className="text-right">Messages</Th>
                <Th className="text-right">Updated</Th>
              </tr>
            </thead>
            <tbody>
              {threads.map((thread) => (
                <tr key={thread.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <a
                        href={`/email/threads/${thread.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        {thread.subject || "(no subject)"}
                      </a>
                    </div>
                  </Td>
                  <Td className="text-muted">{thread.latest?.from_address ?? "—"}</Td>
                  <Td className="tabular text-right text-muted">
                    {thread.message_count}
                  </Td>
                  <Td className="text-muted">{formatDay(thread.updated_at)}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Card>
      )}
    </div>
  );
}

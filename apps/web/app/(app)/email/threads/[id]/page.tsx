import { notFound } from "next/navigation";

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
import { getEmailThread, type EmailMessage } from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDay, formatDateTime, humanise } from "@/lib/format";

/**
 * Email thread view (§8.4). The conversation view for one thread:
 * all messages in the thread, newest last.
 */
export default async function EmailThreadPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const user = await requireUser();

  let messages: EmailMessage[];
  let error: ApiError | null = null;
  try {
    messages = await getEmailThread(id);
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
    messages = [];
  }

  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader title="Message" description="Email conversation." />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That message thread does not exist"
              : "The conversation could not be loaded"
          }
        />
      </div>
    );
  }

  if (messages.length === 0) {
    notFound();
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={messages[0].subject || "(no subject)"}
        description={`Email thread · ${messages.length} message${messages.length === 1 ? "" : "s"}`}
      />

      <div className="space-y-6">
        {messages.map((message) => (
          <Card key={message.id} padded={false}>
            <div className="px-[22px] py-[16px] border-b border-line last:border-b-0">
              <div className="flex flex-wrap items-center gap-2 text-[12px] text-muted">
                <span
                  className={`
                    font-semibold ${
                      message.direction === "outbound"
                        ? "text-atlas"
                        : "text-night"
                    }
                  `}
                >
                  {message.direction === "outbound" ? "You" : humanise(message.from_address)}
                </span>
                <span aria-hidden="true">·</span>
                <span>{formatDateTime(message.sent_at ?? message.created_at)}</span>
                <span className="font-semibold text-[10px] uppercase tracking-[0.08em]">
                  {message.direction === "outbound" ? "Sent" : "Received"}
                </span>
              </div>
              <div className="mt-[10px] whitespace-pre-line text-[14.5px] leading-relaxed">
                {message.body_text}
              </div>
            </div>
          </Card>
        ))}
      </div>
    </div>
  );
}

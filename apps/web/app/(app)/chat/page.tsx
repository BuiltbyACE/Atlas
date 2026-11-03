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
import {
  listChatConversations,
  type ConversationView,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDay, humanise } from "@/lib/format";

/**
 * Chat conversations (§8.5). List of your conversations, newest first.
 * Each links to the active chat view.
 */
export default async function ChatPage() {
  const user = await requireUser();

  let conversations: ConversationView[] = [];
  let error: ApiError | null = null;
  try {
    conversations = await listChatConversations();
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
  }

  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader title="Messages" description="Your conversations, newest first." />
        <ProblemNotice
          error={error}
          title={
            error.status === 403
              ? "You do not have permission to view messages"
              : "Your conversations could not be loaded"
          }
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Messages"
        description="Your conversations, newest first."
        actions={
          <ButtonLink href="/chat/new" variant="primary">
            <Icon name="plus" className="h-4 w-4" />
            New message
          </ButtonLink>
        }
      />

      {conversations.length === 0 ? (
        <div className="p-[22px]">
          <EmptyState
            icon="document"
            iconTone="info"
            className="min-h-[240px]"
            title="No conversations"
          >
            Start your first conversation. CEOs can message any employee;
            employees can message CEOs.
          </EmptyState>
        </div>
      ) : (
        <Card>
          <Table>
            <thead>
              <tr>
                <Th>Conversation</Th>
                <Th>Last message</Th>
                <Th className="text-right">Unread</Th>
                <Th className="text-right">Updated</Th>
              </tr>
            </thead>
            <tbody>
              {conversations.map((conv) => (
                <tr key={conv.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <a
                        href={`/chat/${conv.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        {conv.name ||
                          (conv.type === "direct" ? "Direct message" : "Group chat")}
                      </a>
                      {conv.type === "direct" && (
                        <span className="text-[11.5px] text-muted">1:1 chat</span>
                      )}
                    </div>
                  </Td>
                  <Td>
                    {conv.last_message ? (
                      <div className="flex flex-col gap-0.5">
                        <span className="line-clamp-1 max-w-[32ch]">
                          {conv.last_message.body}
                        </span>
                        <span className="text-[11.5px] text-muted">
                          {humanise(conv.last_message.sender_name ?? "Someone")}
                        </span>
                      </div>
                    ) : (
                      <span className="text-muted">—</span>
                    )}
                  </Td>
                  <Td className="tabular text-right text-muted">
                    {conv.unread_count > 0 ? conv.unread_count : "—"}
                  </Td>
                  <Td className="text-muted">{formatDay(conv.updated_at)}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Card>
      )}
    </div>
  );
}

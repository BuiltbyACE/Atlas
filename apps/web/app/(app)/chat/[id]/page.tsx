import { notFound } from "next/navigation";

import { Button, Field, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import {
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
} from "@/components/ui";
import {
  listChatMessages,
  sendChatMessage,
  type ChatMessage,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { formatDateTime, humanise } from "@/lib/format";

/**
 * Active chat (§8.5). View a conversation's messages and send new ones.
 * Messages are loaded server-side; new messages are sent via POST and the
 * page refreshes to show the update.
 */
export default async function ChatThreadPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id: conversationId } = await params;
  const user = await requireUser();

  let messages: ChatMessage[];
  let error: ApiError | null = null;
  try {
    messages = await listChatMessages(conversationId);
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    error = caught;
    messages = [];
  }

  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader title="Message" description="Conversation." />
        <ProblemNotice
          error={error}
          title={
            error.status === 404
              ? "That conversation does not exist"
              : "The conversation could not be loaded"
          }
        />
      </div>
    );
  }

  if (messages.length === 0) {
    notFound();
  }

  // Messages come newest first; reverse for display.
  const displayMessages = [...messages].reverse();

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const bodyInput = form.elements.namedItem("body") as HTMLInputElement;
    const body = bodyInput.value.trim();
    if (!body) return;

    try {
      await sendChatMessage(conversationId, { body, body_type: "text" });
      bodyInput.value = "";
      // Refresh to show the new message
      window.location.reload();
    } catch (caught) {
      if (caught instanceof ApiError) {
        const el = form.elements.namedItem("send-error") as HTMLElement;
        if (el) el.textContent = caught.message || "Send failed";
      }
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Conversation"
        description={`Conversation · ${messages.length} message${messages.length === 1 ? "" : "s"}`}
      />

      <div className="space-y-4 max-w-2xl mx-auto">
        {displayMessages.map((message) => (
          <Card key={message.id} className="p-[16px]">
            <div className="flex items-start gap-2">
              <div className="flex-shrink-0">
                <Icon
                  name="user"
                  className={`h-4 w-4 ${
                    message.sender_id === user.person_id
                      ? "text-atlas"
                      : "text-muted"
                  }`}
                />
              </div>
              <div className="flex-1">
                <div className="flex items-baseline gap-2 text-[12px]">
                  <span className="font-semibold">
                    {message.sender_id === user.person_id
                      ? "You"
                      : humanise(message.sender_name ?? "Someone")}
                  </span>
                  <span className="text-muted">
                    {formatDateTime(message.created_at)}
                  </span>
                </div>
                <div className="mt-[6px] whitespace-pre-wrap text-[14.5px] leading-relaxed">
                  {message.body}
                </div>
              </div>
            </div>
          </Card>
        ))}
      </div>

      <Card className="max-w-2xl mx-auto p-[16px]">
        <form onSubmit={onSubmit} className="flex gap-2">
          <Field label="" htmlFor="chat-body">
            <TextInput
              id="chat-body"
              name="body"
              placeholder="Type a message…"
              required
              className="flex-1"
            />
          </Field>
          <Button type="submit" variant="primary">
            <Icon name="arrowRight" className="h-4 w-4" />
          </Button>
        </form>
      </Card>
    </div>
  );
}

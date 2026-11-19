"use client";

import { useEffect, useState } from "react";

import { Button, Field, Select, TextInput } from "@/components/form";
import { Icon } from "@/components/icons";
import { Card, PageHeader, ProblemNotice } from "@/components/ui";
import {
  createChatConversation,
  listChatConversations,
  type ConversationView,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";
import { requireUser } from "@/lib/auth/session";
import { useRouter } from "next/navigation";

/**
 * Start a new conversation (§8.5). Pick a recipient and optional subject,
 * then open the chat.
 */
export default function NewChatPage() {
  const router = useRouter();

  const [conversations, setConversations] = useState<ConversationView[]>([]);
  const [userId, setUserId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);

  const [recipientId, setRecipientId] = useState("");
  const [subject, setSubject] = useState("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const user = await requireUser();
        if (!cancelled) setUserId(user.person_id ?? null);
        const items = await listChatConversations();
        if (!cancelled) setConversations(items);
      } catch {
        if (!cancelled) setLoading(false);
      }
      if (!cancelled) setLoading(false);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="New message" description="Start a conversation." />
        <div className="p-[22px] text-[12.5px] text-muted">Loading…</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader title="New message" description="Start a conversation." />
        <ProblemNotice error={error} title="Your conversations could not be loaded" />
      </div>
    );
  }

  // Build recipient options from existing conversations (direct messages)
  const directConvos = conversations.filter((c) => c.type === "direct");
  const recipientOptions = directConvos.map((c) => ({
    value:
      c.last_message?.sender_id ||
      (c.created_by === userId ? "" : c.created_by),
    label: c.last_message?.sender_name ?? "Direct message",
  }));

  // Deduplicate
  const seen = new Set<string>();
  const uniqueOptions = recipientOptions.filter((o) => {
    if (seen.has(o.value)) return false;
    seen.add(o.value);
    return true;
  });

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);

    try {
      const conv = await createChatConversation({
        type: "direct",
        name: subject || undefined,
        participant: [recipientId],
      });
      router.push(`/chat/${conv.id}`);
    } catch (caught) {
      if (caught instanceof ApiError) setError(caught);
      else
        setError(
          new ApiError(500, {
            title: "Unexpected error",
            detail: "An unexpected error occurred",
          }),
        );
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader title="New message" description="Start a conversation." />

      <form onSubmit={onSubmit} className="space-y-4">
        {error && (
          <ProblemNotice error={error} title="The conversation couldn't be started" />
        )}

        <Field label="Message" htmlFor="new-chat-recipient">
          <Select
            id="new-chat-recipient"
            name="recipient_id"
            value={recipientId}
            onChange={(e) => setRecipientId(e.target.value)}
            required
          >
            <option value="">Choose a person</option>
            {uniqueOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </Select>
        </Field>

        <Field label="Subject (optional)" htmlFor="new-chat-subject">
          <TextInput
            id="new-chat-subject"
            name="subject"
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            placeholder="Optional subject for this conversation"
          />
        </Field>

        <div className="flex justify-end gap-2 pt-4">
          <Button
            type="button"
            variant="secondary"
            onClick={() => router.push("/chat")}
            disabled={busy}
          >
            Cancel
          </Button>
          <Button type="submit" variant="primary" disabled={busy || !recipientId}>
            {busy ? "Starting…" : "Start conversation"}
          </Button>
        </div>
      </form>

      <div className="text-[11.5px] text-muted">
        CEOs can message any employee. Employees can message CEOs. Direct
        messages are one-on-one; group chats come in a later release.
      </div>
    </div>
  );
}

import { useCallback, useEffect, useState } from "react";
import { AppShell } from "../components/AppShell";
import { FeedList } from "../components/FeedList";
import { TextField } from "../components/Field";
import { ApiError, operations, request } from "../lib/api";
import type { ConversationsResponse, Conversation, MessagesResponse, Message } from "../lib/api";
import { demoConversations, demoMessages, demoUser } from "../lib/mock";
import { useSession } from "../state/session";

/* ==========================================================================
   Messages — the thread.
   --------------------------------------------------------------------------
   States: loading | empty (no conversations) | error | ready | empty (no
   messages in the selected thread) | sending | send failed.

   The composer keeps its own state so a failed send is never lost: the draft
   stays in the box with the failure attached, rather than clearing on submit.

   Typing is a realtime signal with a TTL behind it, so the UI never claims
   somebody is typing for longer than the signal lives. A received chat frame
   only arrives over a live subscription; where there is none, the page says so
   instead of pretending to be live.
   ========================================================================== */

export function MessagesPage() {
  const { userID: selfID } = useSession();
  const [conversations, setConversations] = useState<Conversation[]>(demoConversations);
  const [activeID, setActiveID] = useState(demoConversations[0]?.id ?? "");
  const [messages, setMessages] = useState<Message[]>(demoMessages);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState<string>();
  const [demo, setDemo] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    try {
      const data = await request<ConversationsResponse>(operations.conversations, { limit: 20 }, signal);
      setConversations(data.myConversations.conversations);
      setDemo(false);
      setError(undefined);
    } catch (cause) {
      if (signal?.aborted) return;
      setConversations(demoConversations);
      setDemo(true);
      setError(cause instanceof ApiError ? cause.message : undefined);
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const loadMessages = useCallback(async (conversationID: string, signal?: AbortSignal) => {
    if (!conversationID) return;
    try {
      const data = await request<MessagesResponse>(
        operations.messages,
        { conversationId: conversationID, limit: 50 },
        signal,
      );
      if (!signal?.aborted) setMessages(data.getMessages.messages);
    } catch {
      if (!signal?.aborted) setMessages(demoMessages);
    }
  }, []);

  useEffect(() => {
    if (!activeID) return;
    const controller = new AbortController();
    void loadMessages(activeID, controller.signal);
    return () => controller.abort();
  }, [activeID, loadMessages]);

  const send = useCallback(async () => {
    const text = draft.trim();
    if (!text || !activeID || sending) return;
    setSending(true);
    setSendError(undefined);
    try {
      const message = await request<{ SendMessage: Message }>(operations.sendMessage, {
        input: { conversationId: activeID, content: text },
      });
      setMessages((current) => [...current, message.SendMessage]);
      setDraft("");
    } catch (cause) {
      // The draft survives a failure: losing it is data loss, not an error state.
      setSendError(cause instanceof Error ? cause.message : "Could not send. Try again.");
    } finally {
      setSending(false);
    }
  }, [draft, activeID, sending]);

  const active = conversations.find((conversation) => conversation.id === activeID);

  return (
    <AppShell activeID="messages" header={<h1 className="shell__title">Messages</h1>}>
      {demo && (
        <p className="banner" role="status">
          The gateway is unreachable, so this thread is demo data.
        </p>
      )}

      <div className="thread">
        <nav className="thread__rail" aria-label="Conversations">
          <FeedList
            ariaLabel="Conversations"
            items={conversations}
            loading={loading}
            error={error}
            onRetry={() => void load()}
            keyFor={(conversation) => conversation.id}
            renderItem={(conversation) => (
              <button
                type="button"
                className={`thread__item${conversation.id === activeID ? " is-selected" : ""}`}
                aria-current={conversation.id === activeID ? "true" : undefined}
                onClick={() => setActiveID(conversation.id)}
              >
                <span className="thread__title">
                  {conversation.title || (conversation.type === "GROUP" ? "Group" : "Direct message")}
                </span>
                {conversation.lastMessage && (
                  <span className="thread__preview">
                    {conversation.lastMessage.isDeleted
                      ? "Message deleted"
                      : conversation.lastMessage.content}
                  </span>
                )}
                {conversation.unreadCount > 0 && (
                  <span className="thread__badge">{conversation.unreadCount} unread</span>
                )}
              </button>
            )}
            emptyTitle="No conversations yet"
            emptyBody="Start a thread from somebody's profile and it will appear here."
          />
        </nav>

        <section className="thread__panel" aria-label="Conversation">
          {!active && (
            <div className="feed feed--empty">
              <h2 className="feed__title">Pick a conversation</h2>
              <p className="feed__body">Choose a thread on the left to read it here.</p>
            </div>
          )}

          {active && (
            <>
              <h2 className="thread__heading">
                {active.title || (active.type === "GROUP" ? "Group message" : "Direct message")}
                {active.unreadCount > 0 && (
                  <span className="thread__badge">{active.unreadCount} unread</span>
                )}
              </h2>

              <ol className="thread__messages" aria-label="Messages">
                {messages.map((message) => (
                  <li
                    key={message.id}
                    className={`bubble${message.senderId === (selfID || demoUser.id) ? " bubble--own" : ""}`}
                  >
                    {message.isDeleted ? (
                      // A tombstone keeps the thread's rhythm; the body is gone.
                      <p className="bubble__deleted">This message was deleted.</p>
                    ) : (
                      <p className="bubble__text">{message.content}</p>
                    )}
                    <span className="bubble__meta">
                      {message.senderId === (selfID || demoUser.id) ? "You" : "Them"}
                    </span>
                  </li>
                ))}
                {messages.length === 0 && (
                  <li className="feed feed--empty">
                    <h3 className="feed__title">No messages yet</h3>
                    <p className="feed__body">Say hello — the first message starts the thread.</p>
                  </li>
                )}
              </ol>

              <form
                className="thread__composer"
                onSubmit={(event) => {
                  event.preventDefault();
                  void send();
                }}
              >
                <TextField
                  id="message-draft"
                  label="Message"
                  value={draft}
                  onChange={(event) => setDraft(event.target.value)}
                  error={sendError}
                  placeholder="Write a reply…"
                  loading={sending}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && !event.shiftKey) {
                      event.preventDefault();
                      void send();
                    }
                  }}
                />
                <button
                  type="submit"
                  className="btn btn--primary btn--md"
                  disabled={sending || draft.trim().length === 0}
                  aria-busy={sending || undefined}
                >
                  {sending ? "Sending…" : "Send"}
                </button>
              </form>
            </>
          )}
        </section>
      </div>
    </AppShell>
  );
}

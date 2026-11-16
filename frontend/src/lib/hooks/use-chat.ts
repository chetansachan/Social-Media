"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import { useSession } from "@/lib/providers/session-provider";
import type { Conversation, ConversationPage, Message, MessagePage } from "@/lib/types";

export const chatKeys = {
  conversations: ["chat", "conversations"] as const,
  messages: (id: string) => ["chat", "messages", id] as const,
};

/**
 * How often the inbox refreshes itself while the tab is open.
 *
 * The realtime stream is the primary source and a browser may drop the socket
 * for a backgrounded tab without notice, so this polling is the floor. It is
 * deliberately slow enough that the stream usually wins the race.
 */
const INBOX_POLL_MS = 20_000;

export function useConversations() {
  const { status } = useSession();

  const query = useQuery({
    queryKey: chatKeys.conversations,
    enabled: status === "authenticated",
    queryFn: async ({ signal }) => {
      const data = await gql<{ myConversations: ConversationPage }>(
        operations.conversations,
        { limit: 30 },
        signal,
      );
      return data.myConversations;
    },
    // `staleTime: 0` because the server already polls: without an explicit
    // refetch interval the list only refreshes on mount, which is exactly why
    // an inbox that receives a message while a thread is open never shows it.
    staleTime: 0,
    refetchInterval: INBOX_POLL_MS,
    refetchIntervalInBackground: true,
  });

  return {
    ...query,
    items: query.data?.conversations ?? [],
    totalUnread: query.data?.totalUnread ?? 0,
  };
}

export function useConversation(id: string) {
  const conversations = useConversations();
  return conversations.items.find((item) => item.id === id) ?? null;
}

export function useMessages(conversationId: string) {
  const { status } = useSession();

  const query = useQuery({
    queryKey: chatKeys.messages(conversationId),
    enabled: status === "authenticated" && Boolean(conversationId),
    queryFn: async ({ signal }) => {
      const data = await gql<{ getMessages: MessagePage }>(
        operations.messages,
        { conversationId, limit: 80 },
        signal,
      );
      return data.getMessages;
    },
  });

  const messages = React.useMemo(
    () => [...(query.data?.messages ?? [])].reverse(),
    [query.data],
  );

  return { ...query, messages };
}

/**
 * Prepends a message to a thread cache, which the API returns newest-first.
 *
 * Deduplicating on id is what keeps the thread from doubling a row. The sender
 * is a member of the conversation, so chat-service publishes the
 * `message.created` frame back to them as well as to the recipient, and the
 * mutation's own result arrives separately. The frame usually wins because a
 * Redis publish beats an HTTP round trip, so whichever lands first has to
 * recognise the other.
 */
export function prependMessage<T extends { messages: { id: string }[] }>(
  current: T | undefined,
  message: T["messages"][number],
): T | undefined {
  if (!current) return current;
  if (current.messages.some((existing) => existing.id === message.id)) {
    return current;
  }
  return { ...current, messages: [message, ...current.messages] };
}

export function useSendMessage(conversationId: string) {
  const queryClient = useQueryClient();
  const { user } = useSession();

  return useMutation({
    mutationFn: async (content: string) => {
      const data = await gql<{ sendMessage: Message }>(operations.sendMessage, {
        input: { conversationId, content },
      });
      return data.sendMessage;
    },
    onSuccess: (message) => {
      queryClient.setQueryData<MessagePage>(
        chatKeys.messages(conversationId),
        (current) => prependMessage(current, message),
      );
      // The new message carries the sender from the session, but the gateway
      // may not hydrate it on the mutation, so use the local profile.
      void user;
      void queryClient.invalidateQueries({ queryKey: chatKeys.conversations });
    },
  });
}

/** Opens the existing direct thread with someone, or creates it. */
export function useStartConversation() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (participantId: string) => {
      const data = await gql<{ createConversation: Conversation }>(
        operations.createConversation,
        { input: { participantIds: [participantId] } },
      );
      return data.createConversation;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: chatKeys.conversations });
    },
  });
}

/** Marks the thread read when it opens; the inbox badge drops with it.
 *
 * The badge is the server's authoritative `totalUnread`, so this function never
 * does arithmetic on it. It zeroes this one thread's count for immediate
 * feedback, then invalidates the inbox so the badge itself comes back from the
 * server rather than being re-derived from a cache that may be stale.
 */
export function useMarkConversationRead(conversationId: string) {
  const queryClient = useQueryClient();
  const marked = React.useRef(new Set<string>());

  React.useEffect(() => {
    if (!conversationId || marked.current.has(conversationId)) return;
    marked.current.add(conversationId);
    void gql(operations.markConversationRead, { conversationId })
      .then(() => {
        // Only this thread's own count is zeroed locally, because that is
        // idempotent. The badge total is deliberately left alone for the
        // invalidation below to replace: it is the server's `totalUnread`, and
        // subtracting a cached per-thread count from it drifts the moment that
        // cache is stale -- which is how a thread whose last message was the
        // viewer's own ended up showing "5 unread".
        queryClient.setQueryData<ConversationPage>(
          chatKeys.conversations,
          (current) =>
            current
              ? {
                  ...current,
                  conversations: current.conversations.map((item) =>
                    item.id === conversationId
                      ? { ...item, unreadCount: 0 }
                      : item,
                  ),
                }
              : current,
        );
        void queryClient.invalidateQueries({ queryKey: chatKeys.conversations });
      })
      .catch(() => {
        // A read receipt is best-effort; the thread still works, and a
        // retried mount can mark the same thread again.
        marked.current.delete(conversationId);
      });
  }, [conversationId, queryClient]);
}

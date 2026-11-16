import type { Conversation, ConversationParticipant } from "./types";

/**
 * Display rules for a conversation.
 *
 * `Conversation.title` is empty for a direct thread, so the name comes from the
 * other participant. Deriving it from `lastMessage.senderId` is wrong the
 * moment the last message is yours.
 */
export function conversationDisplay(
  conversation: Conversation | null | undefined,
  viewerId: string | undefined,
): { name: string; handle: string } {
  if (!conversation) return { name: "Conversation", handle: "conversation" };
  if (conversation.title) return { name: conversation.title, handle: "group" };
  const other = otherParticipant(conversation, viewerId);
  if (other) return { name: other.user.name, handle: other.user.username };
  return { name: "Conversation", handle: "conversation" };
}

export function otherParticipant(
  conversation: Conversation,
  viewerId: string | undefined,
): ConversationParticipant | undefined {
  if (conversation.title) return undefined;
  return conversation.participants.find((participant) => participant.userId !== viewerId);
}

/** The person whose presence is relevant in the thread header. */
export function presenceUserId(
  conversation: Conversation | null | undefined,
  viewerId: string | undefined,
): string | undefined {
  if (!conversation || conversation.title) return undefined;
  return otherParticipant(conversation, viewerId)?.userId;
}

export function conversationPreview(conversation: Conversation): string {
  if (conversation.lastMessage?.isDeleted) return "Message deleted";
  return conversation.lastMessage?.content || "No messages yet";
}

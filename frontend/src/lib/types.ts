/* ==========================================================================
   Domain types — transcribed from backend/api-gateway/graph/schema.graphqls.
   --------------------------------------------------------------------------
   Every field here exists in the schema and every field in the schema that a
   screen needs is here. Nothing is invented: an earlier draft of this file had
   a `bio` and `joinedAt` on User, which the schema does not have.
   ========================================================================== */

export type User = {
  id: string;
  username: string;
  name: string;
  email: string;
  isPublic: boolean;
  createdAt: string;
  updatedAt: string;
};

/** `Post` — returned by search, getAllPost, CreatePost, getPostById. */
export type Post = {
  id: string;
  authorId: string;
  content: string;
  mediaUrl: string;
  isPublic: boolean;
  likeCount: number;
  hasLiked: boolean;
  createdAt: string;
  updatedAt: string;
  author: User;
};

/** `FeedPost` — the feed's own type. Same fields minus `updatedAt`. */
export type FeedPost = {
  id: string;
  authorId: string;
  content: string;
  mediaUrl: string;
  isPublic: boolean;
  likeCount: number;
  hasLiked: boolean;
  createdAt: string;
  author: User;
};

/* ---- Media ---------------------------------------------------------------
   A post carries exactly one `mediaUrl`. The backend has no media-type field
   and no multi-attachment column, so the kind is inferred from the URL and
   each post holds one item. `mediaUrl` is required-but-empty for a text post. */

/** The kinds the composer and the feed both reason about. */
export type MediaKind = "image" | "video" | "gif";

/** What a post's media is, derived from its URL or the file that produced it. */
export type MediaKindSpec = {
  kind: MediaKind;
  /** The source the renderer should use. Empty means "no media". */
  url: string;
};

/**
 * `UploadSignature` — a short-lived Cloudinary signature for a direct upload.
 *
 * The API secret never reaches the browser: post-service signs `folder`,
 * `public_id` and `timestamp` only, and the client posts the file itself to
 * `uploadUrl`. Any field added to that request would invalidate the signature.
 */
export type UploadSignature = {
  cloudName: string;
  apiKey: string;
  /** Unix epoch seconds, carried as a string so the contract outlives 2038. */
  timestamp: string;
  signature: string;
  folder: string;
  publicId: string;
  uploadUrl: string;
  expiresAt: string;
};

/** What the composer holds before the file has been uploaded. */
export type MediaDraft = {
  /** A blob URL for previewing only; never sent to the backend. */
  previewUrl: string;
  kind: MediaKind;
  file: File;
};

export type FeedPage = {
  posts: FeedPost[];
  nextCursor: string | null;
  hasMore: boolean;
};

export type Comment = {
  id: string;
  postId: string;
  authorId: string;
  parentId: string | null;
  content: string;
  likeCount: number;
  replyCount: number;
  depth: number;
  isDeleted: boolean;
  hasLiked: boolean;
  createdAt: string;
  updatedAt: string;
  author: User;
};

export type CommentPage = {
  comments: Comment[];
  nextCursor: string | null;
  hasMore: boolean;
};

export type Message = {
  id: string;
  conversationId: string;
  senderId: string;
  content: string;
  createdAt: string;
  isDeleted: boolean;
  sender: User;
};

export type ConversationParticipant = {
  userId: string;
  lastReadAt: string | null;
  user: User;
};

export type Conversation = {
  id: string;
  type: string;
  title: string;
  createdBy: string;
  participants: ConversationParticipant[];
  lastMessage: Message | null;
  unreadCount: number;
  createdAt: string;
  updatedAt: string;
};

export type ConversationPage = {
  conversations: Conversation[];
  nextCursor: string | null;
  hasMore: boolean;
  totalUnread: number;
};

export type MessagePage = {
  messages: Message[];
  nextCursor: string | null;
  hasMore: boolean;
};

/** The schema's enum, exactly. There is no MESSAGE member. */
export type NotificationType =
  | "FOLLOW"
  | "POST_LIKE"
  | "COMMENT"
  | "COMMENT_REPLY"
  | "MENTION"
  | "CREATOR_POST"
  | "SYSTEM_ALERT";

export type NotificationActor = {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string;
};

export type Notification = {
  id: string;
  type: NotificationType;
  entityId: string;
  entityType: string;
  actors: NotificationActor[];
  totalActorsCount: number;
  title: string;
  contentPreview: string | null;
  actionUrl: string;
  isRead: boolean;
  createdAt: string;
};

export type NotificationPage = {
  notifications: Notification[];
  nextCursor: string | null;
  hasMore: boolean;
  unreadCount: number;
};

/** Which realtime channel an SSE frame arrived on. */
export type RealtimeStream = "notifications" | "chat";

/**
 * One Server-Sent Event from the gateway's `/stream` endpoint.
 *
 * The gateway relays each backend frame's `data` payload untouched, so the
 * field names here are the *backends'* snake_case wire names, not GraphQL's
 * camelCase. notification-service marshals `RealtimeData` and chat-service
 * marshals its own envelope; the gateway adds nothing but the `stream` and
 * `type` discriminators.
 */
export type RealtimeNotificationData = {
  id: string;
  type: NotificationType;
  title: string;
  content_preview?: string;
  action_url: string;
  actors: { id: string; username?: string; display_name?: string }[];
  total_actors_count: number;
  unread_count: number;
  created_at: string;
};

export type RealtimeChatData = {
  conversation_id: string;
  /** Present on message frames; the actor on a typing or read frame. */
  sender_id?: string;
  user_id?: string;
  is_typing?: boolean;
  /** Only on message.created. Unset on every other event. */
  content?: string;
  message_id?: string;
};

export type RealtimeFrame =
  | {
      stream: "notifications";
      /** Always "notification.received": the backend sends only this. */
      type: "notification.received";
      data: RealtimeNotificationData;
      receivedAt: number;
    }
  | {
      stream: "chat";
      type: "message.created" | "message.deleted" | "read" | "typing";
      data: RealtimeChatData;
      receivedAt: number;
    };

/** Tuple subscription listeners receive: the parsed frame and its stream. */
export type RealtimeListener = (frame: RealtimeFrame) => void;

/** The schema names these `followersCount` / `followingCount`. */
export type FollowCounts = {
  followersCount: number;
  followingCount: number;
};

export type FollowStatus = {
  isFollowing: boolean;
};

export type LikePostResponse = {
  isLiked: boolean;
  message: string;
};

/* ---- Typed failure codes -------------------------------------------------
   The proxy maps upstream failures onto these, so every screen has one
   vocabulary for "what went wrong". */
export type ErrorCode =
  | "GATEWAY_DOWN"
  | "UNAUTHENTICATED"
  | "RATE_LIMITED"
  | "NETWORK"
  | "TRANSPORT"
  | "EMPTY"
  | "INTERNAL";

export class ApiError extends Error {
  readonly code: ErrorCode;
  readonly retryAfter?: number;

  constructor(message: string, code: ErrorCode = "INTERNAL", retryAfter?: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.retryAfter = retryAfter;
  }
}

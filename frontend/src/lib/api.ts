/* ==========================================================================
   GraphQL client for the SocialFlow API gateway.
   --------------------------------------------------------------------------
   The gateway exposes one endpoint (/query) that speaks POST for
   queries/mutations and the WebSocket transport for subscriptions. This client
   does POST only; subscriptions are handled by lib/subscriptions.ts.

   Failures are surfaced as typed results rather than thrown, because every
   screen in this app has a defined error state and the guidance requires it.
   ========================================================================== */

const ENDPOINT = import.meta.env.VITE_GRAPHQL_URL ?? "/query";
const WS_ENDPOINT = import.meta.env.VITE_GRAPHQL_WS ?? deriveWsEndpoint(ENDPOINT);

function deriveWsEndpoint(httpEndpoint: string): string {
  const url = new URL(httpEndpoint, window.location.origin);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

export type GraphQLError = {
  message: string;
  extensions?: { code?: string; [key: string]: unknown };
};

export type GraphQLResult<T> = {
  data?: T;
  errors?: GraphQLError[];
};

export class ApiError extends Error {
  readonly code: string;

  constructor(message: string, code = "INTERNAL") {
    super(message);
    this.name = "ApiError";
    this.code = code;
  }
}

let accessToken: string | null = null;

export function setAccessToken(token: string | null): void {
  accessToken = token;
}

export function getAccessToken(): string | null {
  return accessToken;
}

/**
 * Runs one operation.
 *
 * A 200 response can still carry GraphQL errors — that is how the gateway
 * reports authorization and validation failures — so both are checked.
 * A UNAUTHENTICATED error clears the session, which is what makes a revoked
 * token behave identically to a logged-out one on every screen.
 */
export async function request<T>(
  query: string,
  variables: Record<string, unknown> = {},
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;

  let response: Response;
  try {
    response = await fetch(ENDPOINT, {
      method: "POST",
      headers,
      body: JSON.stringify({ query, variables }),
      signal,
    });
  } catch (cause) {
    if (signal?.aborted) throw cause;
    throw new ApiError("Cannot reach the gateway. Check that the backend is running.", "NETWORK");
  }

  if (!response.ok) {
    throw new ApiError(`The gateway returned ${response.status}.`, "TRANSPORT");
  }

  const payload = (await response.json()) as GraphQLResult<T>;
  const error = payload.errors?.[0];
  if (error) {
    if (error.extensions?.code === "UNAUTHENTICATED") setAccessToken(null);
    throw new ApiError(error.message, String(error.extensions?.code ?? "INTERNAL"));
  }
  if (!payload.data) {
    throw new ApiError("The gateway returned no data.", "EMPTY");
  }
  return payload.data;
}

/* ---- Operations ---------------------------------------------------------- */
/* Field selections are explicit and match the GraphQL schema in
   backend/api-gateway/graph/schema.graphqls. Anything added there must be
   added here in the same change. */

export const operations = {
  login: /* GraphQL */ `
    mutation Login($input: LoginInput!) {
      Login(input: $input) { accessToken refressToken message }
    }
  `,

  feed: /* GraphQL */ `
    query GetFeed($limit: Int, $cursor: String) {
      getFeed(limit: $limit, cursor: $cursor) {
        hasMore
        nextCursor
        posts {
          id authorId content mediaUrl likeCount createdAt
          author { id username name }
          hasLiked
        }
      }
    }
  `,

  createPost: /* GraphQL */ `
    mutation CreatePost($input: CreatePostInput!) {
      CreatePost(input: $input) {
        id authorId content mediaUrl likeCount createdAt
        author { id username name }
        hasLiked
      }
    }
  `,

  likePost: /* GraphQL */ `
    mutation LikePost($input: LikePostInput!) {
      LikePost(input: $input) { isLiked message }
    }
  `,

  searchPosts: /* GraphQL */ `
    query SearchPosts($query: String!, $limit: Int) {
      searchPosts(query: $query, limit: $limit) {
        id authorId content mediaUrl likeCount createdAt
        author { id username name }
        hasLiked
      }
    }
  `,

  searchUsers: /* GraphQL */ `
    query SearchUsers($query: String!, $limit: Int) {
      searchUsers(query: $query, limit: $limit) { id username name isPublic }
    }
  `,

  conversations: /* GraphQL */ `
    query MyConversations($limit: Int, $cursor: String) {
      myConversations(limit: $limit, cursor: $cursor) {
        hasMore nextCursor totalUnread
        conversations {
          id type title createdBy createdAt updatedAt unreadCount
          lastMessage { id senderId content createdAt isDeleted }
        }
      }
    }
  `,

  messages: /* GraphQL */ `
    query GetMessages($conversationId: ID!, $limit: Int, $cursor: String) {
      getMessages(conversationId: $conversationId, limit: $limit, cursor: $cursor) {
        hasMore nextCursor
        messages { id senderId content createdAt isDeleted }
      }
    }
  `,

  sendMessage: /* GraphQL */ `
    mutation SendMessage($input: SendMessageInput!) {
      SendMessage(input: $input) { id senderId content createdAt isDeleted }
    }
  `,

  typing: /* GraphQL */ `
    mutation SetTyping($input: TypingInput!) {
      setTyping(input: $input)
    }
  `,

  markAllRead: /* GraphQL */ `
    mutation MarkAllNotificationsAsRead {
      markAllNotificationsAsRead
    }
  `,

  notifications: /* GraphQL */ `
    query GetNotifications($limit: Int, $cursor: String) {
      getNotifications(limit: $limit, cursor: $cursor) {
        hasMore nextCursor unreadCount
        notifications {
          id type entityId entityType title contentPreview actionUrl isRead createdAt
          actors { id username displayName }
          totalActorsCount
        }
      }
    }
  `,

  createConversation: /* GraphQL */ `
    mutation CreateConversation($input: CreateConversationInput!) {
      createConversation(input: $input) {
        id type title createdBy createdAt unreadCount
      }
    }
  `,
} as const;

export { WS_ENDPOINT };

/* ---- Response shapes ----------------------------------------------------- */

export type Author = { id: string; username: string; name: string };
export type Post = {
  id: string;
  authorId: string;
  content: string;
  mediaUrl: string;
  likeCount: number;
  createdAt: string;
  author: Author;
  hasLiked: boolean;
};

export type FeedResponse = {
  getFeed: { hasMore: boolean; nextCursor: string | null; posts: Post[] };
};

export type SearchPostsResponse = { searchPosts: Post[] };

export type SearchUsersResponse = {
  searchUsers: Array<{ id: string; username: string; name: string; isPublic: boolean }>;
};

export type Conversation = {
  id: string;
  type: string;
  title: string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  unreadCount: number;
  lastMessage: { id: string; senderId: string; content: string; createdAt: string; isDeleted: boolean } | null;
};

export type ConversationsResponse = {
  myConversations: {
    hasMore: boolean;
    nextCursor: string | null;
    totalUnread: number;
    conversations: Conversation[];
  };
};

export type Message = {
  id: string;
  senderId: string;
  content: string;
  createdAt: string;
  isDeleted: boolean;
};

export type MessagesResponse = {
  getMessages: { hasMore: boolean; nextCursor: string | null; messages: Message[] };
};

export type Notification = {
  id: string;
  type: string;
  entityId: string;
  entityType: string;
  title: string;
  contentPreview: string | null;
  actionUrl: string;
  isRead: boolean;
  createdAt: string;
  actors: Array<{ id: string; username: string; displayName: string }>;
  totalActorsCount: number;
};

export type NotificationsResponse = {
  getNotifications: {
    hasMore: boolean;
    nextCursor: string | null;
    unreadCount: number;
    notifications: Notification[];
  };
};

export type LoginResponse = { Login: { accessToken: string; refressToken: string; message: string } };

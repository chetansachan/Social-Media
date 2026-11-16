/* ==========================================================================
   Demo data.
   --------------------------------------------------------------------------
   The app must be reviewable with no backend running. When the gateway is
   unreachable, screens render from this fixture and a banner says so.

   Every object here matches schema.graphqls exactly — a fixture that drifts
   from the schema is a fixture that hides bugs. The hard cases are included on
   purpose: a post with no media, a 4,000-character post that must clamp, an
   empty feed, an unread conversation, and a deleted-message tombstone.
   ========================================================================== */

import type {
  Comment,
  Conversation,
  FeedPost,
  FollowCounts,
  Message,
  Notification,
  Post,
  User,
} from "./types";

const EPOCH = Date.UTC(2026, 10, 9, 9, 0, 0);
const ago = (minutes: number) => new Date(EPOCH - minutes * 60_000).toISOString();
const joined = (year: number, month: number, day: number) =>
  new Date(Date.UTC(year, month, day)).toISOString();

function makeUser(
  id: string,
  username: string,
  name: string,
  created = joined(2024, 2, 14),
): User {
  return {
    id,
    username,
    name,
    email: `${username}@example.com`,
    isPublic: true,
    createdAt: created,
    updatedAt: created,
  };
}

export const demoUser: User = makeUser("u-self", "chetan", "Chetan Sachan");

export const demoAuthors: User[] = [
  makeUser("u-1", "systemdesign", "System Design Weekly"),
  makeUser("u-2", "gophers", "The Gopher Post"),
  makeUser("u-3", "a11y", "Accessibility Notes"),
  makeUser("u-4", "postgres", "Postgres Weekly"),
  makeUser("u-5", "distributed", "Distributed Systems"),
  makeUser("u-6", "typescript", "TypeScript Daily"),
  makeUser("u-7", "redis", "Redis Weekly"),
  makeUser("u-8", "kubernetes", "K8s Notes"),
  makeUser("u-9", "observability", "Tracing Daily"),
];

const longCaption =
  "A long caption exists to prove the clamp. It must not push the action row off-screen, must not resize the card, and must expose the full text on demand. " +
  "So here is the rest of it: the clamp is three lines, the expand control is a real button with aria-expanded, and there is no gradient mask pretending the text ended. " +
  "If you are reading this in the expanded state, the transform worked and the card grew by exactly the height of the hidden lines. " +
  "That last sentence exists only to make this genuinely long. #a11y #design";

function makePost(
  id: string,
  author: User,
  content: string,
  minutesAgo: number,
  likeCount: number,
  mediaUrl = "",
  hasLiked = false,
): Post {
  const createdAt = ago(minutesAgo);
  return {
    id,
    authorId: author.id,
    content,
    mediaUrl,
    isPublic: true,
    likeCount,
    hasLiked,
    createdAt,
    updatedAt: createdAt,
    author,
  };
}

export const demoPosts: Post[] = [
  makePost(
    "p-1",
    demoAuthors[0]!,
    "Backpressure is a feature, not a failure. If your queue is always empty, you have a spare machine. #systemdesign",
    14,
    1284,
  ),
  makePost(
    "p-2",
    demoAuthors[4]!,
    "The outbox pattern in one sentence: commit the event in the same transaction as the row. Everything else is retry policy. #distributed",
    52,
    4210,
    "https://images.unsplash.com/photo-1451187580459-43490279c0fa?w=1200&q=80",
    true,
  ),
  makePost(
    "p-3",
    demoAuthors[1]!,
    "Go tip: name your error variables err and stop shadowing them in closures. The bug you save is your own. #golang #errors",
    96,
    342,
  ),
  makePost(
    "p-4",
    demoAuthors[2]!,
    longCaption,
    180,
    89,
    "https://images.unsplash.com/photo-1517849845537-4d257902454a?w=1200&q=80",
  ),
  makePost(
    "p-5",
    demoAuthors[3]!,
    "pg_trgm and tsvector live in the same database as your rows, which means a search hit is never staler than the row. #postgres #search",
    240,
    214,
  ),
  makePost(
    "p-6",
    demoAuthors[5]!,
    "TypeScript 5.9's inferred type predicates removed about forty lines of hand-written guards from our codebase. Worth the upgrade on its own. #typescript",
    320,
    1876,
    "https://images.unsplash.com/photo-1555066931-4365d14bab8c?w=1200&q=80",
  ),
  makePost(
    "p-7",
    demoAuthors[0]!,
    "A rate limiter that returns 429 without Retry-After is a bug report waiting to happen. Tell the client when to come back. #systemdesign",
    410,
    967,
  ),
  makePost(
    "p-8",
    demoAuthors[4]!,
    "Fanout on write is a bet that your followers are fewer than your reads. Fanout on read is a bet that your reads are fewer than your followers. Pick the one you can afford to be wrong about. #distributed",
    520,
    3102,
  ),
  makePost(
    "p-9",
    demoAuthors[6]!,
    "A sorted set is a leaderboard, a timeline, and a rate limiter if you squint. #redis",
    640,
    742,
  ),
  makePost(
    "p-10",
    demoAuthors[8]!,
    "A trace without a span attribute is a very expensive timestamp. #observability",
    720,
    511,
  ),
];

/** `FeedPost` has no `updatedAt`, so the feed fixture is derived, not reused. */
export function toFeedPost(post: Post): FeedPost {
  const { updatedAt: _updatedAt, ...rest } = post;
  return rest;
}

export const demoFeed: FeedPost[] = demoPosts.map(toFeedPost);

/** The follow-only feed: a subset, because the backend only sends followed authors. */
export const demoFollowingFeed: FeedPost[] = demoPosts
  .filter((post) => ["u-1", "u-2", "u-6"].includes(post.authorId))
  .map(toFeedPost);

export const demoComments: Comment[] = [
  {
    id: "cm-1",
    postId: "p-1",
    authorId: "u-2",
    parentId: null,
    content: "The spare-machine line is going on a slide.",
    likeCount: 42,
    replyCount: 0,
    depth: 0,
    isDeleted: false,
    hasLiked: false,
    createdAt: ago(10),
    updatedAt: ago(10),
    author: demoAuthors[1]!,
  },
  {
    id: "cm-2",
    postId: "p-1",
    authorId: "u-3",
    parentId: null,
    content: "Only if the queue depth is actually exported as a metric.",
    likeCount: 18,
    replyCount: 0,
    depth: 0,
    isDeleted: false,
    hasLiked: true,
    createdAt: ago(8),
    updatedAt: ago(8),
    author: demoAuthors[2]!,
  },
];

export const demoConversations: Conversation[] = [
  {
    id: "c-1",
    type: "DIRECT",
    title: "",
    createdBy: "u-self",
    unreadCount: 2,
    createdAt: ago(5760),
    updatedAt: ago(24),
    participants: [
      { userId: "u-self", lastReadAt: ago(30), user: demoUser },
      { userId: "u-1", lastReadAt: ago(24), user: demoAuthors[0]! },
    ],
    lastMessage: {
      id: "m-2",
      conversationId: "c-1",
      senderId: "u-1",
      content: "Did you see the outbox commit?",
      createdAt: ago(24),
      isDeleted: false,
      sender: demoAuthors[0]!,
    },
  },
  {
    id: "c-2",
    type: "GROUP",
    title: "Platform team",
    createdBy: "u-self",
    unreadCount: 0,
    createdAt: ago(12000),
    updatedAt: ago(120),
    participants: [
      { userId: "u-self", lastReadAt: ago(100), user: demoUser },
      { userId: "u-2", lastReadAt: ago(120), user: demoAuthors[1]! },
      { userId: "u-3", lastReadAt: ago(200), user: demoAuthors[2]! },
    ],
    lastMessage: {
      id: "m-4",
      conversationId: "c-2",
      senderId: "u-2",
      content: "",
      createdAt: ago(120),
      isDeleted: true,
      sender: demoAuthors[1]!,
    },
  },
  {
    id: "c-3",
    type: "DIRECT",
    title: "",
    createdBy: "u-3",
    unreadCount: 0,
    createdAt: ago(9000),
    updatedAt: ago(400),
    participants: [
      { userId: "u-self", lastReadAt: ago(400), user: demoUser },
      { userId: "u-3", lastReadAt: ago(400), user: demoAuthors[2]! },
    ],
    lastMessage: {
      id: "m-5",
      conversationId: "c-3",
      senderId: "u-3",
      content: "Pushed the focus-ring fix, take a look when you can.",
      createdAt: ago(400),
      isDeleted: false,
      sender: demoAuthors[2]!,
    },
  },
];

export const demoMessages: Record<string, Message[]> = {
  "c-1": [
    {
      id: "m-1",
      conversationId: "c-1",
      senderId: "u-self",
      content: "Shipping the relay today.",
      createdAt: ago(360),
      isDeleted: false,
      sender: demoUser,
    },
    {
      id: "m-2",
      conversationId: "c-1",
      senderId: "u-1",
      content: "Did you see the outbox commit?",
      createdAt: ago(24),
      isDeleted: false,
      sender: demoAuthors[0]!,
    },
  ],
  "c-2": [
    {
      id: "m-3",
      conversationId: "c-2",
      senderId: "u-self",
      content: "Fanout thresholds are in the config now.",
      createdAt: ago(200),
      isDeleted: false,
      sender: demoUser,
    },
    {
      id: "m-4",
      conversationId: "c-2",
      senderId: "u-2",
      content: "",
      createdAt: ago(120),
      isDeleted: true,
      sender: demoAuthors[1]!,
    },
  ],
  "c-3": [
    {
      id: "m-5",
      conversationId: "c-3",
      senderId: "u-3",
      content: "Pushed the focus-ring fix, take a look when you can.",
      createdAt: ago(400),
      isDeleted: false,
      sender: demoAuthors[2]!,
    },
  ],
};

export const demoNotifications: Notification[] = [
  {
    id: "n-1",
    type: "POST_LIKE",
    entityId: "p-2",
    entityType: "post",
    title: "New like on your post",
    contentPreview: "Go tip: name your error variables err and stop shadowing them…",
    actionUrl: "/post/p-2",
    isRead: false,
    createdAt: ago(12),
    totalActorsCount: 1,
    actors: [
      { id: "u-1", username: "systemdesign", displayName: "System Design Weekly", avatarUrl: "" },
    ],
  },
  {
    id: "n-2",
    type: "COMMENT",
    entityId: "p-1",
    entityType: "post",
    title: "New comment on your post",
    contentPreview: "Backpressure is a feature…",
    actionUrl: "/post/p-1",
    isRead: false,
    createdAt: ago(48),
    totalActorsCount: 2,
    actors: [
      { id: "u-3", username: "a11y", displayName: "Accessibility Notes", avatarUrl: "" },
      { id: "u-4", username: "postgres", displayName: "Postgres Weekly", avatarUrl: "" },
    ],
  },
  {
    id: "n-3",
    type: "FOLLOW",
    entityId: "u-2",
    entityType: "user",
    title: "New follower",
    contentPreview: null,
    actionUrl: "/profile/gophers",
    isRead: true,
    createdAt: ago(1800),
    totalActorsCount: 1,
    actors: [{ id: "u-2", username: "gophers", displayName: "The Gopher Post", avatarUrl: "" }],
  },
  {
    id: "n-4",
    type: "MENTION",
    entityId: "p-8",
    entityType: "post",
    title: "You were mentioned",
    contentPreview: "@chetan the fanout numbers here are yours, right?",
    actionUrl: "/post/p-8",
    isRead: true,
    createdAt: ago(2600),
    totalActorsCount: 1,
    actors: [
      { id: "u-5", username: "distributed", displayName: "Distributed Systems", avatarUrl: "" },
    ],
  },
];

export const demoFollowCounts: FollowCounts = {
  followersCount: 48200,
  followingCount: 312,
};

/**
 * Trending tags.
 *
 * These are presentation-only: the backend has no trending endpoint, and
 * `searchPosts` already handles `#tag` through its hashtag array overlap, so a
 * tag click is just a search. Counts are illustrative, not measured.
 */
export type Trend = {
  tag: string;
  posts: number;
  category: string;
};

export const demoTrends: Trend[] = [
  { tag: "systemdesign", posts: 12400, category: "Engineering" },
  { tag: "golang", posts: 8210, category: "Languages" },
  { tag: "postgres", posts: 5600, category: "Databases" },
  { tag: "a11y", posts: 3180, category: "Design" },
  { tag: "typescript", posts: 2940, category: "Languages" },
  { tag: "distributed", posts: 1870, category: "Engineering" },
];

/** Suggested accounts. Also presentation-only; no recommendation endpoint. */
export const demoWhoToFollow: User[] = [demoAuthors[6]!, demoAuthors[7]!, demoAuthors[8]!];

/** An authenticated-but-empty account, used for the empty-state story. */
export const demoEmpty = {
  posts: [] as Post[],
  feed: [] as FeedPost[],
  conversations: [] as Conversation[],
  notifications: [] as Notification[],
};

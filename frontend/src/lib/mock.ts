/* ==========================================================================
   Demo data.
   --------------------------------------------------------------------------
   The frontend has to be reviewable on its own: a designer or a QA engineer
   must be able to open the app and see every component state without six
   databases running. When the gateway is unreachable or VITE_API_MODE=mock,
   the app renders from this fixture instead.

   The fixture deliberately includes the hard cases the component rules call
   out: a post with no media, a post with a very long caption that must clamp,
   an empty feed, an unread conversation, and a deleted message tombstone.
   ========================================================================== */

import type { Post, Conversation, Message, Notification } from "./api";

const EPOCH = Date.UTC(2026, 0, 15, 9, 0, 0);

export const demoUser = {
  id: "u-self",
  username: "you",
  name: "You",
  email: "you@example.com",
};

export const demoAuthors = [
  { id: "u-1", username: "systemdesign", name: "System Design Weekly" },
  { id: "u-2", username: "gophers", name: "The Gopher Post" },
  { id: "u-3", username: "a11y", name: "Accessibility Notes" },
  { id: "u-4", username: "postgres", name: "Postgres Weekly" },
];

const longCaption =
  "A long caption exists to prove the clamp. ".repeat(24) +
  "It must not push the action row off-screen, must not resize the card, and must expose the full text on demand.";

export const demoPosts: Post[] = [
  {
    id: "p-1",
    authorId: "u-1",
    content:
      "Backpressure is a feature, not a failure. If your queue is always empty, you have a spare machine. #systemdesign",
    mediaUrl: "",
    likeCount: 1284,
    createdAt: new Date(EPOCH - 60 * 60 * 1000).toISOString(),
    author: demoAuthors[0],
    hasLiked: false,
  },
  {
    id: "p-2",
    authorId: "u-2",
    content:
      "Go tip: name your error variables err and stop shadowing them in closures. #golang #errors",
    mediaUrl: "",
    likeCount: 342,
    createdAt: new Date(EPOCH - 5 * 60 * 60 * 1000).toISOString(),
    author: demoAuthors[1],
    hasLiked: true,
  },
  {
    id: "p-3",
    authorId: "u-3",
    content: longCaption,
    mediaUrl: "https://images.unsplash.com/photo-1517849845537-4d257902454a?w=800",
    likeCount: 89,
    createdAt: new Date(EPOCH - 26 * 60 * 60 * 1000).toISOString(),
    author: demoAuthors[2],
    hasLiked: false,
  },
  {
    id: "p-4",
    authorId: "u-4",
    content:
      "pg_trgm and tsvector live in the same database as your rows, which means a search hit is never staler than the row. #postgres #search",
    mediaUrl: "",
    likeCount: 214,
    createdAt: new Date(EPOCH - 30 * 60 * 60 * 1000).toISOString(),
    author: demoAuthors[3],
    hasLiked: false,
  },
  {
    id: "p-5",
    authorId: "u-1",
    content: "The outbox pattern in one sentence: commit the event in the same transaction as the row. #systemdesign",
    mediaUrl: "",
    likeCount: 967,
    createdAt: new Date(EPOCH - 50 * 60 * 60 * 1000).toISOString(),
    author: demoAuthors[0],
    hasLiked: false,
  },
];

export const demoConversations: Conversation[] = [
  {
    id: "c-1",
    type: "DIRECT",
    title: "",
    createdBy: "u-self",
    createdAt: new Date(EPOCH - 96 * 60 * 60 * 1000).toISOString(),
    updatedAt: new Date(EPOCH - 4 * 60 * 60 * 1000).toISOString(),
    unreadCount: 2,
    lastMessage: {
      id: "m-2",
      senderId: "u-1",
      content: "Did you see the outbox commit?",
      createdAt: new Date(EPOCH - 4 * 60 * 60 * 1000).toISOString(),
      isDeleted: false,
    },
  },
  {
    id: "c-2",
    type: "GROUP",
    title: "Platform team",
    createdBy: "u-self",
    createdAt: new Date(EPOCH - 200 * 60 * 60 * 1000).toISOString(),
    updatedAt: new Date(EPOCH - 20 * 60 * 60 * 1000).toISOString(),
    unreadCount: 0,
    lastMessage: {
      id: "m-4",
      senderId: "u-2",
      content: "",
      createdAt: new Date(EPOCH - 20 * 60 * 60 * 1000).toISOString(),
      isDeleted: true,
    },
  },
];

export const demoMessages: Message[] = [
  {
    id: "m-1",
    senderId: "u-self",
    content: "Shipping the relay today.",
    createdAt: new Date(EPOCH - 6 * 60 * 60 * 1000).toISOString(),
    isDeleted: false,
  },
  {
    id: "m-2",
    senderId: "u-1",
    content: "Did you see the outbox commit?",
    createdAt: new Date(EPOCH - 4 * 60 * 60 * 1000).toISOString(),
    isDeleted: false,
  },
];

export const demoNotifications: Notification[] = [
  {
    id: "n-1",
    type: "POST_LIKE",
    entityId: "p-2",
    entityType: "post",
    title: "New like on your post",
    contentPreview: "Go tip: name your error variables err and stop shadowing them…",
    actionUrl: "socialflow://posts/p-2",
    isRead: false,
    createdAt: new Date(EPOCH - 2 * 60 * 60 * 1000).toISOString(),
    actors: [{ id: "u-1", username: "systemdesign", displayName: "System Design Weekly" }],
    totalActorsCount: 1,
  },
  {
    id: "n-2",
    type: "COMMENT",
    entityId: "p-1",
    entityType: "post",
    title: "New comment on your post",
    contentPreview: "Backpressure is a feature…",
    actionUrl: "socialflow://posts/p-1",
    isRead: false,
    createdAt: new Date(EPOCH - 3 * 60 * 60 * 1000).toISOString(),
    actors: [
      { id: "u-3", username: "a11y", displayName: "Accessibility Notes" },
      { id: "u-4", username: "postgres", displayName: "Postgres Weekly" },
    ],
    totalActorsCount: 2,
  },
  {
    id: "n-3",
    type: "FOLLOW",
    entityId: "u-2",
    entityType: "user",
    title: "New follower",
    contentPreview: null,
    actionUrl: "socialflow://users/u-2",
    isRead: true,
    createdAt: new Date(EPOCH - 30 * 60 * 60 * 1000).toISOString(),
    actors: [{ id: "u-2", username: "gophers", displayName: "The Gopher Post" }],
    totalActorsCount: 1,
  },
];

/** An authenticated-but-empty account, used for the empty-state story. */
export const demoEmpty = { posts: [], conversations: [], notifications: [] };

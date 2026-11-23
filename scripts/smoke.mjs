/**
 * Smoke-tests the live gateway on the paths the web client depends on.
 * Run: node scripts/smoke.mjs
 */

const GATEWAY = process.env.GATEWAY_URL ?? "http://localhost:8080/query";

async function gql(query, variables = {}, token) {
  const response = await fetch(GATEWAY, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ query, variables }),
  });
  const payload = await response.json();
  if (!response.ok || payload.errors?.length) {
    throw new Error(
      `${response.status}: ${payload.errors?.[0]?.message ?? "request failed"}`,
    );
  }
  return payload.data;
}

const results = [];
function check(name, ok, detail = "") {
  results.push({ name, ok, detail });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${detail ? ` — ${detail}` : ""}`);
}

const login = await gql(
  `mutation { Login(input: {email: "chetan@example.com", password: "password123"}) {
    accessToken
  } }`,
);
const token = login.Login.accessToken;
check("login", Boolean(token));

const profile = await gql(
  `query { getUserByUsername(username: "chetan") { id username email } }`,
);
const me = await gql(
  `query($id: ID!) { getUserById(id: $id) { id username email } }`,
  { id: profile.getUserByUsername.id },
  token,
);
check("session profile", me.getUserById.username === "chetan", me.getUserById.username);

const feed = await gql(
  `query { getFeed(limit: 12) { hasMore nextCursor posts { id content likeCount hasLiked createdAt author { id username name } } } }`,
  {},
  token,
);
check("feed", feed.getFeed.posts.length > 0, `${feed.getFeed.posts.length} posts`);
check(
  "feed authors hydrate",
  feed.getFeed.posts.every((post) => post.author?.username),
  [...new Set(feed.getFeed.posts.map((post) => post.author.username))].join(", "),
);

const tag = await gql(`query { searchPosts(query: "#golang", limit: 10) { id content } }`);
check("tag search", tag.searchPosts.length > 0, `${tag.searchPosts.length} hits`);

const text = await gql(`query { searchPosts(query: "postgres", limit: 10) { id content } }`);
check("text search", text.searchPosts.length > 0, `${text.searchPosts.length} hits`);

const people = await gql(`query { searchUsers(query: "pri", limit: 5) { username name } }`);
check("people search", people.searchUsers.some((user) => user.username === "priya"));

const notifications = await gql(
  `query { getNotifications(limit: 20) { unreadCount notifications { id type title actionUrl isRead actors { username } } } }`,
  {},
  token,
);
check(
  "notifications",
  notifications.getNotifications.notifications.length > 0,
  `${notifications.getNotifications.notifications.length} rows, ${notifications.getNotifications.unreadCount} unread`,
);

const conversations = await gql(
  `query { myConversations(limit: 10) { totalUnread conversations { id title unreadCount lastMessage { content } participants { user { username } } } } }`,
  {},
  token,
);
check(
  "conversations",
  conversations.myConversations.conversations.length > 0,
  `${conversations.myConversations.conversations.length} threads`,
);

const postId = feed.getFeed.posts[0].id;
const comments = await gql(
  `query { getComments(postId: "${postId}", limit: 20) { comments { id content author { username } } } }`,
);
check("comments", comments.getComments.comments.length >= 0, `${comments.getComments.comments.length} replies`);

const counts = await gql(
  `query($id: String!) { getFollowCounts(userId: $id) { followersCount followingCount } }`,
  { id: profile.getUserByUsername.id },
);
check(
  "follow counts",
  counts.getFollowCounts.followersCount > 0 && counts.getFollowCounts.followingCount > 0,
  `${counts.getFollowCounts.followersCount} followers / ${counts.getFollowCounts.followingCount} following`,
);

const failed = results.filter((result) => !result.ok);
console.log(`\n${results.length - failed.length}/${results.length} checks passed`);
if (failed.length) process.exitCode = 1;

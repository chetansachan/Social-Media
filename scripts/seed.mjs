/**
 * Seeds the local SocialFlow stack with a small, coherent data set.
 *
 * Run with: node scripts/seed.mjs
 *
 * Idempotent by account (existing users are signed in, not re-created) and by
 * post (an author with posts is skipped), so re-running does not duplicate the
 * feed. Requires the gateway on :8080 and all gRPC services up.
 */

const GATEWAY = process.env.GATEWAY_URL ?? "http://localhost:8080/query";
const PASSWORD = "password123";

const USERS = [
  { username: "chetan", name: "Chetan Sachan", email: "chetan@example.com" },
  { username: "priya", name: "Priya Nair", email: "priya@example.com" },
  { username: "arjun", name: "Arjun Rao", email: "arjun@example.com" },
  { username: "maya", name: "Maya Iyer", email: "maya@example.com" },
  { username: "dev", name: "Dev Patel", email: "dev@example.com" },
  { username: "sara", name: "Sara Khan", email: "sara@example.com" },
];

const IMAGES = [
  "https://res.cloudinary.com/demo/image/upload/f_auto,q_auto,w_1200/cld-sample.jpg",
  "https://res.cloudinary.com/demo/image/upload/f_auto,q_auto,w_1200/cld-sample-2.jpg",
  "https://res.cloudinary.com/demo/image/upload/f_auto,q_auto,w_1200/cld-sample-3.jpg",
  "https://res.cloudinary.com/demo/image/upload/f_auto,q_auto,w_1200/cld-sample-4.jpg",
  "https://res.cloudinary.com/demo/image/upload/f_auto,q_auto,w_1200/cld-sample-5.jpg",
  "https://res.cloudinary.com/demo/image/upload/f_auto,q_auto,w_1200/sample.jpg",
];

const POSTS = [
  {
    author: "priya",
    content:
      "The Redis sorted set is still the most underrated data structure in my toolkit. One index gives you a chronological timeline, a score-based rank, and cursor pagination without a single offset scan. #redis #systemdesign",
  },
  {
    author: "arjun",
    content:
      "Spent the morning reading query plans instead of guessing. Postgres was choosing a sequential scan because the planner underestimated selectivity — a stale statistic, not a missing index. ANALYZE first, index second. #postgres #performance",
  },
  {
    author: "maya",
    content:
      "Shipped cursor pagination on the feed today. The cursor is an opaque base64 pair of (score, id), so a post published mid-scroll can never shift a page boundary. Users see a stable list; the API stays stateless. #golang #api",
    media: 0,
  },
  {
    author: "dev",
    content:
      "Fanout on write versus fanout on read is not a binary. Hybrid works: push to timelines for normal accounts, pull at read time for celebrities past a threshold. The hard part is the threshold, not the serving path. #distributed-systems #systemdesign",
  },
  {
    author: "sara",
    content:
      "Accessibility note I keep repeating: an icon-only button needs an accessible name, and a state needs more than colour. aria-pressed and a changing label cost nothing and carry the whole interaction. #frontend #a11y",
  },
  {
    author: "priya",
    content:
      "gRPC deadlines are not optional. A missing deadline turns one slow dependency into a pile of pinned workers, and the outage looks like a traffic spike. Propagate the context, cancel the work, fail fast. #golang #grpc",
    media: 1,
  },
  {
    author: "arjun",
    content:
      "The outbox pattern finally clicked: write the domain row and the event in one transaction, then let a relay publish. No dual-write window, no lost events, and retries are just a queue drain. #postgres #architecture",
  },
  {
    author: "maya",
    content:
      "A weekly reminder that the fastest query is the one you do not run. We cut p99 on the profile page by 60% with a singleflight group in front of Redis. Ten thousand concurrent misses, one database read. #golang #performance",
    media: 2,
  },
  {
    author: "dev",
    content:
      "Full-text search inside Postgres is good enough far longer than most teams admit. tsvector for relevance, trigram for fuzzy recall, and a GIN index. Adding a separate index means a second source of truth to keep fresh. #postgres #search",
  },
  {
    author: "sara",
    content:
      "Design systems are mostly a contract about restraint: one type scale, one spacing scale, one focus treatment. Every exception you allow becomes something the next person has to remember. #frontend #design",
    media: 3,
  },
  {
    author: "chetan",
    content:
      "Rebuilt the feed UI around one honest assumption: the home feed is follow-only. No For-you tab, no algorithmic fiction. Just the people you chose, newest first, with a cursor that never skips. #systemdesign #frontend",
    media: 4,
  },
  {
    author: "chetan",
    content:
      "Reading the Go scheduler source again. The run queue is per-P and the global queue is a fallback, which is why goroutines are cheap until they are not: blocking syscalls steal an M, and spinning steals a P. #golang",
  },
];

const COMMENTS = [
  { post: 0, author: "chetan", content: "The stable-cursor detail is the part everyone gets wrong." },
  { post: 0, author: "arjun", content: "Score plus id is such a clean tiebreaker." },
  { post: 2, author: "maya", content: "Opaque cursors also let you change the encoding later. Worth it." },
  { post: 3, author: "priya", content: "The threshold is the whole design. Everything else is plumbing." },
  { post: 8, author: "dev", content: "Trigram recall saved us from a whole Elasticsearch cluster." },
];

const LIKES = [
  { post: 0, by: ["chetan", "maya", "dev"] },
  { post: 1, by: ["chetan", "priya"] },
  { post: 2, by: ["chetan", "arjun", "sara"] },
  { post: 4, by: ["chetan", "priya", "maya", "dev"] },
  { post: 10, by: ["priya", "arjun", "maya", "dev", "sara"] },
];

/** Who follows whom. Everyone follows Chetan so his feed has content. */
const FOLLOWS = [
  ["chetan", "priya"],
  ["chetan", "arjun"],
  ["chetan", "maya"],
  ["chetan", "dev"],
  ["chetan", "sara"],
  ["priya", "chetan"],
  ["arjun", "chetan"],
  ["maya", "chetan"],
  ["dev", "chetan"],
  ["sara", "chetan"],
  ["priya", "arjun"],
  ["maya", "priya"],
];

async function gql(query, variables = {}, token) {
  const response = await fetch(GATEWAY, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ query, variables }),
  });
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    throw new Error(`HTTP ${response.status}: ${payload?.message ?? "request failed"}`);
  }
  if (payload?.errors?.length) {
    throw new Error(payload.errors[0].message);
  }
  return payload.data;
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function ensureUser(user) {
  try {
    await gql(
      `mutation Register($input: RegisterUserInput!) {
        register(input: $input) { id username name }
      }`,
      { input: { ...user, password: PASSWORD, isPublic: true } },
    );
    console.log(`  + registered @${user.username}`);
  } catch (error) {
    if (!/exist|duplicate|unique|taken/i.test(String(error.message))) {
      throw error;
    }
    console.log(`  = @${user.username} already exists`);
  }

  const data = await gql(
    `mutation Login($input: LoginInput!) {
      Login(input: $input) { accessToken refressToken }
    }`,
    { input: { email: user.email, password: PASSWORD } },
  );
  const accessToken = data.Login.accessToken;

  const profile = await gql(
    `query GetUserByUsername($username: String!) {
      getUserByUsername(username: $username) { id username name }
    }`,
    { username: user.username },
  );

  return { ...profile.getUserByUsername, accessToken };
}

async function main() {
  console.log("Seeding SocialFlow…\n1. Accounts");
  const accounts = {};
  for (const user of USERS) {
    accounts[user.username] = await ensureUser(user);
  }

  console.log("\n2. Follows");
  for (const [follower, following] of FOLLOWS) {
    const a = accounts[follower];
    const b = accounts[following];
    try {
      await gql(
        `mutation Follow($input: FollowUserInput!) { followUser(input: $input) { message } }`,
        { input: { followerId: a.id, followingId: b.id } },
        a.accessToken,
      );
    } catch (error) {
      console.log(`  ! ${follower} → ${following}: ${error.message}`);
    }
  }
  console.log(`  + ${FOLLOWS.length} follow edges`);

  console.log("\n3. Posts");
  const created = [];
  let newCount = 0;
  for (const [index, post] of POSTS.entries()) {
    const author = accounts[post.author];
    const existing = await gql(
      `query GetAllPost($authorId: String!) { getAllPost(author_id: $authorId) { id content } }`,
      { authorId: author.id },
    );
    const duplicate = existing.getAllPost.find(
      (candidate) => candidate.content === post.content,
    );
    if (duplicate) {
      created.push({ id: duplicate.id, seedIndex: index });
      console.log(`  = @${post.author} post already seeded`);
      continue;
    }

    const data = await gql(
      `mutation CreatePost($input: CreatePostInput!) {
        CreatePost(input: $input) {
          id authorId content mediaUrl createdAt author { username name }
        }
      }`,
      {
        input: {
          authorId: author.id,
          content: post.content,
          mediaUrl: post.media ? IMAGES[post.media] : "",
          isPublic: true,
        },
      },
      author.accessToken,
    );
    created.push({ ...data.CreatePost, seedIndex: index });
    newCount += 1;
    console.log(`  + @${post.author}: ${post.content.slice(0, 48)}…`);
    await sleep(120);
  }

  if (newCount === 0) {
    console.log("\nAll posts already present; skipping comments, likes, and chat.");
    return;
  }

  console.log("\n4. Comments");
  for (const comment of COMMENTS) {
    const root = created.find((post) => post.seedIndex === comment.post);
    if (!root) continue;
    const author = accounts[comment.author];
    await gql(
      `mutation CreateComment($input: CreateCommentInput!) {
        createComment(input: $input) { id content }
      }`,
      { input: { postId: root.id, content: comment.content } },
      author.accessToken,
    );
    console.log(`  + @${comment.author}: ${comment.content.slice(0, 44)}…`);
  }

  console.log("\n5. Likes");
  for (const like of LIKES) {
    const post = created.find((item) => item.seedIndex === like.post);
    if (!post) continue;
    for (const username of like.by) {
      const account = accounts[username];
      await gql(
        `mutation Like($input: LikePostInput!) { likePost(input: $input) { isLiked } }`,
        { input: { postId: post.id, userId: account.id } },
        account.accessToken,
      );
    }
    console.log(`  + post ${like.post}: ${like.by.length} likes`);
  }

  console.log("\n6. Conversation");
  const chetan = accounts.chetan;
  const priya = accounts.priya;
  const conversation = await gql(
    `mutation CreateConversation($input: CreateConversationInput!) {
      createConversation(input: $input) { id title participants { userId } }
    }`,
    { input: { participantIds: [priya.id] } },
    chetan.accessToken,
  );
  const conversationId = conversation.createConversation.id;

  const messages = [
    { from: priya, text: "That cursor-pagination post was exactly what I needed." },
    { from: chetan, text: "Glad it helped. The opaque encoding is the trick." },
    { from: priya, text: "Did you keep score and id in the same token?" },
    { from: chetan, text: "Yeah — score first, id as the tiebreaker. No offset anywhere." },
  ];
  for (const message of messages) {
    await gql(
      `mutation Send($input: SendMessageInput!) { sendMessage(input: $input) { id } }`,
      { input: { conversationId, content: message.text } },
      message.from.accessToken,
    );
  }
  console.log(`  + conversation with ${messages.length} messages`);

  console.log("\nDone. Sign in as chetan@example.com / password123");
}

main().catch((error) => {
  console.error("\nSeed failed:", error.message);
  process.exitCode = 1;
});

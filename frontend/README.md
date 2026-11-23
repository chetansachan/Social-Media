# SocialFlow Frontend

Next.js 15 App Router frontend for the SocialFlow backend: a chronological,
follow-only newsfeed for engineers, wired to the real GraphQL gateway.

## Run it

```powershell
npm install
npm run dev          # http://localhost:3000
```

The app expects the gateway on `http://localhost:8080/query` (override with
`GATEWAY_URL`). With the gateway down, the feed falls back to a fixture and
shows a banner; every other screen reports the failure it actually got.

### Demo data

With Postgres, Redis and the services running:

```powershell
node scripts/seed.mjs
```

That creates six accounts, follow edges, twelve posts (some with images), five
comments, likes, and a direct conversation. Idempotent by account and by post.

Sign in at `/login` with **chetan@example.com / password123** (the form has a
one-click "Use demo account" button).

## Stack

| Piece | Role |
| :--- | :--- |
| Next.js 15 (App Router) | Route-level splitting; the token never leaves the server |
| React 19 | Required by Next 15 |
| TypeScript 5, `strict` + `noUncheckedIndexedAccess` | Domain types transcribed from the schema |
| Tailwind v4 (`@theme`) | Utilities resolve to tokens in `src/styles/tokens.css` |
| Radix primitives (shadcn pattern) | Dialog, tabs, dropdown, switch, avatar, tooltip — restyled onto the tokens |
| TanStack Query v5 | Every screen: caching, infinite feed, optimistic like/follow |
| TanStack Virtual v3 | Windowed feed — variable-height cards, measured |
| GSAP 3.13 | Like burst and entrance motion, lazily imported through `lib/motion.ts` |
| `next/font` | Self-hosted Instrument Serif / Geist / Geist Mono |
| Zod | Auth request bodies |
| Sonner | Toasts for actions that complete outside the page |

## Fonts

| Token | Family | Used for |
| :--- | :--- | :--- |
| `--font-display` | **Instrument Serif** (400) | wordmark, hero, page titles, empty states |
| `--font-sans` | **Geist** | post copy, comments, interface chrome, buttons |
| `--font-mono` | **Geist Mono** | handles, counts, timestamps, tags |

All three families are self-hosted by `next/font` with `adjustFontFallback`, so
there is no runtime Google request and no layout shift. Instrument Serif ships a
single 400 cut, so display headings use its natural weight instead of a
synthesised bold.

## Routes

| Route | Data |
| :--- | :--- |
| `/` | `getFeed` infinite — follow-only, one feed, no For-you/Following tabs |
| `/explore` | `searchPosts` + `searchUsers`, 250 ms debounce, query in `?q=` |
| `/post/[id]` | `getPostById`, threaded `getComments`, `createComment`, `likeComment`, delete-own |
| `/notifications` | `getNotifications` with server-side type filters, mark-one / mark-all read |
| `/messages` | `myConversations` split pane + `searchUsers`/`createConversation` picker |
| `/messages/[id]` | `getMessages`, `sendMessage`, `markConversationRead`, `getPresence` |
| `/profile/[username]` | `getUserByUsername`, `getAllPost`, `getAllLikedPosts`, counts, follow, `UpdateUser`, `createConversation` |
| `/login` | Sign in / create account (`Login`, `register`) |
| `/states` | Component QA surface, no data |
| `/api/graphql` | Proxy: httpOnly cookie → `Authorization`, refresh-on-401 |
| `/api/auth/session` | Decodes the cookie, resolves the profile, rotates tokens |
| `/api/auth/login`, `/register`, `/logout` | Cookie lifecycle |

## Auth

The access token (15 min) and refresh token (7 days) live in httpOnly,
`SameSite=Lax` cookies. Nothing in the browser can read them.

- `/api/auth/session` decodes the `user_id` claim server-side, resolves the
  profile through `getUserById`, and transparently rotates an expired access
  token with the refresh cookie.
- `/api/graphql` attaches the bearer server-side. On a 401 it rotates the pair
  and replays the request once, so a long-lived tab does not silently sign out.
- `SessionProvider` exposes the viewer to every screen; composer authorship,
  like attribution, the profile link, and the account menu all follow it.
- `logout` also blacklists the current `jti` at the gateway.

## Responsive layout

| Width | Nav | Feed | Right rail |
| :--- | :--- | :--- | :--- |
| ≥1280 | labelled rail 250px | 600px | 350px |
| 1024–1279 | icon rail 88px + tooltips | 600px | 300px |
| 768–1023 | icon rail 88px | flexible | hidden (decorative) |
| ≤767 | bottom bar 56px + safe-area + compose FAB | full width | hidden |

There is no global page-title bar. Each surface owns its header: the conversation
rail and thread headers, the profile identity strip, and a back-only post header.
Account controls (theme, sign out) live in the rail account menu and the mobile
profile menu.

The shell clips on the x axis and every flexible column is `min-w-0`, so there
is no horizontal scroll with long handles, long words, or wide images.

## What the backend actually offers

Verified against `api-gateway/graph/schema.graphqls` and the services behind it:

- **Tag search exists** through `searchPosts`. post-service extracts hashtags
  from the same string and ORs a GIN-indexed `hashtags && $2::text[]` overlap on
  top of tsvector and trigram matching, so `#golang` and `golang` both work.
  There is no separate tag endpoint, and none is needed.
- **The feed is follow-only.** `getFeed` resolves the viewer from the token and
  reads the timeline fanout built from followed authors. There is no global
  feed, so there are no feed tabs.
- **There is no trending endpoint.** The rail counts hashtags in the viewer's
  own feed and labels itself "in your feed" rather than inventing a global list.
- `User` has no `bio` and no `joinedAt`; the profile uses `createdAt`.
- `getAllPost` has no cursor; `getFeed` returns `FeedPost` (no `updatedAt`);
  `likePost` takes `{ postId, userId }`; `NotificationType` has no `MESSAGE`.

## Icon scale

One scale in `tokens.css`: `--icon-xs` 18, `--icon-sm` 20, `--icon-md` 22,
`--icon-lg` 26. Components use `size-icon-*`.

## Search field

The Explore field renders exactly one icon: the magnifier on the left. There is
no second clear icon — `Esc` clears the field and a keyboard hint says so. The
previous version showed a magnifier and a separate X at the same time.

## Verified

| Check | Result |
| :--- | :--- |
| `npm run typecheck` | Clean (`strict`, `noUncheckedIndexedAccess`) |
| `npm run build` | 14 routes, 200 kB first-load JS on `/` |
| Live API | 11/11 smoke checks: login, session, feed, tag/text/people search, notifications, conversations, comments, follow counts |
| Mutations through the proxy | `CreatePost`, `likePost`, `sendMessage`, `logout` all verified |
| Auth | Cookie set → session resolves → feed authorized; logout clears session |

## Known gaps

- **Subscriptions.** `graphql-ws` is installed; no screen subscribes. Chat and
  notifications refresh by query invalidation, not push.
- **Media upload.** `getUploadSignature` exists; the composer still posts text
  only. Seeded posts carry images from Cloudinary's public demo account.
- **Moderation queue.** `getReports` is admin-only and there is no role UI.

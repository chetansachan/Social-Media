# SocialFlow Newsfeed Frontend — Next.js Rebuild

**Date:** 2026-11-09
**Status:** design approved in conversation, awaiting spec review
**Supersedes:** `frontend/design/Home-X-Guidelines.md` §2.1 (typography) and §2.2 (colour), which this document re-measures and replaces. Every other rule in that contract carries forward unchanged and is restated here as binding.

---

## 1. Intent

SocialFlow has a complete Go backend — eight services, a GraphQL gateway, WebSocket subscriptions — and no frontend. An earlier Vite/React frontend was committed and then removed. This design rebuilds it on Next.js with a real design system, honouring the existing UI contract rather than starting from taste.

**Who it is for:** readers of a technical social feed, on phones and desktops, some of them keyboard-only or using a screen reader.

**Success looks like:** every screen renders and is reviewable with no backend running; every interaction is reachable from the keyboard; every colour decision is measured; the feed holds 20,000 items without allocating 20,000 nodes; and the production bundle stays inside the budget in §10.

**Non-goals:** pixel parity with any existing product; a native mobile app; server-side rendering of personalised data (auth is cookie-based and per-request, so the feed is client-rendered behind a shell that streams).

---

## 2. Decisions locked in conversation

These came from the visual companion session, not from assumption. The click stream is in `.superpowers/brainstorm/223-1794221420/state/events`.

| # | Decision | Chosen | Rejected |
|---|---|---|---|
| D1 | **Scope** | All five screens (Home, Explore, Notifications, Messages, Profile) + the `/states` QA page | Feed-first, or feed+explore+profile |
| D2 | **Theme** | Dark-first, **both** themes, toggle in header, persisted, respects `prefers-color-scheme` on first visit | Dark only; light-first |
| D3 | **Type** | Newsreader (display/headlines) + Inter (UI/body) + JetBrains Mono (handles, counters, labels) | Instrument Serif + Geist; Source Serif 4 + Source Sans 3 |
| D4 | **Transport** | Next.js route handler proxy + **httpOnly** cookies | Direct-to-gateway with localStorage; proxy with in-memory token |
| D5 | **Visual direction** | **Hybrid** — three-column layout from A (labelled nav rail + interests, centre feed, trending rail) with the media-forward card design from B | A alone; B alone; monochrome |
| D6 | **Media band** | Rendered **only when the post has a real `mediaUrl`**. Text posts stand alone — no invented gradient placeholder | Gradient placeholder on every card |
| D7 | **Phone tabs** | **Keep** For you / Following | Drop them for vertical space |
| D8 | **Responsive collapse** | Right rail drops first (≤1024), nav rail → icon-only (≤1024), bottom bar on phones (≤640) | Other orders |

### 2.1 Why D6 matters more than it looks

In the gateway's own fixture, three of five posts have `mediaUrl: ""`. A generated placeholder on every card would mean the majority of the feed is decoration standing in for nothing. D6 keeps the feed honest and is the reason the card is designed to work **text-first**, with media as an enhancement.

---

## 3. Architecture

### 3.1 Stack and the reason for each piece

| Piece | Version | Why it is here, tied to a contract rule |
|---|---|---|
| Next.js App Router | 15.x | Route-level code splitting for the five destinations (§10), streaming shell, server-side cookie access for the proxy |
| React | 19.x | Required by Next 15 |
| TypeScript | 5.x, `strict` | The contract's error states are typed results, not thrown strings |
| Tailwind CSS | v4 (`@theme`) | Tokens become CSS custom properties — the single-source-of-truth rule in §6 survives the move from hand-written CSS |
| shadcn/ui | latest | Components are copied in, not depended on. Radix underneath supplies the roving-tabindex and ARIA-tabs primitives that checks A4 and A15 demand |
| TanStack Query | v5 | Infinite feed query, optimistic like with rollback (§8.4), one cache for server state |
| TanStack Virtual | v3 | The windowing requirement — `ceil(viewport / 220) + 4` rows mounted |
| TanStack Table | v8 | The moderation report queue (`getReports`) |
| GSAP + ScrollTrigger, `@gsap/react` | 3.13.x | Entrance choreography and the like burst, through one module, with `useGSAP` cleanup |
| `next/font` | built in | Self-hosts Newsreader/Inter/JetBrains Mono. No runtime request to Google, no layout shift |
| Zod | v3 | Validates the login form and the proxy's request body |
| `graphql-ws` | v5 | Subscriptions, direct to the gateway |

**Rejected:** TanStack Router (would duplicate the App Router), Redux (TanStack Query already owns server state; the only client state is the session and theme), Zustand (not warranted at this size), Recoil (deprecated).

### 3.2 The transport problem, and the fix

The gateway at `api-gateway/server.go:220` mounts GraphQL on `/query` with **no CORS middleware anywhere in the stack**. A browser page served from `localhost:3000` therefore cannot call `localhost:8080` cross-origin. Three facts shape the design:

1. **Queries and mutations go through a Next.js route handler** at `POST /api/graphql`. It reads the bearer token from an httpOnly cookie, attaches it as an `Authorization` header, forwards the body, and returns the response verbatim. Servers are not subject to CORS, so this works with no change to the Go services.
2. **Subscriptions bypass the proxy.** WebSockets are not subject to the same-origin policy, so the browser connects directly to `ws://localhost:8080/query`. The token travels in the `connection_init` payload, which `auth.WebsocketInit` already reads. Proxy-ing WebSockets through Next would mean owning a socket per client for no benefit.
3. **The token never reaches JavaScript.** It lives in an httpOnly, `SameSite=Lax` cookie. An XSS in any dependency cannot exfiltrate it.

**Trade-off accepted:** server components cannot use the same client instance as the browser, because the token is read per request. The feed is therefore client-rendered; the shell, nav, and static chrome stream from the server. This costs a little first-paint time on the feed itself and buys XSS-safety plus a single place that knows what "gateway is down" means.

### 3.3 Data flow

```
browser ──POST /api/graphql──▶ route handler ──▶ http://localhost:8080/query
   │                              (reads httpOnly cookie,            │
   │                               attaches Bearer,                  │
   │                               maps failures→typed codes)        │
   │                                                                  │
   └──WSS /query (subscriptions only)──────────────────────────────────┘
```

Failure mapping in one place — the handler converts these into a typed envelope the client already understands:

| Upstream | Client sees | UI result |
|---|---|---|
| `ECONNREFUSED` | `GATEWAY_DOWN` | Demo-data banner on every screen |
| HTTP 401 / `UNAUTHENTICATED` | session cleared | Signed-out state, sign-in prompt in place of the composer |
| HTTP 429 (`ratelimit/guard.go`) | `RATE_LIMITED` | Retry control with the `Retry-After` value |
| GraphQL `errors[]` non-empty | `firstError.code` | Attached to the control that failed, not just a banner |

### 3.4 Route map

| Route | Kind | Data |
|---|---|---|
| `/` | client feed inside server shell | `getFeed` infinite |
| `/explore` | server shell + client search | `searchPosts`, `searchUsers` |
| `/notifications` | client, subscribes | `getNotifications`, `notificationReceived` |
| `/messages` | client | `myConversations` |
| `/messages/[id]` | client, subscribes | `getMessages` + `chatEvent` |
| `/profile/[username]` | server shell | `getUserByUsername`, `getAllPost`, `getFollowCounts` |
| `/states` | static | none — renders the fixture |
| `/api/graphql` | route handler | proxy |
| `/api/auth/login` · `logout` · `refresh` | route handlers | set/clear cookies |

---

## 4. Design tokens

`src/styles/tokens.css` is the **only** file permitted to contain a raw hex, a raw spacing value, or a raw font size. Tailwind's `@theme` maps utilities onto those variables, so `bg-surface-raised` and `text-secondary` resolve to tokens and no component can smuggle in a literal. This preserves the contract's §2 rule and its grep check.

### 4.1 Typography

| Token | Family | Size | Use |
|---|---|---|---|
| `--font-display` | Newsreader (variable, 300–700, real italics) | — | Card headlines, page titles, brand wordmark |
| `--font-sans` | Inter (variable) | — | Body, controls, labels, meta |
| `--font-mono` | JetBrains Mono | — | Handles, counters, timestamps, hashtag chips, section labels |

Scale (unchanged from the contract's 11→30 range, retargeted to the new pairing):

| Token | Value | Use |
|---|---|---|
| `--text-2xs` | 11px | trending-rail micro labels |
| `--text-xs` | 12.5px | meta rows, action counts |
| `--text-sm` | 13.5px | secondary text, card body, help |
| `--text-base` | 15px | default UI text |
| `--text-lg` | 17px | emphasised body |
| `--text-xl` | 19px | **card headline (Newsreader)** |
| `--text-2xl` | 22px | section headings |
| `--text-3xl` | 28px | page titles |
| `--text-4xl` | 34px | display / empty-state headings |

Newsreader needs ~0.01em tighter tracking than Inter at display sizes; that is one token, `--tracking-display: -0.008em`, not a per-component value.

### 4.2 Measured contrast — dark theme (default)

Surfaces: page `#0b1017`, raised/card `#0e0e16`, overlay `#14141f`, inset `#12121c`.

| Token | Value | on page | on raised | Verdict |
|---|---|---|---|---|
| `--color-text-primary` | `#f2f2f7` | 17.10 | 17.22 | AAA |
| `--color-text-secondary` | `#a9a9bd` | 8.26 | 8.32 | AAA |
| `--color-text-tertiary` | `#8b8ba3` | 5.74 | 5.78 | **AA at every size** |
| `--color-text-accent` | `#a78bfa` | 7.01 | 7.06 | AAA |
| `--color-text-accent-alt` | `#5fdce8` | 11.71 | 11.79 | AAA |
| `--color-text-danger` | `#f4576b` | 5.83 | 5.88 | AA at every size |
| `--color-text-success` | `#3ddc97` | 10.80 | 10.87 | AAA |

**The contract's tertiary restriction is lifted.** The old palette's `#71767b` measured 4.04:1 and needed aliases for body text. `#8b8ba3` measures 5.74:1, so it is legal at every size and the `--color-text-tertiary-large` / `-essential` aliases are deleted rather than kept as ceremony. The rule that replaces them is simpler: **every text token in this table is AA at 12.5px or larger**, and no alias exists to be misused.

### 4.3 Measured contrast — light theme

Surfaces: page `#faf9f7`, raised/card `#ffffff`, overlay `#ffffff`, inset `#f1efeb`.

| Token | Value | on page | on raised | Verdict |
|---|---|---|---|---|
| `--color-text-primary` | `#12121a` | 17.71 | 18.63 | AAA |
| `--color-text-secondary` | `#4c4c5a` | 8.02 | 8.44 | AAA |
| `--color-text-tertiary` | `#5d5d6b` | 6.15 | 6.47 | AAA |
| `--color-text-accent` | `#5b3df5` | 5.82 | 6.12 | AA |
| `--color-text-accent-alt` | `#116b7a` | 5.86 | 6.16 | AA |
| `--color-text-danger` | `#c01840` | 5.79 | 6.09 | AA |
| `--color-text-success` | `#0a6b50` | 6.18 | 6.50 | AA |

### 4.4 Interactive boundaries — the measured fix

WCAG 2.2 §1.4.11 requires **3:1** for boundaries that carry meaning: input borders, focus rings, tab indicators, checkbox/radio edges, and the filled edge of a state indicator. Purely decorative dividers between cards are exempt — but the first draft of this palette failed for both categories, and the fix is split accordingly.

| Role | Dark | Light | Rule |
|---|---|---|---|
| `--color-border` (decorative dividers) | `#26263a` | `#e0ded8` | **Exempt.** Visible separation only; no meaning is lost if it disappears. |
| `--color-border-interactive` | **`#8050b2`** | **`#9482b2`** | **≥3:1 mandatory.** Input edges, unfocused control outlines. Dark measures 3.37/3.39/3.23/3.28; light measures 3.27/3.45/3.00. |
| `--color-focus-ring` | `#8050b2` | `#9482b2` | Same measured pair, so the ring is never the thing that fails check A1. |

Measured with `tmp/contrast.cjs` and solved with `tmp/contrast-solve.cjs`, both committed so the numbers can be reproduced rather than trusted.

### 4.5 The accent gradient — text-free by rule

The visual direction uses a violet→teal gradient. Measured as flat ink it is **not** safe behind text:

| Pair | Ratio | Verdict |
|---|---|---|
| white on `#7c5cff` | 4.35 | **fails 4.5** for button labels |
| white on `#3fd0e0` | 1.86 | **fails badly** |
| `#7c5cff` as ink on card | 4.42 | passes for large text only |

So the gradient is governed by one rule: **`--gradient-accent` may never sit behind text.** It is used only where 3:1 applies — the like-heart fill, the active tab indicator, the brand dot, and the "Post" button's *glow*, never its label. Text on the primary button sits on the solved flat fill below.

| Token | Dark | Light | Constraint it satisfies |
|---|---|---|---|
| `--color-accent-solid` | `#8428f8` | `#4428f8` | White label at 5.77 (dark) / 7.23 (light); edge vs card at 3.33 / 7.23 |
| `--gradient-accent` | `linear-gradient(135deg, #7c5cff, #3fd0e0)` | `linear-gradient(135deg, #5b3df5, #1e9fb0)` | Non-text use only |

### 4.6 Spacing, radius, elevation, motion

Carried forward from the contract with one addition (`--space-9` = 24px for the hybrid card's inner padding) and otherwise unchanged: the scale is not re-derived, because a second scale is how token systems die.

| Group | Tokens |
|---|---|
| Spacing | `--space-1` 1px … `--space-8` 20px, `--space-9` 24px, plus composites `--space-control-x/y`, `--space-card-x/y`, `--space-gutter`, `--space-nav-item` |
| Radius | `--radius-xs` 4px (chips), `--radius-md` 12px (controls), `--radius-lg` 16px (composer), `--radius-xl` 18px (cards), `--radius-full` 9999px |
| Elevation | `--shadow-1` card hover, `--shadow-2` overlay |
| Motion | `--motion-duration-instant` 200ms, `--reveal` 320ms, `--celebration` 480ms; `--motion-ease-standard`, `--motion-ease-entrance` |
| Control metrics | `--control-height-sm/md/lg` 28/34/42px, `--focus-ring-width` 2px, `--focus-ring-offset` 2px |

---

## 5. Components

The contract's seven-state rule (default, hover, focus-visible, active, disabled, loading, error) is unchanged and applies to every component below. Only additions and deltas are recorded here.

### 5.1 Component inventory

| Group | Components | Notes |
|---|---|---|
| Primitives (shadcn) | `Button`, `IconButton`, `Input`, `Textarea`, `Avatar`, `Badge`, `Skeleton`, `Separator`, `Tooltip`, `DropdownMenu`, `Dialog`, `Sheet`, `Tabs`, `ScrollArea`, `Switch`, `Toast` | Copied in, then restyled onto the tokens in §4 |
| Chrome | `AppShell`, `SideRail` (labelled), `IconRail`, `BottomNav`, `TopBar`, `SkipLink`, `TrendingRail`, `ThemeToggle` | `AppShell` owns the responsive collapse from §7 |
| Feed | `Composer`, `PostCard`, `MediaBand`, `PostActions`, `LikeButton`, `FeedList`, `ShowMoreButton`, `Tombstone`, `PostSkeleton` | `FeedList` is the TanStack Virtual surface |
| Explore | `SearchField` (combobox), `PostResultList`, `UserResultList`, `TrendChip` | |
| Notifications | `NotificationList`, `NotificationRow`, `ActorStack`, `UnreadBadge`, `MarkAllReadButton` | Subscribes to `notificationReceived` |
| Messages | `ConversationList`, `ConversationRow`, `MessageThread`, `MessageBubble`, `MessageComposer`, `TypingIndicator`, `PresenceDot` | Subscribes to `chatEvent` |
| Profile | `ProfileHeader`, `FollowCounts`, `FollowButton`, `ProfileTabs` | Reuses the ARIA tabs pattern |
| States | `StateGrid`, `StateCell` | Renders every component in all seven states on `/states` |
| Motion | `Reveal`, `useLikeBurst`, `useIndicator` | Thin wrappers over `lib/motion.ts`; no component calls `gsap.*` directly |

### 5.2 `PostCard` — anatomy and the D6 consequence

```
┌─ [avatar] name @handle · 1h                    [⋯] ─┐
│  headline            (Newsreader 19px, clamped 2)  │
│  body                (Inter 13.5px, clamped 3)     │
│  [media band]        ONLY when mediaUrl is set     │
│  #tags               (JetBrains Mono chips)        │
│  ♥ 1.3K   ♺ 214   ↗ Share   ⌘                  │
└────────────────────────────────────────────────────┘
```

The headline/body split is the design's one real invention: posts get a **display headline** in Newsreader and body copy in Inter. Since the API returns a single `content` string, the client derives the headline as the first sentence (first `.`, `!`, or `?`, max 120 chars) and treats the remainder as body. When there is no remainder, the headline stands alone and no body renders. This is a **presentation-only** transform, pure and unit-tested, and it must never alter the text it renders.

States specific to this card:

| State | Rule |
|---|---|
| no media (D6) | Body top-margin collapses; the card is shorter, not padded with a placeholder |
| long content | Headline clamps at 2 lines, body at 3, **Show more** is a real `<button>` that expands and announces `aria-expanded`. No gradient mask (contract §6 anti-pattern 9) |
| liked | `aria-pressed="true"`, heart fills at `--color-text-danger`, accessible name becomes "Unlike" |
| tombstone | Dashed `--color-border-interactive`, "This post was removed by its author." Row stays so thread rhythm survives |
| skeleton | `aria-hidden` + `aria-busy` on the container; skeleton **keeps the final height** so nothing jumps |
| media failure | `role="alert"` caption inside the figure; frame keeps its 16:10 ratio |

---

## 6. Responsive design

Breakpoints and the collapse order from D8. Verified at 360 / 414 / 768 / 1024 / 1280 / 1600.

| Width | Nav | Feed | Right rail | Notes |
|---|---|---|---|---|
| ≥1280 | Labelled rail 218px + interests | flexible, max 640px | 224px trending + who-to-follow | Full hybrid |
| 1024–1279 | Icon rail 64px, tooltips carry the labels | flexible | narrows to 220px | Labels move into `aria-label` + tooltip, so A4 still passes |
| 768–1023 | Icon rail 64px | flexible | **hidden** | Dropped first: decorative |
| 640–767 | Icon rail 64px | flexible | hidden | |
| ≤639 | **Bottom bar**, five destinations, 56px + safe-area inset | full width | hidden | Top bar carries avatar + wordmark; For you / Following tabs per D7 |

Rules that hold at every width:

- No horizontal scroll, ever. `overflow-x: clip` on the shell and a test that asserts `scrollWidth === clientWidth`.
- Long headlines clamp rather than wrap to three lines; controls truncate with `title`, never wrap.
- Touch targets ≥24px (A9); the bottom bar's are 44px.
- The bottom bar adds `env(safe-area-inset-bottom)` so it clears the iOS home indicator.
- Under `pointer: coarse`, `:active` residue is suppressed (A12).
- Icon rail and bottom bar labels are real text in the DOM at ≥1024 (visually hidden below), so screen readers never lose a destination.

---

## 7. Accessibility — carried forward, plus deltas

The contract's A1–A15 remain the acceptance criteria verbatim. What changes:

| Check | Change |
|---|---|
| A5 (body contrast) | **Strengthened.** All text tokens now measure ≥5.74:1 (dark) / ≥5.79:1 (light) at every size, so no size restriction exists to enforce. |
| A6 (non-text contrast) | **Now measured, not asserted.** `--color-border-interactive` is 3.37/3.28 dark and 3.27/3.00 light (§4.4). |
| A15 (roving tabindex) | Extended to the bottom bar and the trending rail's hashtag list. |
| new A16 | **Theme switch announces.** Toggling theme sets `aria-pressed` on the toggle and the change is perceivable without relying on colour. |
| new A17 | **Reduced motion covers theme + reveal together.** Switching theme must not animate when `prefers-reduced-motion: reduce`. |
| new A18 | **Subscriptions are announced.** New notifications arrive in an `aria-live="polite"` region with the actor name; a silent count change is a fail. |

---

## 8. Behaviour and data

### 8.1 Query keys

```
["feed", { tab, limit }]                    infinite
["post", id]                                single
["comments", postId, parentId]              infinite
["search", "posts"|"users", query]          debounced 250ms
["conversations"]                           infinite
["messages", conversationId]                infinite
["notifications", filter]                   infinite
["presence", ...userIds]                    polled 30s
["moderation", "status", userId]
["reports", status]                         table
```

### 8.2 Demo mode (contract-mandated)

When the proxy returns `GATEWAY_DOWN`, the feed renders `src/lib/mock.ts` (ported from `f640293`) and a dismissible-but-returning banner reads: *"Cannot reach the gateway. Showing demo posts."* The fixture deliberately includes the hard cases: a post with no media, a 4,000-character caption, an empty feed, an unread conversation, a deleted-message tombstone, and a 20,000-item synthetic feed for the virtualization test.

### 8.3 Auth

`POST /api/auth/login` → validates with Zod, calls the gateway `Login`, writes `sf_access` (15 min) and `sf_refresh` (7 days) as httpOnly cookies, returns only the user's display fields. `POST /api/auth/logout` → calls the gateway's `logout` mutation (which blacklists the token per `auth/revocation.go`), then clears both cookies. The proxy refreshes once on a 401 and retries the original request exactly once; a second 401 clears the session (contract: revoked token behaves exactly like logged-out).

### 8.4 Optimistic like, with the contract's rollback rule

`useMutation` on `likePost`: cancel in-flight `["feed"]` queries → snapshot → flip `hasLiked` and `likeCount` in every cached page → on error, restore the snapshot **and** surface the reason on the button itself (`role="alert"` under the control), not only in a toast. Contract §5 is explicit that a silent revert is a fail.

### 8.5 Windowing

`FeedList` uses TanStack Virtual with `estimateSize: () => 220` and `overscan: 4`, mounting `ceil(viewport / 220) + 4` rows. Measured on the 20,000-item fixture with a DOM node count assertion in the test suite.

---

## 9. Error and edge-case matrix

| Situation | Required rendering |
|---|---|
| Gateway down | Demo banner + demo content. Never a blank screen. |
| `UNAUTHENTICATED` | Session cleared; composer replaced by "Sign in to publish a post." |
| Rate limited (429) | Retry control carrying the `Retry-After` seconds |
| Empty feed | "Your feed is empty. Follow someone to fill it." + a link to Explore |
| No search results | `No matches for "golang"` + a next step |
| Media 404 | `role="alert"` caption; the frame keeps its ratio |
| Post deleted while open | Tombstone, thread rhythm preserved |
| Optimistic action fails | Reverts **and** explains, attached to the control |
| Conversation empty | "No messages yet. Say hello." |
| 4,000-char post | Clamps, expands via a real button |
| Slow network | Skeletons that preserve final height |
| JS disabled | Server-rendered shell with nav and a noscript explanation; the feed needs JS and says so |

---

## 10. Performance budget

| Metric | Budget | How |
|---|---|---|
| LCP (feed, cold, Fast 3G) | < 2.5s | Shell streams; skeleton paints before data |
| CLS | < 0.05 | Font metrics matched via `next/font` `adjustFontFallback`; skeletons match final height; media frames reserve 16:10 |
| INP | < 200ms | Virtualized rows; no layout-property animation; every handler under 50ms |
| First-load JS, `/` | < 180 KB gzip | Route-level splitting; `optimizePackageImports` for `lucide-react`; GSAP core + ScrollTrigger only, imported in the two components that use them |
| DOM nodes, 20,000-item feed | < 120 | Windowing (§8.5) |
| Runtime Google Fonts requests | 0 | `next/font` self-hosts |
| Lighthouse (mobile) | ≥ 95 Perf, 100 A11y, 100 BP, 100 SEO | Verified before the build is called done |

Techniques: `next/dynamic` for the GSAP-heavy composer and the states page; `next/image` with explicit `sizes` for avatar and media; `staleTime` 30s on the feed, 5 min on profiles; route prefetch on nav hover; `content-visibility: auto` on off-screen chat bubbles.

---

## 11. Verification plan

Every claim above is checkable. No item is "reviewed by eye".

| # | Check | Method |
|---|---|---|
| V1 | Type-safety | `tsc --noEmit` clean, `strict: true` |
| V2 | Production build | `next build` succeeds, first-load JS within §10 |
| V3 | **No raw hex outside tokens** | Grep across `src/**` excluding `tokens.css`; must return zero hits |
| V4 | Contrast table | Re-run `tmp/contrast.cjs`; zero failures on text, and interactive boundaries ≥3:1 |
| V5 | Seven states | `/states` renders every component in all seven; a Playwright snapshot per state |
| V6 | Keyboard completion | Playwright: create a post, like one, switch a tab, open a thread, toggle theme — keyboard only |
| V7 | Focus visibility | Every focusable element shows a 2px ring at 2px offset while tabbing the whole page |
| V8 | Responsive | `scrollWidth === clientWidth` asserted at 360/414/768/1024/1280/1600 |
| V9 | Windowing | 20,000-item feed mounts <120 nodes |
| V10 | Reduced motion | With `reduce`, GSAP tweens are skipped and nothing is left invisible |
| V11 | Demo fallback | Kill the gateway: every screen renders with the banner |
| V12 | Auth revocation | Logout blacklists the token; a stale cookie lands on the signed-out state |
| V13 | a11y tree | `@axe-core/playwright` zero violations on all six routes |
| V14 | Theme persistence | Toggle survives reload; first visit follows `prefers-color-scheme` |
| V15 | Font metrics | CLS < 0.05 on cold load, measured in Playwright |

---

## 12. File structure

```
frontend/
  src/
    app/
      layout.tsx              fonts, theme script, providers
      page.tsx                Home (For you / Following)
      explore/page.tsx
      notifications/page.tsx
      messages/page.tsx
      messages/[id]/page.tsx
      profile/[username]/page.tsx
      states/page.tsx
      api/graphql/route.ts    the proxy
      api/auth/{login,logout,refresh}/route.ts
      globals.css             @import tailwindcss + tokens
    components/
      ui/                     shadcn, restyled onto tokens
      chrome/                 AppShell, rails, nav, skip link, theme toggle
      feed/  explore/  notifications/  messages/  profile/  states/
      motion/                 Reveal, useLikeBurst, useIndicator
    lib/
      graphql/{client,operations,types}.ts
      hooks/{use-feed,use-like,use-session,use-theme,use-subscription}.ts
      motion.ts               the only module that calls gsap.*
      format.ts               compact numbers, relative time, headline split
      mock.ts                 the fixture
      utils.ts                cn()
    styles/tokens.css         the only file with raw values
  tests/
    e2e/                      Playwright: V5–V15
    unit/                     vitest: format, headline split, contrast table
```

---

## 13. Out of scope

Deliberately not built, with the reason:

- **Media upload.** `getUploadSignature` exists and returns a Cloudinary signature. Wiring a real upload UI means an unsigned-preset decision and a storage account; the composer accepts a URL for now and the contract's upload path is a separate task.
- **Admin report queue UI as a *product* screen.** The TanStack Table surface is built and reachable at `/states` for verification, but no nav entry — the schema marks `getReports` admin-only and there is no role model in the UI yet.
- **Push notification registration.** `registerDeviceToken` is for native clients; the web app uses in-app subscriptions only.
- **Group creation UI.** `createConversation` supports it; the first build opens existing threads and direct messages, which is what the contract's message screen describes.
- **Offline/PWA.** No service worker. Would double the caching model for a feed that is cheap to re-fetch.

---

## 14. Open questions

None blocking. Two to settle during implementation, both cosmetic and reversible:

1. Whether the trending rail's hashtags should be clickable through to `/explore?q=#tag` in this build, or rendered as plain text until Explore's query param handling exists.
2. Whether `--color-border-interactive` at `#8050b2` (a violet-tinted grey) reads as intentional in the light theme at `#9482b2`, or whether a neutral `#8a8a86`-family value is preferable once seen at full size. Both pass the 3:1 requirement; this is taste within a measured constraint.

# Home / X — frontend

React + TypeScript + Vite, animated with GSAP, themed entirely by design tokens.
Every colour, spacing, and type decision lives in `src/styles/tokens.css`;
`src/styles/components.css` contains no raw value and neither should any new
component.

**Full component rules, states, and the 15 testable accessibility criteria:
[design/Home-X-Guidelines.md](design/Home-X-Guidelines.md).**

## Quick start

```bash
cd frontend
npm install
npm run dev        # http://localhost:5173
```

`npm run dev` proxies `/query` to `http://localhost:8080` (the Go API gateway).
If the gateway is not running, the app renders demo data and says so in a banner,
so every screen is reviewable on its own.

| Command | What it does |
| :--- | :--- |
| `npm run dev` | Vite dev server with hot reload. |
| `npm run build` | Type-check, then a production bundle into `dist/`. |
| `npm run preview` | Serve the built bundle locally. |
| `npm run typecheck` | Type-check only. |

## Routes

| Route | Screen |
| :--- | :--- |
| `#/` | Home — the feed, the composer, the list state matrix |
| `#/explore` | Search — combobox, posts and people |
| `#/messages` | Direct and group threads, tombstones, unread counts |
| `#/notifications` | Inbox, unread summary, mark-all-read |
| `#/states` | **Every component in every state** — the page to review first |

## Layout

```
frontend/
  design/            guidelines (the component contract)
  src/
    lib/             api client, GSAP motion, formatting, demo data
    state/           session (token in memory + sessionStorage)
    components/      Button, Link, Field, PostCard, FeedList, Navigation, Composer, AppShell
    pages/           Home, Explore, Messages, Notifications, States
    styles/          tokens.css (only raw values), base.css, components.css
```

## Talking to the backend

`src/lib/api.ts` holds every GraphQL document used, next to its response type.
Adding a field to the gateway schema and not adding it here is a bug, so keep the
two in the same commit. Authentication is a bearer token held in memory; a
`UNAUTHENTICATED` error clears the session, so a revoked token behaves exactly
like a logged-out one on every screen.

## Motion

All animation goes through `src/lib/motion.ts`, which reads durations from
tokens and honours `prefers-reduced-motion` in one place. The like burst scales
rather than translates, and entrance choreography places elements at their final
state immediately when reduced motion is requested — nothing is ever left
invisible.

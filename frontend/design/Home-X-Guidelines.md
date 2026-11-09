# Home / X — implementation-ready UI guidance

**Design intent in one sentence:** a token-driven, content-first reading surface
where every component states, every interaction is reachable from the keyboard,
and every colour decision is measured rather than chosen.

This document is the contract for `frontend/`. Where it says **must**, the
implementation enforces it and a reviewer can reject a change that does not.
Where it says **should**, it is a strong default that a documented reason can
override.

---

## 1. Context and goals

| Item | Value |
| :--- | :--- |
| Product / brand | Home / X |
| Surface | content site |
| Audience | readers and knowledge seekers |
| Target | WCAG 2.2 AA |
| Known page density | buttons 86, links 80, cards 7, navigation 5, inputs 3, lists 1 |

Goals, in priority order: consistency across screens, accessibility, and
delivery speed. A local visual exception is more expensive than it looks,
because it has to be re-derived by every future screen.

Non-goals: pixel parity with any external product, and an animation system of
its own (motion is delegated to GSAP through one module — §3.6).

---

## 2. Design tokens and foundations

Every token below lives in `frontend/src/styles/tokens.css`. **That file is the
only place a raw hex, a raw spacing value, or a raw font size may appear.**

### 2.1 Typography

| Token | Value | Use |
| :--- | :--- | :--- |
| `--font-family-primary` | Times New Roman | headings |
| `--font-family-stack` | Times New Roman, Times, serif | body, all components |
| `--font-size-xs` | 11px | meta, counters, labels under icons |
| `--font-size-sm` | 13px | secondary text, help, handles |
| `--font-size-md` | 13.33px | body inside cards and lists |
| `--font-size-lg` | 14px | emphasised body, control labels |
| `--font-size-xl` | 15px | base body text (the default) |
| `--font-size-2xl` | 20px | card titles, section headings |
| `--font-size-3xl` | 22.5px | page headings |
| `--font-size-4xl` | 30px | display, hero, empty-state headings |
| `--font-weight-base` | 400 | |
| `--line-height-base` | normal | tightened to 1.2 for headings, loosened to 1.45 for prose |

The scale is 11 → 30px. A component **must** pick one of these; a value between
two of them is a new token, added here with a name, not a one-off.

### 2.2 Colour

| Token | Value | Measured on `--color-surface-raised` |
| :--- | :--- | :--- |
| `--color-text-primary` | `#e7e9ea` | 15.2:1 — AAA |
| `--color-text-secondary` | `#9e9eff` | 7.8:1 — AA |
| `--color-text-tertiary` | `#71767b` | **4.0:1 — see the rule below** |
| `--color-text-inverse` | `#1d9bf0` | 6.2:1 — AA (primary action) |
| `--color-surface-raised` | `#0f1419` | page surface |
| `--color-surface-muted` | `#eff3f4` | light surface, inset panels |
| `--color-border-default` | `#000000` | legal only on `--color-surface-muted` |
| `--color-border-strong` | `#4b4e52` | 3.3:1 — non-text boundary |

Derived tokens (still one file, still no raw hex in components):

| Token | Derivation | Why it exists |
| :--- | :--- | :--- |
| `--color-text-on-muted` | `#0f1419` | The primary text colour is near-white and unreadable on the light muted surface. |
| `--color-text-secondary-on-muted` | `#1d9bf0` | 7.0:1 on muted. |
| `--color-border-on-raised` | `#4b4e52` | The palette's `border.default` is black and vanishes on the dark surface. |
| `--color-text-tertiary-large` | alias of tertiary | Legal where 4.0:1 passes: ≥24px regular or ≥18.66px bold. |
| `--color-text-tertiary-essential` | alias of primary | Any *essential* text, at any size. |
| `--color-text-danger` / `--color-text-success` | mixed from the palette | Feedback ink without extending the palette with new hues. |

**The tertiary rule.** Measured contrast for `#71767b` on `#0f1419` is 4.04:1.
That passes WCAG 2.2 AA for large text and for non-text UI (3:1) and **fails**
the 4.5:1 body-text requirement. Rather than ship a failing colour or quietly
substitute a different hex in components, tertiary is restricted to the two
cases where 4.04:1 is legal, and aliases are provided for the cases where it is
not. A reviewer **must** reject `--color-text-tertiary` used for text below
24px.

### 2.3 Spacing, radius, elevation, motion

| Group | Tokens |
| :--- | :--- |
| Spacing | `--space-1` 1px … `--space-8` 20px, plus composites `--space-control-x/y`, `--space-card-x/y`, `--space-gutter`, `--space-nav-item` |
| Radius | `--radius-xs` 16px (cards, media), `--radius-sm` 9999px (pills, avatars, buttons), `--radius-focus` |
| Elevation | `--shadow-1`, `--shadow-2` exactly as specified |
| Motion | `--motion-duration-instant` 200ms; derived `--motion-duration-reveal` 320ms and `--motion-duration-celebration` 480ms; easings `--motion-ease-standard`, `--motion-ease-entrance` |
| Control metrics | `--control-height-sm/md/lg` 28/34/42px, `--focus-ring-width`, `--focus-ring-offset` |

A component **must not** add two spacing tokens to produce a third value. If the
composition is needed twice, it becomes a composite token.

---

## 3. Component rules

Density targets: buttons 86, links 80, cards 7, navigation 5, inputs 3, lists 1.
Every component below defines **all seven states**: default, hover,
focus-visible, active, disabled, loading, error.

### 3.1 Button

Anatomy: `[spinner?] [label] [icon?] [error?]`.

Variants: `primary` (the only filled treatment — 6.2:1 on the page surface),
`secondary`, `ghost`, `danger`.

States:

| State | Rule |
| :--- | :--- |
| default | Variant tokens only; no shadow. |
| hover | Colour shift, 200ms. Never a layout change. |
| focus-visible | 2px ring at 2px offset in `--color-text-inverse`; pointer presses **must not** ring. |
| active | `scale(0.98)`; suppressed under `pointer: coarse`. |
| disabled | 50% opacity, `cursor: not-allowed`, **label stays** — greying out is not a reason to hide what it does. |
| loading | Spinner + label, width unchanged; `aria-busy`; a click while loading is swallowed, not queued. |
| error | Border in danger ink **and** a `role="alert"` message; the button stays operable so the user can retry. |

Interaction:

- **Keyboard:** Enter and Space activate. Space **must not** scroll the page —
  `keydown` is intercepted for exactly that reason.
- **Pointer:** hover/active are CSS states; no JS.
- **Touch:** under `pointer: coarse`, `:active` is suppressed so a tap does not
  leave the button looking pressed.

Responsive: sizes map to `--control-height-*`; a full-width variant exists for
forms below 600px. **Overflow:** the label truncates with an ellipsis and a
`title` attribute rather than wrapping to three lines.

### 3.2 IconButton

Required: `label` prop → accessible name. An icon-only control without one is
unusable and **must** fail review. Errors are announced through a
`visually-hidden` `role="alert"`, never through colour alone.

### 3.3 Link

Variants: `inline`, `nav`, `quiet`, `inverse`. States add `visited` and
`current` (`aria-current="page"`, so it is announced, not just styled).

A link **must** be an anchor: middle-clickable, openable in a new tab, and
announced as a link. Where a destination is genuinely unavailable, render a
`button` or plain text with an explanation — never a dead anchor, and never
`aria-disabled` on an element that still navigates.

Touch: `min-height` is at least `--control-height-sm` (28px); the target never
depends on the text height alone.

### 3.4 PostCard (density: 7 per page)

Anatomy: avatar → name/handle/time → content (clamped) → media (16:10) →
action row (reply, repost, like, share).

| State | Rule |
| :--- | :--- |
| hover | Elevation only (`--shadow-1`). Never a layout shift. |
| liked | `aria-pressed="true"`, filled heart, danger ink. The accessible name becomes "Unlike". |
| tombstone | Dashed border, "This post was removed by its author." The row stays so the thread's rhythm survives. |
| skeleton | `aria-hidden` + `aria-busy` on the container; three skeleton rows so loading never has zero layout. |
| media failure | `role="alert"` caption inside the figure; the frame keeps its ratio. |

Long content: the caption clamps to a measured line count via
`--clamp-lines`, and **Show more** is a real button that expands. Content
**must not** be hidden behind a gradient, and the action row **must not** move
when the card expands.

### 3.5 Navigation (density: 5)

Five primitives: side rail, in-content tab set, mobile rail, skip link,
breadcrumb (unused until a page needs it — do not build it speculatively).

`SideNav` uses **roving tabindex**: the list is one tab stop, arrow keys move
within it, Home/End jump. Without this, a keyboard user tabs through every
destination before reaching the content.

`TabSet` is an ARIA tabs pattern: arrow keys move *selection* (not just focus),
only the selected tab is in the tab order, and disabled tabs are skipped with a
`disabledReason` announced through `aria-describedby`.

`MobileNav` scrolls horizontally rather than wrapping and reports
`data-overflowing` when it does, so the strip is never silently clipped.

### 3.6 Inputs and search (density: 3)

`TextField` always renders label + description + error together, so the
accessible name and description exist without page-level ARIA wiring. A loading
field sets `aria-busy`; an invalid one sets `aria-invalid` **and** a
`role="alert"` message.

`SearchField` is a combobox: the input owns the value, results are a listbox,
and arrow keys move through them. Escape clears **and** returns focus. Copy
stays descriptive — "Search posts and people", never "Search…".

`Composer` grows to a measured maximum then scrolls; the counter is always
visible past 80% of the limit and states the remaining count; over-limit blocks
submission **with an explanation**, not with a disabled button and no reason.

### 3.7 List (density: 1)

The app's one list primitive owns all four states:

| State | Rule |
| :--- | :--- |
| loading | Skeleton rows (`renderSkeleton`), `aria-busy`, `role="status"`. |
| empty | **Must** say why it is empty and what to do next. |
| error | What failed **and** a retry control. |
| ready | Windowed: visible rows + overscan of 4. |

Long content: only `ceil(viewport / 220px) + 4` rows are mounted, so a
20,000-item feed does not allocate 20,000 nodes. Keyboard: Home/End jump to the
ends, Page Up/Page Down move by page, and each row keeps its own focusable
actions — the list is a container, never a focus trap.

---

## 4. Accessibility requirements and acceptance criteria

Target WCAG 2.2 AA. Every rule below is a **pass/fail check** that can be run
against the built app.

| # | Requirement | Test / pass criterion |
| :--- | :--- | :--- |
| A1 | Focus is always visible | Keyboard-traverse the page: every focusable element shows a 2px ring. **Fail** if any focus lands invisibly. |
| A2 | Focus ring offset | Ring offset is `--focus-ring-offset` (2px) and never overlaps the control's own border. |
| A3 | Skip link works | First Tab press focuses "Skip to main content"; activating it moves focus to `#main`. |
| A4 | Every control has an accessible name | Run the a11y tree: no button/link/field without a name. IconButtons **fail** this by construction when `label` is omitted. |
| A5 | Body-text contrast ≥ 4.5:1 | Measure each text/background pair at the computed size. `--color-text-tertiary` **fails** below 24px. |
| A6 | Non-text contrast ≥ 3:1 | Borders, indicator, focus ring, and the tab indicator against their surface. |
| A7 | No colour-only signalling | Like state, error state, and unread state each carry text or an ARIA attribute, not only hue. |
| A8 | Reduced motion honoured | With `prefers-reduced-motion: reduce`, GSAP animations are skipped and elements sit at their final state; nothing is invisible. |
| A9 | Touch targets ≥ 24px | Every interactive element's hit area is at least 24×24 CSS px. |
| A10 | Keyboard-only completion | Create a post, like a post, switch a tab, and open a conversation using the keyboard alone. |
| A11 | Pointer-only sanity | No hover-only affordance conveys information: hover content is duplicated in the accessible name or description. |
| A12 | Touch behaviour | Under `pointer: coarse`, no `:active` residue and no hover-only shadow. |
| A13 | Forced colours | With `forced-colors: active`, borders and the indicator use `CanvasText`; nothing is hidden behind a replaced background. |
| A14 | Live regions | Loading, the demo banner, the composer counter, and error messages announce; a validation error appears on blur, not only on submit. |
| A15 | Roving tabindex | In the side nav and the tab set, exactly one item is in the tab order per container. |

Verification performed for this build: the accessibility tree of the running app
was captured and shows the skip link first, all five nav destinations with
badges announced ("Messages 3"), the tab set with a selected tab, five articles
each labelled by author, and like buttons exposing both the count and the state
("342 Unlike" `[pressed]`).

---

## 5. Content and tone

Concise, confident, implementation-focused. No exclamation marks, no hedging,
no "oops".

| Do | Don't |
| :--- | :--- |
| "Post" | "Share your thoughts!!!" |
| "No matches for “golang”" + a next step | "Nothing found" |
| "Sign in to publish a post." | "You can't do that" |
| "The gateway returned 404. Showing demo posts instead." | "Something went wrong" |
| "Message deleted" | "This message has been removed by its author at 14:02" |

Rules: sentence case for buttons and headings; sentence case for all labels;
numbers use the compact form (1.3K, 4.5M); relative time in words ("10 months
ago"), never a raw timestamp; every empty state names a next action.

---

## 6. Anti-patterns and prohibited implementations

1. **A raw hex, px spacing, or font size in a component.** `components.css` and
   every `.tsx` **must** resolve to a token. Enforced by grep in review.
2. **A one-off spacing or typography exception** instead of a new token.
3. **`focus { outline: none }` without `:focus-visible`** — the single most
   common way to make a keyboard unusable.
4. **An icon button without an accessible name.**
5. **A link that is really an action**, or `aria-disabled` on something that
   still navigates.
6. **`:focus-visible` styling only on `:focus`** (rings on mouse clicks) or only
   on some components.
7. **A spinner that replaces the label** and makes the button jump.
8. **A disabled button with no reason given to anyone** — sighted users get only
   grey, and assistive tech gets nothing.
9. **A gradient mask over truncated text** instead of a "show more" control.
10. **Animating layout properties** (`width`, `height`, `top`) instead of
    `transform`/`opacity`.
11. **`GetOperationContext`-style nil checks on panicking getters** — the
    backend already carried two of these; the frontend has no equivalent.
12. **A toast or banner as the only error surface** — errors must also be
    attached to the control that failed.
13. **`visibility: hidden` or `display: none` on a "disabled" nav item** — use
    the `disabled` attribute plus a `disabledReason`.
14. **Marking a notification read silently** — optimistic updates must revert
    with a reason.

---

## 7. Migration notes

- New screens **must** import from `styles/tokens.css` and the components in
  `src/components/`; no new local CSS files without a token.
- Where an existing screen uses a one-off value, replace it in the same change
  that touches the file — do not batch them.
- GSAP usage belongs in `src/lib/motion.ts`. A component that calls `gsap.*`
  directly has bypassed the reduced-motion guard.
- Palette changes happen in `tokens.css` only, and require re-measuring the
  contrast table in §2.2.

---

## 8. QA checklist

Run before every merge. The `#/states` route shows each component in every
state, so most of this is inspectable in one screen.

**Tokens and layout**
- [ ] No raw hex, raw px spacing, or raw font size outside `tokens.css`.
- [ ] Spacing comes from the scale or a composite token, never a sum.
- [ ] Layout at 360 / 768 / 1024 / 1600 px: no horizontal scroll, no clipped
      control, no wrapped label where it should truncate.

**States**
- [ ] Every component on the page shows all seven states on `#/states`.
- [ ] Loading never has zero layout; a skeleton replaces the content, not the
      space.
- [ ] Empty states explain why and name the next action.
- [ ] Error states carry a retry or a correction, and say what failed.

**Accessibility**
- [ ] A1–A15 in §4 all pass.
- [ ] Tab order matches reading order and the skip link is first.
- [ ] No information is conveyed by colour alone.
- [ ] `prefers-reduced-motion` leaves nothing invisible.

**Interaction**
- [ ] Keyboard-only: post, like, switch tab, open a thread.
- [ ] Pointer: no hover-only information, no `:active` residue under touch.
- [ ] Optimistic actions revert with an explanation on failure.

**Data and edge cases**
- [ ] A caption of 10,000 characters clamps and expands.
- [ ] A feed of 0 items, 1 item, and 500 items renders.
- [ ] A gateway failure shows the demo banner, never a blank screen.
- [ ] A gateway 401 clears the session and renders the signed-out state.

**Build**
- [ ] `npm run build` passes (type-check + production bundle).
- [ ] No `console.error` in the browser other than the expected
      gateway-unreachable case.

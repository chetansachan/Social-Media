import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { revealStagger } from "../lib/motion";

/* ==========================================================================
   FeedList — the app's one list primitive.
   --------------------------------------------------------------------------
   Density: 1 list per page, but every screen reuses it, so it owns all four
   states a list can be in:
     loading (skeleton rows, never a spinner with no layout) |
     empty   (explains why, and what to do) |
     error   (what failed, and a retry) |
     ready   (windowed, keyboard-reachable)

   Long-content handling: only the visible window plus an overscan is rendered,
   so a 20,000-item feed does not allocate 20,000 DOM nodes. The overscan
   absorbs the row-height estimate being wrong, which would otherwise flash
   blank rows during a fast scroll.

   Keyboard: Home/End jump to the ends, Page Up/Page Down move by page. Each
   row keeps its own focusable actions, so the list is a container for roving
   rather than a trap.
   ========================================================================== */

export interface FeedListProps<T> {
  items: T[];
  renderItem: (item: T, index: number) => ReactNode;
  /** Stable key for each row; must be unique per item. */
  keyFor: (item: T, index: number) => string;
  /** Rendered instead of the rows while loading. */
  renderSkeleton?: () => ReactNode;
  loading?: boolean;
  error?: string;
  onRetry?: () => void;
  /** Copy for the empty state. Must say why it is empty. */
  emptyTitle: string;
  emptyBody?: string;
  emptyAction?: ReactNode;
  skeletonRows?: number;
  ariaLabel: string;
}

const OVERSCAN = 4;
const ROW_ESTIMATE = 220;
const PAGE_STEP = 3;

export function FeedList<T>({
  items,
  renderItem,
  keyFor,
  renderSkeleton,
  loading = false,
  error,
  onRetry,
  emptyTitle,
  emptyBody,
  emptyAction,
  skeletonRows = 3,
  ariaLabel,
}: FeedListProps<T>) {
  const listRef = useRef<HTMLUListElement>(null);
  const [visible, setVisible] = useState(() => Math.min(items.length, PAGE_STEP + OVERSCAN));

  // Windowed rendering: measure the viewport, render what it can hold plus the
  // overscan. The row height is an estimate because cards vary with media.
  useEffect(() => {
    if (loading || error) return;

    const update = () => {
      const rows = Math.ceil(window.innerHeight / ROW_ESTIMATE) + OVERSCAN;
      setVisible(Math.max(1, Math.min(items.length, rows)));
    };

    update();
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, [items.length, loading, error]);

  // Entrance choreography runs once per batch, after the rows exist in the DOM.
  useEffect(() => {
    const list = listRef.current;
    if (!list || loading || error) return;
    revealStagger(list, Array.from(list.querySelectorAll("[data-row]")));
  }, [loading, error, items.length]);

  if (loading) {
    return (
      <div className="feed feed--loading" role="status" aria-live="polite" aria-busy="true">
        <span className="visually-hidden">Loading {ariaLabel}</span>
        <ul className="feed__list" aria-label={ariaLabel}>
          {Array.from({ length: skeletonRows }, (_, index) => (
            <li key={index} data-row>
              {renderSkeleton?.() ?? null}
            </li>
          ))}
        </ul>
      </div>
    );
  }

  if (error) {
    return (
      <div className="feed feed--error" role="alert">
        <h2 className="feed__title">{emptyTitle}</h2>
        <p className="feed__body">{error}</p>
        {onRetry && (
          <button type="button" className="btn btn--secondary btn--md" onClick={onRetry}>
            Try again
          </button>
        )}
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="feed feed--empty">
        <h2 className="feed__title">{emptyTitle}</h2>
        {emptyBody && <p className="feed__body">{emptyBody}</p>}
        {emptyAction}
      </div>
    );
  }

  const page = items.slice(0, visible);
  const onListKeys = (event: React.KeyboardEvent<HTMLUListElement>) => {
    const rows = Array.from(listRef.current?.querySelectorAll<HTMLElement>("[data-row]") ?? []);
    if (rows.length === 0) return;
    const current = rows.findIndex((row) => row.contains(document.activeElement));
    if (current === -1) return;

    let next = current;
    switch (event.key) {
      case "PageDown":
        next = Math.min(rows.length - 1, current + PAGE_STEP);
        break;
      case "PageUp":
        next = Math.max(0, current - PAGE_STEP);
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = rows.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    rows[next].querySelector<HTMLElement>("a, button, input, [tabindex]")?.focus();
  };

  return (
    <div className="feed">
      <ul className="feed__list" aria-label={ariaLabel} ref={listRef} onKeyDown={onListKeys}>
        {page.map((item, index) => (
          <li key={keyFor(item, index)} data-row>
            {renderItem(item, index)}
          </li>
        ))}
      </ul>
      {items.length > page.length && (
        <p className="feed__truncated">
          Showing the first {page.length} of {items.length}. Narrow the query to see the rest.
        </p>
      )}
    </div>
  );
}

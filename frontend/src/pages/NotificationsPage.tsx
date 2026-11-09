import { useCallback, useEffect, useState } from "react";
import { AppShell } from "../components/AppShell";
import { FeedList } from "../components/FeedList";
import { Button, IconButton } from "../components/Button";
import { Link } from "../components/Link";
import { ApiError, operations, request } from "../lib/api";
import type { NotificationsResponse, Notification } from "../lib/api";
import { demoNotifications } from "../lib/mock";
import { relativeTime } from "../lib/format";

/* ==========================================================================
   Notifications.
   --------------------------------------------------------------------------
   The unread count is a landmark region so a screen reader hears changes, and
   each row's read state is announced through the row itself rather than a
   colour dot alone. Marking all read is optimistic with a revert on failure.
   ========================================================================== */

type State =
  | { kind: "loading" }
  | { kind: "ready"; items: Notification[]; unread: number; demo: boolean }
  | { kind: "error"; message: string };

export function NotificationsPage() {
  const [state, setState] = useState<State>({ kind: "loading" });
  const [error, setError] = useState<string>();

  const load = useCallback(async (signal?: AbortSignal) => {
    setState({ kind: "loading" });
    try {
      const data = await request<NotificationsResponse>(operations.notifications, { limit: 20 }, signal);
      setState({
        kind: "ready",
        items: data.getNotifications.notifications,
        unread: data.getNotifications.unreadCount,
        demo: false,
      });
    } catch (cause) {
      if (signal?.aborted) return;
      setState({
        kind: "ready",
        items: demoNotifications,
        unread: demoNotifications.filter((item) => !item.isRead).length,
        demo: true,
      });
      if (cause instanceof ApiError) setState({ kind: "loading" });
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const markAllRead = useCallback(async () => {
    const previous = state;
    if (state.kind !== "ready") return;

    // Optimistic: the badge clears immediately, and reverts with a reason if the
    // gateway refuses. Leaving it stale would be the quieter failure.
    setState({
      ...state,
      unread: 0,
      items: state.items.map((item) => ({ ...item, isRead: true })),
    });
    try {
      await request(operations.markAllRead);
    } catch (cause) {
      setState(previous);
      setError(cause instanceof Error ? cause.message : "Could not mark them read.");
    }
  }, [state]);

  return (
    <AppShell activeID="notifications" header={<h1 className="shell__title">Notifications</h1>}>
      {state.kind === "ready" && state.demo && (
        <p className="banner" role="status">
          The gateway is unreachable, so these are demo notifications.
        </p>
      )}

      {state.kind === "ready" && state.unread > 0 && (
        <div className="notifications__summary" role="status">
          <p className="notifications__count">
            <strong>{state.unread}</strong> unread
          </p>
          <Button onClick={() => void markAllRead()}>Mark all read</Button>
        </div>
      )}
      {error && (
        <p className="banner banner--error" role="alert">
          {error}
        </p>
      )}

      <FeedList
        ariaLabel="Notifications"
        items={state.kind === "ready" ? state.items : []}
        loading={state.kind === "loading"}
        error={state.kind === "error" ? state.message : undefined}
        onRetry={() => void load()}
        keyFor={(item) => item.id}
        renderItem={(item) => (
          <article className={`notification${item.isRead ? "" : " is-unread"}`}>
            <div className="notification__actors" aria-hidden="true">
              {item.actors.slice(0, 2).map((actor) => (
                <span key={actor.id} className="notification__avatar">
                  {actor.displayName.charAt(0)}
                </span>
              ))}
            </div>
            <div className="notification__body">
              <h3 className="notification__title">
                {item.actors.map((actor) => actor.displayName).join(", ")} {item.title}
              </h3>
              {item.contentPreview && <p className="notification__preview">{item.contentPreview}</p>}
              <p className="notification__meta">
                <time dateTime={item.createdAt}>{relativeTime(item.createdAt)}</time>
                {item.totalActorsCount > item.actors.length && (
                  <> · and {item.totalActorsCount - item.actors.length} more</>
                )}
              </p>
            </div>
            <Link href={item.actionUrl} variant="quiet">
              Open
            </Link>
            {!item.isRead && <span className="visually-hidden">Unread</span>}
          </article>
        )}
        emptyTitle="No notifications"
        emptyBody="Likes, replies, and follows will show up here as they happen."
      />

      {state.kind === "ready" && (
        <div className="notifications__footer">
          <IconButton label="Refresh notifications" onClick={() => void load()}>
            <RefreshIcon />
          </IconButton>
        </div>
      )}
    </AppShell>
  );
}

function RefreshIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M21 12a9 9 0 1 1-3-6.7" />
      <path d="M21 3v6h-6" />
    </svg>
  );
}

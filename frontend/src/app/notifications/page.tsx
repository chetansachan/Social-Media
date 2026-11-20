"use client";

import * as React from "react";
import Link from "next/link";
import {
  AtSign,
  Bell,
  CheckCheck,
  Heart,
  Megaphone,
  MessageCircle,
  ShieldAlert,
  UserPlus,
} from "lucide-react";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";
import { relativeTime } from "@/lib/format";
import {
  useMarkAllRead,
  useMarkRead,
  useNotifications,
} from "@/lib/hooks/use-notifications";
import { useUnreadCount } from "@/lib/hooks/use-unread";
import { notify } from "@/lib/toast";
import { useErrorToast } from "@/lib/hooks/use-error-toast";
import { useSession } from "@/lib/providers/session-provider";
import type { Notification, NotificationType } from "@/lib/types";

const iconFor: Record<NotificationType, React.ElementType> = {
  POST_LIKE: Heart,
  COMMENT: MessageCircle,
  COMMENT_REPLY: MessageCircle,
  FOLLOW: UserPlus,
  MENTION: AtSign,
  CREATOR_POST: Megaphone,
  SYSTEM_ALERT: ShieldAlert,
};

const toneFor: Record<NotificationType, string> = {
  POST_LIKE: "text-danger",
  COMMENT: "text-accent",
  COMMENT_REPLY: "text-accent",
  FOLLOW: "text-success",
  MENTION: "text-accent-alt",
  CREATOR_POST: "text-accent-alt",
  SYSTEM_ALERT: "text-danger",
};

/**
 * Notifications carry deep links in the app's own scheme
 * (`socialflow://posts/<id>`) because they are written for mobile too. The web
 * app maps the ones it can resolve and falls back to the actor's profile.
 */
function webPath(item: Notification): string {
  if (item.actionUrl.startsWith("/")) return item.actionUrl;

  const post = /^socialflow:\/\/posts?\/(.+)$/.exec(item.actionUrl);
  if (post?.[1]) return `/post/${post[1]}`;

  const actor = item.actors[0];
  if (item.type === "FOLLOW" && actor) return `/profile/${actor.username}`;

  const user = /^socialflow:\/\/users?\/(.+)$/.exec(item.actionUrl);
  if (user?.[1] && actor) return `/profile/${actor.username}`;

  return "/notifications";
}

type Filter = "all" | "MENTION" | "POST_LIKE" | "FOLLOW";

const TABS: { value: Filter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "MENTION", label: "Mentions" },
  { value: "POST_LIKE", label: "Likes" },
  { value: "FOLLOW", label: "Follows" },
];

export default function NotificationsPage() {
  const [filter, setFilter] = React.useState<Filter>("all");
  const { status } = useSession();
  const { items, isLoading, error, refetch } = useNotifications(
    filter === "all" ? null : filter,
  );
  const totalUnread = useUnreadCount();
  const markAll = useMarkAllRead();
  const markRead = useMarkRead();
  useErrorToast(error, "Notifications unavailable");

  if (status === "anonymous") {
    return (
      <>
        <div className="px-8 py-16 text-center">
          <Bell className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
          <h2 className="mt-3 font-display text-3xl text-primary">
            Sign in for notifications
          </h2>
          <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">
            Likes, replies, mentions, and follows are tied to your account.
          </p>
          <Button asChild size="md" className="mt-6">
            <Link href="/login">Sign in</Link>
          </Button>
        </div>
      </>
    );
  }

  return (
    <>
      <div className="sticky top-0 z-[calc(var(--z-sticky)-1)] bg-page/90 backdrop-blur-xl">
        <Tabs value={filter} onValueChange={(value) => setFilter(value as Filter)}>
          <TabsList aria-label="Notification filter">
            {TABS.map((tab) => (
              <TabsTrigger key={tab.value} value={tab.value}>
                {tab.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>

      <div aria-live="polite" className="sr-only">
        {totalUnread > 0 ? `${totalUnread} unread notifications` : "No unread notifications"}
      </div>

      <div className="flex items-center justify-between border-b border-line px-4 py-3">
        <p className="font-mono text-2xs text-tertiary">
          {items.length} {items.length === 1 ? "notification" : "notifications"}
        </p>
        <Button
          variant="ghost"
          size="sm"
          onClick={() =>
            markAll.mutate(undefined, {
              onSuccess: () => notify.success("All notifications marked read"),
              onError: (error) =>
                notify.error("Could not mark notifications read", error.message),
            })
          }
          disabled={totalUnread === 0 || markAll.isPending}
          className="gap-1.5"
        >
          <CheckCheck className="size-icon-xs" aria-hidden="true" />
          Mark all read
        </Button>
      </div>

      {isLoading ? (
        <div aria-hidden="true">
          {[0, 1, 2, 3].map((index) => (
            <div key={index} className="flex gap-3 border-b border-line px-4 py-4">
              <div className="sf-skeleton size-10 rounded-full" />
              <div className="flex-1 space-y-2">
                <div className="sf-skeleton h-3 w-2/3 rounded" />
                <div className="sf-skeleton h-3 w-1/3 rounded" />
              </div>
            </div>
          ))}
        </div>
      ) : error ? (
        <div className="px-8 py-16 text-center">
          <Bell className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
          <h2 className="mt-3 font-display text-2xl text-primary">
            Notifications are unavailable
          </h2>
          <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">{error.message}</p>
          <Button variant="secondary" size="sm" className="mt-5" onClick={() => refetch()}>
            Try again
          </Button>
        </div>
      ) : items.length === 0 ? (
        <div className="px-8 py-16 text-center">
          <Bell className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
          <h2 className="mt-3 font-display text-3xl text-primary">
            {filter === "all" ? "Nothing here yet" : "Nothing in this filter"}
          </h2>
          <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">
            Likes, replies, mentions, and follows land here.
          </p>
        </div>
      ) : (
        <ul>
          {items.map((item) => {
            const Icon = iconFor[item.type];
            const actor = item.actors[0];

            return (
              <li key={item.id}>
                <Link
                  href={webPath(item)}
                  onClick={() => {
                    if (!item.isRead) {
                      markRead.mutate([item.id], {
                        onError: (error) =>
                          notify.error("Could not mark notification read", error.message),
                      });
                    }
                  }}
                  className={cn(
                    "flex items-start gap-3 border-b border-line px-4 py-4 transition-colors",
                    "hover:bg-hover focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
                    !item.isRead && "bg-accent-soft/40",
                  )}
                >
                  <span className="relative shrink-0">
                    <Avatar
                      name={actor?.displayName ?? "Someone"}
                      handle={actor?.username ?? "unknown"}
                      src={actor?.avatarUrl || undefined}
                      size="md"
                    />
                    <span
                      aria-hidden="true"
                      className={cn(
                        "absolute -bottom-1 -right-1 grid size-6 place-items-center rounded-full bg-page",
                        toneFor[item.type],
                      )}
                    >
                      <Icon className="size-3.5" fill="currentColor" />
                    </span>
                  </span>

                  <div className="min-w-0 flex-1">
                    <p
                      className={cn(
                        "text-base leading-snug text-primary",
                        !item.isRead && "font-medium",
                      )}
                    >
                      {item.title}
                    </p>

                    {item.contentPreview ? (
                      <p className="mt-1 clamp-2 text-sm text-tertiary">
                        {item.contentPreview}
                      </p>
                    ) : null}

                    <time
                      dateTime={item.createdAt}
                      className="mt-1 block font-mono text-xs text-tertiary"
                    >
                      {relativeTime(item.createdAt)}
                    </time>
                  </div>

                  {!item.isRead ? (
                    <span
                      aria-label="Unread"
                      className="mt-2 size-2 shrink-0 rounded-full bg-accent-solid"
                    />
                  ) : null}
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </>
  );
}
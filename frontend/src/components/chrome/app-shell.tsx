"use client";

import * as React from "react";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { SideRail, IconRail, BottomNav } from "./nav";
import { TrendingRail } from "./trending-rail";
import { SkipLink } from "./skip-link";
import { PostButton } from "@/components/feed/compose-dialog";
import { useUnreadCount } from "@/lib/hooks/use-unread";
import { useConversations } from "@/lib/hooks/use-chat";
import { useSession } from "@/lib/providers/session-provider";

/**
 * The shell owns the responsive collapse.
 *
 *   ≥1280  labelled rail 250px | feed 600px | aside 350px
 *   1024–  icon rail 88px      | feed 600px | aside 300px
 *   768–   icon rail 88px      | feed flexible, aside dropped
 *   ≤767   bottom bar, full-width feed
 *
 * The shell clips on the x axis and every flexible column is min-w-0, so a
 * long handle or a wide image never creates horizontal scroll.
 */
export function AppShell({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  const pathname = usePathname();
  const { status } = useSession();
  const notificationsUnread = useUnreadCount();
  // The inbox badge. The query is shared with the messages screen, so
  // mounting it here costs one cached read, not a second request.
  const inbox = useConversations();
  const badge = notificationsUnread;
  const messagesBadge = inbox.totalUnread;
  const isMessages = Boolean(pathname?.startsWith("/messages"));
  const wide = isMessages;
  const showAside =
    !isMessages &&
    (pathname === "/" ||
      Boolean(pathname?.startsWith("/explore")) ||
      Boolean(pathname?.startsWith("/notifications")) ||
      Boolean(pathname?.startsWith("/profile")) ||
      Boolean(pathname?.startsWith("/post")));
  const showComposeFab = status === "authenticated" && !isMessages;

  return (
    <div className="min-h-dvh overflow-x-clip bg-page">
      <SkipLink />

      <div className="mx-auto flex w-full max-w-[1260px] justify-center">
        {/* Labelled rail */}
        <div className="sticky top-0 hidden h-dvh w-[var(--layout-rail)] shrink-0 xl:block">
          <SideRail unread={badge} messagesUnread={messagesBadge} />
        </div>

        {/* Icon rail */}
        <div className="sticky top-0 hidden h-dvh w-[var(--layout-rail-icon)] shrink-0 md:block xl:hidden">
          <IconRail unread={badge} messagesUnread={messagesBadge} />
        </div>

        {/* Centre column */}
        <div
          className={cn(
            "flex min-w-0 flex-1 flex-col border-x border-line",
            wide
              ? "max-w-none lg:max-w-[1000px]"
              : "max-w-[var(--layout-feed-max)]",
            className,
          )}
        >
          <main
            id="main"
            className={cn(
              "min-w-0 flex-1",
              !isMessages &&
                "pb-[calc(var(--layout-bottom-bar)+env(safe-area-inset-bottom,0px))] md:pb-0",
            )}
          >
            {children}
          </main>
        </div>

        {/* Right rail — dropped first at ≤1023, because it is decorative. */}
        {showAside ? (
          <aside className="sticky top-0 hidden h-dvh w-[var(--layout-aside-narrow)] shrink-0 overflow-y-auto lg:block xl:w-[var(--layout-aside)]">
            <div className="px-6 py-2">
              <TrendingRail showSearch={!pathname?.startsWith("/explore")} />
            </div>
          </aside>
        ) : null}
      </div>

      <div className="md:hidden">
        <BottomNav unread={badge} messagesUnread={messagesBadge} />
        {showComposeFab ? (
          <div className="fixed bottom-[calc(var(--layout-bottom-bar)+16px+env(safe-area-inset-bottom,0px))] right-4 z-[var(--z-sticky)]">
            <PostButton compact className="size-[var(--control-height-xl)] shadow-glow" />
          </div>
        ) : null}
      </div>
    </div>
  );
}
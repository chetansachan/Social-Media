"use client";

import * as React from "react";
import Link from "next/link";
import { MailOpen, Users } from "lucide-react";
import { cn } from "@/lib/utils";
import { relativeTime } from "@/lib/format";
import { conversationDisplay, conversationPreview, otherParticipant } from "@/lib/chat";
import { useConversations } from "@/lib/hooks/use-chat";
import { usePresence } from "@/lib/hooks/use-presence";
import { useSession } from "@/lib/providers/session-provider";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { NewMessageDialog } from "./new-message-dialog";

/**
 * The conversation rail.
 *
 * On desktop it sits beside the thread; on phones it is the whole inbox screen.
 * Rows show the other participant for a direct thread, a group chip for group
 * threads, the real last message, unread count, and an online dot from
 * `getPresence`.
 */
export function ConversationList({
  activeId,
  className,
}: {
  activeId?: string;
  className?: string;
}) {
  const { user } = useSession();
  const { items, totalUnread, isLoading, error, refetch } = useConversations();

  const presenceIds = React.useMemo(
    () =>
      items
        .map((conversation) => otherParticipant(conversation, user?.id)?.userId)
        .filter((id): id is string => Boolean(id)),
    [items, user?.id],
  );
  const { isOnline } = usePresence(presenceIds);

  return (
    <div className={cn("flex h-full min-h-0 flex-col bg-page", className)}>
      <header className="flex h-[60px] shrink-0 items-center justify-between gap-3 border-b border-line px-4">
        <div className="min-w-0">
          <h1 className="font-sans text-pane font-semibold text-primary">Messages</h1>
          <p className="truncate font-mono text-2xs text-tertiary">
            {totalUnread > 0 ? `${totalUnread} unread` : "All caught up"}
          </p>
        </div>
        <NewMessageDialog />
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isLoading ? (
          <ul aria-hidden="true">
            {[0, 1, 2, 3].map((index) => (
              <li key={index} className="flex gap-3 border-b border-line px-4 py-3">
                <div className="sf-skeleton size-10 shrink-0 rounded-full" />
                <div className="flex-1 space-y-2">
                  <div className="sf-skeleton h-3 w-1/3 rounded" />
                  <div className="sf-skeleton h-3 w-2/3 rounded" />
                </div>
              </li>
            ))}
          </ul>
        ) : error ? (
          <div className="px-6 py-14 text-center">
            <p className="font-sans text-base font-semibold text-primary">
              Messages are unavailable
            </p>
            <p className="mt-1 text-sm text-secondary">{error.message}</p>
            <Button variant="secondary" size="sm" className="mt-4" onClick={() => refetch()}>
              Try again
            </Button>
          </div>
        ) : items.length === 0 ? (
          <div className="px-6 py-16 text-center">
            <span className="mx-auto grid size-12 place-items-center rounded-full bg-accent-soft text-accent">
              <MailOpen className="size-icon-md" aria-hidden="true" />
            </span>
            <h2 className="mt-3 font-sans text-lg font-semibold text-primary">
              No messages yet
            </h2>
            <p className="mt-1 text-sm leading-relaxed text-secondary">
              Open someone&rsquo;s profile and press Message to start a direct
              conversation.
            </p>
            <Button asChild variant="secondary" size="sm" className="mt-4">
              <Link href="/explore">Find people</Link>
            </Button>
          </div>
        ) : (
          <ul>
            {items.map((conversation) => {
              const { name, handle } = conversationDisplay(conversation, user?.id);
              const other = otherParticipant(conversation, user?.id);
              const online = isOnline(other?.userId);
              const active = conversation.id === activeId;
              const preview = conversationPreview(conversation);
              const mine = conversation.lastMessage?.senderId === user?.id;

              return (
                <li key={conversation.id}>
                  <Link
                    href={`/messages/${conversation.id}`}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "relative flex items-start gap-3 border-b border-line px-4 py-3 transition-colors",
                      "hover:bg-hover focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
                      active && "bg-hover",
                      !active && conversation.unreadCount > 0 && "bg-accent-soft/25",
                    )}
                  >
                    {active ? (
                      <span
                        aria-hidden="true"
                        className="absolute inset-y-0 left-0 w-0.5 bg-accent"
                      />
                    ) : null}

                    <span className="relative shrink-0">
                      <Avatar
                        name={name}
                        handle={handle}
                        size="md"
                        className={cn(
                          conversation.type === "GROUP" && "rounded-lg",
                        )}
                      />
                      {online ? (
                        <span
                          aria-hidden="true"
                          className="absolute -bottom-0.5 -right-0.5 size-3 rounded-full border-2 border-page bg-success"
                        />
                      ) : null}
                    </span>

                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-1.5">
                        <span className="truncate font-sans text-base font-semibold text-primary">
                          {name}
                        </span>
                        {conversation.type === "GROUP" ? (
                          <Users
                            className="size-3.5 shrink-0 text-tertiary"
                            aria-label="Group conversation"
                          />
                        ) : (
                          <span className="truncate font-mono text-xs text-tertiary">
                            @{handle}
                          </span>
                        )}
                        <time
                          dateTime={conversation.updatedAt}
                          className="ml-auto shrink-0 font-mono text-xs text-tertiary"
                        >
                          {relativeTime(conversation.updatedAt)}
                        </time>
                      </span>

                      <span className="mt-0.5 flex items-center gap-2">
                        <span
                          className={cn(
                            "min-w-0 flex-1 truncate text-sm",
                            conversation.lastMessage?.isDeleted
                              ? "italic text-tertiary"
                              : conversation.unreadCount > 0
                                ? "font-medium text-primary"
                                : "text-secondary",
                          )}
                        >
                          {mine && conversation.lastMessage ? "You: " : ""}
                          {preview}
                        </span>
                        {conversation.unreadCount > 0 ? (
                          <span className="grid min-w-5 shrink-0 place-items-center rounded-full bg-accent-solid px-1.5 font-mono text-2xs font-semibold leading-5 text-on-accent">
                            {conversation.unreadCount > 9 ? "9+" : conversation.unreadCount}
                            <span className="sr-only"> unread</span>
                          </span>
                        ) : null}
                      </span>
                    </span>
                  </Link>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}

"use client";

import Link from "next/link";
import { Mail, MessageSquare } from "lucide-react";
import { ConversationList } from "@/components/messages/conversation-list";
import { Button } from "@/components/ui/button";
import { useSession } from "@/lib/providers/session-provider";

/**
 * Messages — split pane on desktop, inbox on phones.
 *
 * The backend has no conversation-search endpoint, so the rail is a plain
 * recency list. Selecting a thread moves to /messages/[id]; on wide screens the
 * rail stays mounted beside it.
 */
export default function MessagesPage() {
  const { status } = useSession();

  if (status === "loading") {
    return (
      <>
        <div className="flex h-[calc(100dvh_-_var(--layout-bottom-bar)_-_env(safe-area-inset-bottom,0px))] min-h-0 md:h-dvh">
          <div className="w-full border-r border-line md:w-[360px]" aria-hidden="true">
            <div className="space-y-3 p-4">
              <div className="sf-skeleton h-6 w-28 rounded" />
              <div className="sf-skeleton h-14 w-full rounded-lg" />
              <div className="sf-skeleton h-14 w-full rounded-lg" />
            </div>
          </div>
          <div className="hidden flex-1 md:block" />
        </div>
      </>
    );
  }

  if (status === "anonymous") {
    return (
      <>
        <div className="px-8 py-20 text-center">
          <Mail className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
          <h1 className="mt-3 font-display text-3xl text-primary">
            Sign in to see your messages
          </h1>
          <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">
            Direct conversations live behind your session.
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
      <div className="flex h-[calc(100dvh_-_var(--layout-bottom-bar)_-_env(safe-area-inset-bottom,0px))] min-h-0 md:h-dvh">
        <div className="w-full min-w-0 border-r border-line md:w-[360px] md:shrink-0">
          <ConversationList />
        </div>

        <div className="hidden min-w-0 flex-1 items-center justify-center bg-page px-8 md:flex">
          <div className="max-w-xs text-center">
            <span className="mx-auto grid size-14 place-items-center rounded-full bg-accent-soft text-accent">
              <MessageSquare className="size-icon-lg" aria-hidden="true" />
            </span>
            <h2 className="mt-4 font-display text-3xl text-primary">
              Select a message
            </h2>
            <p className="mt-2 text-sm leading-relaxed text-secondary">
              Choose a conversation from the list, or find someone new and open a
              thread from their profile.
            </p>
            <Button asChild variant="secondary" size="sm" className="mt-5">
              <Link href="/explore">Find people</Link>
            </Button>
          </div>
        </div>
      </div>
    </>
  );
}

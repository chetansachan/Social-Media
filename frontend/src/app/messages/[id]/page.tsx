"use client";

import * as React from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft, Send, Users } from "lucide-react";
import { ConversationList } from "@/components/messages/conversation-list";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { absoluteDate, relativeTime } from "@/lib/format";
import {
  conversationDisplay,
  otherParticipant,
  presenceUserId,
} from "@/lib/chat";
import {
  useConversation,
  useMarkConversationRead,
  useMessages,
  useSendMessage,
} from "@/lib/hooks/use-chat";
import { usePresence } from "@/lib/hooks/use-presence";
import { useErrorToast } from "@/lib/hooks/use-error-toast";
import { useSession } from "@/lib/providers/session-provider";
import { notify } from "@/lib/toast";
import type { Message } from "@/lib/types";

const GROUP_WINDOW_MS = 5 * 60_000;

function dayKey(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;
}

function dayLabel(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  const today = new Date();
  const yesterday = new Date();
  yesterday.setDate(today.getDate() - 1);
  if (dayKey(iso) === dayKey(today.toISOString())) return "Today";
  if (dayKey(iso) === dayKey(yesterday.toISOString())) return "Yesterday";
  return absoluteDate(iso);
}

/**
 * One thread, with the conversation rail beside it on desktop.
 *
 * Messages are grouped by day and by sender. The newest message scrolls into
 * view without moving the page. Presence is polled from `getPresence`; the app
 * does not yet subscribe to chat events, so there is no fake "typing" state.
 */
export default function ThreadPage() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? "";
  const { user, status } = useSession();

  const conversation = useConversation(id);
  const messages = useMessages(id);
  useMarkConversationRead(id);
  const send = useSendMessage(id);
  useErrorToast(messages.error, "Conversation could not load");

  const other = conversation ? otherParticipant(conversation, user?.id) : undefined;
  const presence = usePresence(
    React.useMemo(
      () => [conversation ? presenceUserId(conversation, user?.id) : undefined].filter(
        (value): value is string => Boolean(value),
      ),
      [conversation, user?.id],
    ),
  );
  const online = presence.isOnline(other?.userId);
  const { name, handle } = conversationDisplay(conversation, user?.id);

  const [value, setValue] = React.useState("");
  const listRef = React.useRef<HTMLDivElement>(null);
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);

  React.useEffect(() => {
    const node = listRef.current;
    if (!node) return;
    node.scrollTop = node.scrollHeight;
  }, [messages.messages.length, id]);

  React.useEffect(() => {
    const node = textareaRef.current;
    if (!node) return;
    node.style.height = "auto";
    node.style.height = `${Math.min(node.scrollHeight, 132)}px`;
  }, [value]);

  if (status === "loading") {
    return (
      <>
        <div className="flex h-[calc(100dvh_-_var(--layout-bottom-bar)_-_env(safe-area-inset-bottom,0px))] min-h-0 md:h-dvh">
          <div className="hidden w-[360px] shrink-0 border-r border-line md:block" aria-hidden="true">
            <div className="space-y-3 p-4">
              <div className="sf-skeleton h-6 w-28 rounded" />
              <div className="sf-skeleton h-14 w-full rounded-lg" />
              <div className="sf-skeleton h-14 w-full rounded-lg" />
            </div>
          </div>
          <div className="flex-1 p-4" aria-hidden="true">
            <div className="sf-skeleton h-12 w-full rounded-lg" />
          </div>
        </div>
      </>
    );
  }

  if (status === "anonymous") {
    return (
      <>
        <div className="grid h-[calc(100dvh_-_var(--layout-bottom-bar)_-_env(safe-area-inset-bottom,0px))] place-items-center px-6 text-center md:h-dvh">
          <div>
            <h1 className="font-display text-3xl text-primary">
              Sign in to read this conversation
            </h1>
            <Button asChild size="md" className="mt-5">
              <Link href="/login">Sign in</Link>
            </Button>
          </div>
        </div>
      </>
    );
  }

  const submit = () => {
    const content = value.trim();
    if (!content || send.isPending) return;
    setValue("");
    send.mutate(content, {
      onError: (error) => notify.error("Message not sent", error.message),
    });
  };

  const isGroup = conversation?.type === "GROUP";

  return (
    <>
      <div className="flex h-[calc(100dvh_-_var(--layout-bottom-bar)_-_env(safe-area-inset-bottom,0px))] min-h-0 md:h-dvh">
        <div className="hidden w-[360px] shrink-0 border-r border-line md:block">
          <ConversationList activeId={id} />
        </div>

        <section className="flex min-w-0 flex-1 flex-col">
          <h1 className="sr-only">Conversation with {name}</h1>
          <header className="flex h-[60px] shrink-0 items-center gap-2.5 border-b border-line bg-page/90 px-3 backdrop-blur-xl md:px-4">
            <Button asChild variant="ghost" size="icon" className="md:hidden" aria-label="Back to messages">
              <Link href="/messages">
                <ArrowLeft className="size-icon-sm" aria-hidden="true" />
              </Link>
            </Button>

            {other ? (
              <Link
                href={`/profile/${other.user.username}`}
                className="shrink-0 rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                aria-label={`${other.user.name} profile`}
              >
                <span className="relative block">
                  <Avatar name={name} handle={handle} size="md" />
                  {online ? (
                    <span
                      aria-hidden="true"
                      className="absolute -bottom-0.5 -right-0.5 size-3 rounded-full border-2 border-page bg-success"
                    />
                  ) : null}
                </span>
              </Link>
            ) : (
              <span className="relative block shrink-0">
                <Avatar name={name} handle={handle} size="md" />
              </span>
            )}

            <div className="min-w-0 flex-1">
              <p className="truncate font-sans text-base font-semibold text-primary">{name}</p>
              <p className="flex items-center gap-1 truncate font-mono text-xs text-tertiary">
                {isGroup ? (
                  <>
                    <Users className="size-3" aria-hidden="true" />
                    {conversation?.participants.length ?? 0} members
                  </>
                ) : online ? (
                  <>
                    <span
                      aria-hidden="true"
                      className="size-1.5 rounded-full bg-success"
                    />
                    Active now
                  </>
                ) : (
                  `@${handle}`
                )}
              </p>
            </div>

            {other ? (
              <Button asChild variant="ghost" size="sm" className="hidden sm:inline-flex">
                <Link href={`/profile/${other.user.username}`}>View profile</Link>
              </Button>
            ) : null}
          </header>

          <div
            ref={listRef}
            role="log"
            aria-label={`Conversation with ${name}`}
            aria-live="polite"
            aria-relevant="additions"
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4"
          >
            {messages.isLoading ? (
              <div className="space-y-3" aria-hidden="true">
                <div className="sf-skeleton h-9 w-2/3 rounded-lg" />
                <div className="sf-skeleton ml-auto h-9 w-1/2 rounded-lg" />
                <div className="sf-skeleton h-9 w-3/5 rounded-lg" />
              </div>
            ) : messages.error ? (
              <div className="px-6 py-14 text-center">
                <p className="font-sans text-base font-semibold text-primary">
                  Conversation unavailable
                </p>
                <p className="mx-auto mt-1 max-w-xs text-sm text-secondary">
                  {messages.error.message}
                </p>
                <Button
                  variant="secondary"
                  size="sm"
                  className="mt-4"
                  onClick={() => messages.refetch()}
                >
                  Try again
                </Button>
              </div>
            ) : messages.messages.length === 0 ? (
              <div className="grid h-full place-items-center text-center">
                <div className="max-w-xs">
                  <span className="mx-auto grid size-14 place-items-center rounded-full bg-accent-soft text-accent">
                    <Send className="size-icon-md" aria-hidden="true" />
                  </span>
                  <h2 className="mt-4 font-display text-3xl text-primary">Say hello</h2>
                  <p className="mt-1.5 text-sm leading-relaxed text-secondary">
                    This is the beginning of your conversation with {name}.
                  </p>
                </div>
              </div>
            ) : (
              <MessageTranscript
                messages={messages.messages}
                isGroup={isGroup}
                viewerId={user?.id}
              />
            )}
          </div>

          <div className="shrink-0 border-t border-line bg-page px-3 py-3 md:px-4">
            <form
              className="flex items-end gap-2"
              onSubmit={(event) => {
                event.preventDefault();
                submit();
              }}
            >
              <label htmlFor="message" className="sr-only">
                Message {name}
              </label>
              <textarea
                id="message"
                ref={textareaRef}
                value={value}
                onChange={(event) => setValue(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault();
                    submit();
                  }
                }}
                rows={1}
                placeholder={`Message ${name}`}
                className="max-h-[132px] min-h-11 w-full resize-none rounded-lg border border-line bg-inset px-4 py-2.5 text-base text-primary placeholder:text-tertiary transition-colors focus-visible:border-accent focus-visible:bg-page focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring"
              />
              <Button
                type="submit"
                size="icon-lg"
                aria-label="Send message"
                disabled={!value.trim()}
                loading={send.isPending}
                className="shrink-0"
              >
                <Send className="size-icon-sm" aria-hidden="true" />
              </Button>
            </form>
            <p className="mt-1.5 hidden font-mono text-2xs text-tertiary sm:block">
              Enter to send · Shift + Enter for a new line
            </p>
            {send.error ? (
              <p role="alert" className="mt-1.5 text-xs text-danger">
                {send.error.message}
              </p>
            ) : null}
          </div>
        </section>
      </div>
    </>
  );
}

/**
 * Chronological transcript with day separators and sender grouping. The
 * grouping window keeps a burst of replies visually connected without merging
 * messages that arrived minutes apart.
 */
function MessageTranscript({
  messages,
  isGroup,
  viewerId,
}: {
  messages: Message[];
  isGroup: boolean;
  viewerId: string | undefined;
}) {
  return (
    <div>
      {messages.map((message, index) => {
        const previous = messages[index - 1];
        const next = messages[index + 1];
        const newDay = !previous || dayKey(previous.createdAt) !== dayKey(message.createdAt);
        const gapFromPrevious = previous
          ? new Date(message.createdAt).getTime() -
            new Date(previous.createdAt).getTime()
          : Number.POSITIVE_INFINITY;
        const newGroup =
          newDay ||
          !previous ||
          previous.senderId !== message.senderId ||
          gapFromPrevious > GROUP_WINDOW_MS;
        const gapToNext = next
          ? new Date(next.createdAt).getTime() - new Date(message.createdAt).getTime()
          : Number.POSITIVE_INFINITY;
        const groupEnd =
          !next ||
          next.senderId !== message.senderId ||
          dayKey(next.createdAt) !== dayKey(message.createdAt) ||
          gapToNext > GROUP_WINDOW_MS;
        const mine = message.senderId === viewerId;
        const senderName = message.sender?.name ?? "Member";
        const senderHandle = message.sender?.username ?? "member";

        return (
          <React.Fragment key={message.id}>
            {newDay ? (
              <div className="my-3 flex items-center gap-3" role="separator">
                <span aria-hidden="true" className="h-px flex-1 bg-line" />
                <span className="font-mono text-2xs text-tertiary">
                  {dayLabel(message.createdAt)}
                </span>
                <span aria-hidden="true" className="h-px flex-1 bg-line" />
              </div>
            ) : null}

            <div
              className={cn(
                "flex items-end gap-2",
                mine ? "justify-end" : "justify-start",
                newGroup ? "mt-3" : "mt-0.5",
              )}
            >
              {!mine ? (
                newGroup ? (
                  <Link
                    href={`/profile/${senderHandle}`}
                    className="shrink-0 rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
                    aria-label={`${senderName} profile`}
                  >
                    <Avatar name={senderName} handle={senderHandle} size="xs" />
                  </Link>
                ) : (
                  <span aria-hidden="true" className="w-6 shrink-0" />
                )
              ) : null}

              <div className={cn("flex max-w-[78%] flex-col", mine ? "items-end" : "items-start")}>
                {!mine && isGroup && newGroup ? (
                  <span className="mb-1 pl-1 font-sans text-xs font-semibold text-primary">
                    {senderName}
                  </span>
                ) : null}

                <div
                  className={cn(
                    "whitespace-pre-wrap break-words rounded-lg px-4 py-2.5 text-base leading-relaxed",
                    mine
                      ? "rounded-br-sm bg-accent-solid text-on-accent"
                      : "rounded-bl-sm bg-surface text-primary",
                    message.isDeleted && "italic text-tertiary",
                  )}
                >
                  {message.isDeleted ? "This message was deleted." : message.content}
                </div>

                {groupEnd ? (
                  <time
                    dateTime={message.createdAt}
                    className="mt-1 px-1 font-mono text-2xs text-tertiary"
                  >
                    {relativeTime(message.createdAt)}
                  </time>
                ) : null}
              </div>
            </div>
          </React.Fragment>
        );
      })}
    </div>
  );
}

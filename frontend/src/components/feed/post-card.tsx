"use client";

import * as React from "react";
import Link from "next/link";
import {
  Link2,
  MoreHorizontal,
  Trash2,
} from "lucide-react";
import { notify } from "@/lib/toast";
import { cn } from "@/lib/utils";
import { Avatar } from "@/components/ui/avatar";
import { MediaBand } from "./media-band";
import { PostActions } from "./post-actions";
import {
  absoluteDate,
  relativeTime,
  segmentTags,
} from "@/lib/format";
import { useDeletePost } from "@/lib/hooks/use-feed";
import { useSession } from "@/lib/providers/session-provider";
import type { FeedPost, Post } from "@/lib/types";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

/**
 * A post row.
 *
 * The content is rendered exactly as written. Hashtags are styled in place
 * rather than duplicated as chips, and long copy clamps to four lines behind a
 * real button carrying aria-expanded. The menu is available to keyboard users
 * as well as pointer hover.
 */
export function PostCard({
  post,
  onLike,
  likeError,
  priority = false,
  replyCount,
  className,
  variant = "feed",
  onDeleted,
}: {
  post: Post | FeedPost;
  onLike?: (post: Post | FeedPost, liked: boolean) => void;
  likeError?: string;
  priority?: boolean;
  replyCount?: number;
  className?: string;
  variant?: "feed" | "detail";
  onDeleted?: (postId: string) => void;
}) {
  const [expanded, setExpanded] = React.useState(false);
  const { user } = useSession();
  const deletePost = useDeletePost();
  const isOwn = Boolean(user && user.id === post.authorId);
  const isLong = post.content.length > 420;

  const copyLink = async () => {
    try {
      await navigator.clipboard.writeText(
        `${window.location.origin}/post/${post.id}`,
      );
      notify.success("Link copied to clipboard");
    } catch {
      notify.error("Could not copy the link");
    }
  };

  const handleDelete = () => {
    deletePost.mutate(post.id, {
      onSuccess: () => {
        notify.success("Post deleted");
        onDeleted?.(post.id);
      },
      onError: (error) => notify.error(error.message),
    });
  };

  return (
    <article
      className={cn(
        "group relative flex gap-3 px-4 transition-colors duration-200",
        variant === "feed"
          ? "border-b border-line py-3 hover:bg-hover/40"
          : "py-4",
        className,
      )}
    >
      <Link
        href={`/profile/${post.author.username}`}
        className="h-fit shrink-0 rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        aria-label={`${post.author.name} profile`}
      >
        <Avatar
          name={post.author.name}
          handle={post.author.username}
          size={variant === "detail" ? "lg" : "md"}
        />
      </Link>

      <div className="min-w-0 flex-1">
        <header className="flex items-center gap-1.5">
          <Link
            href={`/profile/${post.author.username}`}
            className="truncate font-sans text-base font-semibold text-primary hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            {post.author.name}
          </Link>
          <span className="truncate font-mono text-xs text-tertiary">
            @{post.author.username}
          </span>
          <span aria-hidden="true" className="text-tertiary">
            ·
          </span>
          <Link
            href={`/post/${post.id}`}
            className="shrink-0 font-mono text-xs text-tertiary transition-colors hover:text-secondary hover:underline"
            title={absoluteDate(post.createdAt)}
          >
            <time dateTime={post.createdAt}>{relativeTime(post.createdAt)}</time>
          </Link>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                aria-label={`Post menu for ${post.author.name}`}
                className={cn(
                  "ml-auto grid size-9 shrink-0 place-items-center rounded-full text-tertiary",
                  "transition-[background-color,color] hover:bg-accent-soft hover:text-accent",
                  "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                )}
              >
                <MoreHorizontal className="size-icon-sm" aria-hidden="true" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-[190px]">
              <DropdownMenuItem onSelect={() => void copyLink()}>
                <Link2 className="size-icon-xs" aria-hidden="true" />
                Copy link
              </DropdownMenuItem>
              {isOwn ? (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    onSelect={handleDelete}
                    className="text-danger data-[highlighted]:bg-danger/10 data-[highlighted]:text-danger"
                  >
                    <Trash2 className="size-icon-xs" aria-hidden="true" />
                    Delete post
                  </DropdownMenuItem>
                </>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        </header>

        <p
          id={`post-body-${post.id}`}
          className={cn(
            "mt-0.5 whitespace-pre-wrap break-words font-sans text-primary",
            variant === "detail"
              ? "text-xl leading-copy"
              : "text-copy leading-copy",
            variant === "feed" && isLong && !expanded && "clamp-4",
          )}
        >
          <TaggedText text={post.content} />
        </p>

        {isLong && variant === "feed" ? (
          <button
            type="button"
            onClick={() => setExpanded((value) => !value)}
            aria-expanded={expanded}
            aria-controls={`post-body-${post.id}`}
            className="mt-1 rounded text-sm font-medium text-accent transition-colors hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            {expanded ? "Show less" : "Show more"}
          </button>
        ) : null}

        {post.mediaUrl ? (
          <MediaBand
            src={post.mediaUrl}
            alt={`Media attached to ${post.author.name}'s post`}
            priority={priority}
          />
        ) : null}

        <PostActions
          postId={post.id}
          liked={post.hasLiked}
          likeCount={post.likeCount}
          replyCount={replyCount}
          onLike={() => onLike?.(post, !post.hasLiked)}
          error={likeError}
        />
      </div>
    </article>
  );
}

/**
 * Renders text with its hashtags styled and linked where they already are.
 * Nothing is removed or reordered: the segments concatenate back to the exact
 * input string.
 */
function TaggedText({ text }: { text: string }) {
  const segments = React.useMemo(() => segmentTags(text), [text]);
  return (
    <>
      {segments.map((segment, index) =>
        segment.isTag ? (
          <Link
            key={`${segment.text}-${index}`}
            href={`/explore?q=${encodeURIComponent(segment.text)}`}
            className="text-accent transition-colors hover:underline"
          >
            {segment.text}
          </Link>
        ) : (
          <React.Fragment key={index}>{segment.text}</React.Fragment>
        ),
      )}
    </>
  );
}

/** Skeleton that keeps the final height, so nothing jumps when data lands. */
export function PostSkeleton({ withMedia = false }: { withMedia?: boolean }) {
  return (
    <div
      aria-hidden="true"
      aria-busy="true"
      className="flex gap-3 border-b border-line px-4 py-3"
    >
      <div className="sf-skeleton size-10 shrink-0 rounded-full" />
      <div className="min-w-0 flex-1 space-y-2">
        <div className="sf-skeleton h-3 w-40 rounded" />
        <div className="sf-skeleton h-4 w-[92%] rounded" />
        <div className="sf-skeleton h-4 w-[78%] rounded" />
        {withMedia ? (
          <div className="sf-skeleton aspect-[16/9] w-full rounded-lg" />
        ) : null}
        <div className="flex gap-6 pt-1">
          <div className="sf-skeleton size-8 rounded-full" />
          <div className="sf-skeleton size-8 rounded-full" />
          <div className="sf-skeleton size-8 rounded-full" />
        </div>
      </div>
    </div>
  );
}
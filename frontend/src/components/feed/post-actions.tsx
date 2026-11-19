"use client";

import * as React from "react";
import Link from "next/link";
import { Heart, MessageCircle, Share2 } from "lucide-react";
import { notify } from "@/lib/toast";
import { cn } from "@/lib/utils";
import { compactCount } from "@/lib/format";
import { celebrate } from "@/lib/motion";

/**
 * The action row.
 *
 * The like button is a real toggle: aria-pressed carries the state and the
 * accessible name flips to "Unlike", so the control is not colour-only. Reply
 * opens the thread. Share copies a permalink. Repost is absent because the
 * schema has no repost concept — a button that cannot work is worse than no
 * button.
 */
export function PostActions({
  postId,
  liked,
  likeCount,
  replyCount = 0,
  onLike,
  error,
}: {
  postId: string;
  liked: boolean;
  likeCount: number;
  replyCount?: number;
  onLike: () => void;
  error?: string;
}) {
  const heartRef = React.useRef<HTMLSpanElement>(null);

  const handleLike = () => {
    if (!liked) celebrate(heartRef.current);
    onLike();
  };

  const share = async () => {
    const url =
      typeof window === "undefined" ? "" : `${window.location.origin}/post/${postId}`;
    try {
      await navigator.clipboard.writeText(url);
      notify.success("Link copied to clipboard");
    } catch {
      notify.error("Could not copy the link");
    }
  };

  return (
    <div className="mt-3">
      <div className="flex max-w-[var(--layout-actions)] items-center justify-between">
        <Link
          href={`/post/${postId}`}
          aria-label={`Reply${replyCount ? `, ${replyCount} replies` : ""}`}
          className="group -ml-2 inline-flex items-center gap-1 rounded-full p-2 text-tertiary transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <span className="grid size-8 place-items-center rounded-full transition-colors group-hover:bg-accent-soft">
            <MessageCircle className="size-icon-sm" aria-hidden="true" />
          </span>
          <span className="min-w-4 font-mono text-xs tabular-nums">
            {replyCount ? compactCount(replyCount) : ""}
          </span>
        </Link>

        <button
          type="button"
          onClick={handleLike}
          aria-pressed={liked}
          aria-label={
            liked
              ? `Unlike, ${compactCount(likeCount)} likes`
              : `Like, ${compactCount(likeCount)} likes`
          }
          className={cn(
            "group inline-flex items-center gap-1 rounded-full p-2 text-xs transition-colors",
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
            liked ? "text-danger" : "text-tertiary hover:text-danger",
          )}
        >
          <span
            ref={heartRef}
            className={cn(
              "grid size-8 place-items-center rounded-full transition-colors",
              liked ? "bg-danger/10" : "group-hover:bg-danger/10",
            )}
          >
            <Heart
              className={cn("size-icon-sm", liked && "scale-105")}
              fill={liked ? "currentColor" : "none"}
              aria-hidden="true"
            />
          </span>
          <span className="min-w-4 font-mono tabular-nums">
            {likeCount > 0 ? compactCount(likeCount) : ""}
          </span>
        </button>

        <button
          type="button"
          onClick={() => void share()}
          aria-label="Copy link to post"
          className="group inline-flex items-center rounded-full p-2 text-tertiary transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <span className="grid size-8 place-items-center rounded-full transition-colors group-hover:bg-accent-soft">
            <Share2 className="size-icon-sm" aria-hidden="true" />
          </span>
        </button>
      </div>

      {error ? (
        <p role="alert" className="mt-1 text-xs text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
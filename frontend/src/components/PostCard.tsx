import type { ReactNode } from "react";
import { useMemo, useRef, useState } from "react";
import type { Post } from "../lib/api";
import { compactCount, relativeTime } from "../lib/format";
import { celebrate } from "../lib/motion";
import { Button } from "./Button";
import { Link } from "./Link";

/* ==========================================================================
   PostCard
   --------------------------------------------------------------------------
   Density: 7 cards per page is the design intent. Anatomy:
     [avatar] [name + handle + time]
               [content, clamped with a "show more" control]
               [media, 16:10, never reflows the row]
               [actions: reply, repost, like, share]
   States: default | hover (elevation only, never a layout shift) |
           focus-visible (on the card and on each action) | liked |
           error (media failed) | loading (skeleton) | deleted (tombstone).

   Long-content handling: the caption clamps to a measured line count; "Show
   more" is a real button that grows the card and keeps the action row attached
   to the bottom. Overflow never hides content behind a gradient.
   ========================================================================== */

export interface PostCardProps {
  post: Post;
  liked?: boolean;
  likeCount?: number;
  onLike?: (postID: string, liked: boolean) => void;
  onReply?: (postID: string) => void;
  /** Renders the skeleton instead of the post. */
  loading?: boolean;
  /** Renders the tombstone variant. */
  deleted?: boolean;
  /** Error copy for a media failure, shown under the frame. */
  mediaError?: string;
}

const CLAMP_LINES = 6;

export function PostCard({
  post,
  liked,
  likeCount,
  onLike,
  onReply,
  loading = false,
  deleted = false,
  mediaError,
}: PostCardProps) {
  const [expanded, setExpanded] = useState(false);
  const likeRef = useRef<HTMLButtonElement>(null);
  const isLiked = liked ?? post.hasLiked;
  const count = likeCount ?? post.likeCount;
  const lines = useMemo(() => post.content.split("\n"), [post.content]);

  if (loading) return <PostCardSkeleton />;

  if (deleted) {
    return (
      <article className="card card--tombstone" aria-label="Deleted post">
        <p className="card__tombstone-text">This post was removed by its author.</p>
      </article>
    );
  }

  const tooLong = lines.length > CLAMP_LINES || post.content.length > 480;

  return (
    <article className="card" aria-labelledby={`post-${post.id}-author`}>
      <header className="card__header">
        <span className="card__avatar" aria-hidden="true">
          {initial(post.author.name)}
        </span>
        <div className="card__identity">
          <span className="card__name" id={`post-${post.id}-author`}>
            {post.author.name}
          </span>
          <span className="card__handle">
            @{post.author.username} · <time dateTime={post.createdAt}>{relative(post.createdAt)}</time>
          </span>
        </div>
      </header>

      <div
        className={`card__body${expanded ? " is-expanded" : ""}`}
        style={expanded ? undefined : { ["--clamp-lines" as string]: String(CLAMP_LINES) }}
      >
        <p className="card__text">
          {/* Mentions and hashtags render as links; the accessible text of the
              paragraph is unchanged, so a screen reader reads the sentence. */}
          {linkify(post.content)}
        </p>
      </div>

      {tooLong && !expanded && (
        <Button variant="ghost" size="sm" onClick={() => setExpanded(true)}>
          Show more
        </Button>
      )}
      {expanded && tooLong && (
        <Button variant="ghost" size="sm" onClick={() => setExpanded(false)}>
          Show less
        </Button>
      )}

      {post.mediaUrl && (
        <figure className="card__media">
          <img src={post.mediaUrl} alt="" loading="lazy" decoding="async" />
          {mediaError && (
            <figcaption className="card__media-error" role="alert">
              {mediaError}
            </figcaption>
          )}
        </figure>
      )}
      {mediaError && !post.mediaUrl && (
        <p className="card__media-error" role="alert">
          {mediaError}
        </p>
      )}

      <footer className="card__actions">
        <Button variant="ghost" size="sm" onClick={() => onReply?.(post.id)} iconAfter={<ReplyIcon />}>
          Reply
        </Button>
        <Button variant="ghost" size="sm" iconAfter={<RepostIcon />}>
          Repost
        </Button>
        <button
          ref={likeRef}
          type="button"
          className={`action action--like${isLiked ? " is-active" : ""}`}
          aria-pressed={isLiked}
          onClick={() => {
            const next = !isLiked;
            if (next) celebrate(likeRef.current);
            onLike?.(post.id, next);
          }}
        >
          <span aria-hidden="true">
            <HeartIcon filled={isLiked} />
          </span>
          <span className="action__count">{formatCount(count + (isLiked ? 0 : 0))}</span>
          <span className="visually-hidden">{isLiked ? "Unlike" : "Like"}</span>
        </button>
        <Button variant="ghost" size="sm" iconAfter={<ShareIcon />}>
          Share
        </Button>
      </footer>
    </article>
  );
}

/* ---- Skeleton ------------------------------------------------------------ */

export function PostCardSkeleton() {
  return (
    <article className="card card--skeleton" aria-hidden="true" aria-busy="true">
      <div className="card__header">
        <span className="skeleton skeleton--avatar" />
        <span className="skeleton skeleton--name" />
      </div>
      <span className="skeleton skeleton--line" />
      <span className="skeleton skeleton--line is-short" />
      <span className="skeleton skeleton--media" />
      <div className="card__actions">
        <span className="skeleton skeleton--action" />
        <span className="skeleton skeleton--action" />
        <span className="skeleton skeleton--action" />
      </div>
    </article>
  );
}

/* ---- Helpers ------------------------------------------------------------- */

function initial(name: string): string {
  return name.trim().charAt(0).toUpperCase() || "?";
}

/** Relative time for a card's meta row. Shared so wording never drifts. */
function relative(iso: string): string {
  return relativeTime(iso);
}

/** Compact count for the like meter. */
function formatCount(value: number): string {
  return compactCount(value);
}

/** Wraps #tags and @mentions in links without altering the sentence text. */
function linkify(text: string): ReactNode[] {
  const parts = text.split(/([#@][\p{L}\p{N}_]+)/u);
  return parts.map((part, index) => {
    if (part.startsWith("#")) {
      return (
        <Link key={index} href={`#/explore?q=${encodeURIComponent(part.slice(1))}`} variant="inline">
          {part}
        </Link>
      );
    }
    if (part.startsWith("@")) {
      return (
        <Link key={index} href={`#/u/${part.slice(1)}`} variant="inline">
          {part}
        </Link>
      );
    }
    return part;
  });
}


function HeartIcon({ filled }: { filled: boolean }) {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill={filled ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2">
      <path d="M12 20s-7-4.35-9.33-8.36C1.02 8.9 2.6 5.5 6.1 4.9c2.2-.38 4.05.8 5.9 2.9 1.85-2.1 3.7-3.28 5.9-2.9 3.5.6 5.08 4 3.43 6.74C19 15.65 12 20 12 20z" />
    </svg>
  );
}

function ReplyIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M21 11.5a8.38 8.38 0 0 1-8.5 8.5 8.5 8.5 0 0 1-3.8-.9L3 21l1.9-5.7a8.5 8.5 0 0 1-.9-3.8 8.38 8.38 0 0 1 8.5-8.5 8.38 8.38 0 0 1 8.5 8.5z" />
    </svg>
  );
}

function RepostIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M17 2l4 4-4 4" />
      <path d="M3 11V9a4 4 0 0 1 4-4h14" />
      <path d="M7 22l-4-4 4-4" />
      <path d="M21 13v2a4 4 0 0 1-4 4H3" />
    </svg>
  );
}

function ShareIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="M4 12v7a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-7" />
      <path d="M12 3v13" />
      <path d="m7 8 5-5 5 5" />
    </svg>
  );
}

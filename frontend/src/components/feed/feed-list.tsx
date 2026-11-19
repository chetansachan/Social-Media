"use client";

import * as React from "react";
import { useWindowVirtualizer } from "@tanstack/react-virtual";
import { PostCard, PostSkeleton } from "./post-card";
import type { FeedPost, Post } from "@/lib/types";

/**
 * The windowed feed.
 *
 * TanStack Virtual with a 260px estimate and 4 rows of overscan, so a
 * 20,000-item feed mounts roughly `ceil(viewport / 260) + 4` rows rather than
 * 20,000 nodes. Cards have variable height, so measurement is enabled and the
 * estimate is only a starting point.
 *
 * The scroll container is the window rather than a nested div: that keeps
 * mobile browser chrome collapsing naturally, avoids a double scrollbar, and
 * means the sticky top bar keeps working with no extra plumbing.
 */
export function FeedList({
  posts,
  onLike,
  likeErrors,
  isLoading,
  isFetchingNextPage,
  hasNextPage,
  onLoadMore,
}: {
  posts: (Post | FeedPost)[];
  onLike?: (post: Post | FeedPost, liked: boolean) => void;
  likeErrors?: Record<string, string>;
  isLoading?: boolean;
  isFetchingNextPage?: boolean;
  hasNextPage?: boolean;
  onLoadMore?: () => void;
}) {
  const listRef = React.useRef<HTMLDivElement>(null);
  const [offset, setOffset] = React.useState(0);

  React.useEffect(() => {
    if (listRef.current) setOffset(listRef.current.offsetTop);
  }, []);

  const virtualizer = useWindowVirtualizer({
    count: posts.length,
    estimateSize: () => 260,
    overscan: 4,
    scrollMargin: offset,
  });

  const items = virtualizer.getVirtualItems();

  // Fetch the next page when the last rendered row comes into view.
  React.useEffect(() => {
    const last = items[items.length - 1];
    if (!last) return;
    if (last.index >= posts.length - 2 && hasNextPage && !isFetchingNextPage) {
      onLoadMore?.();
    }
  }, [items, posts.length, hasNextPage, isFetchingNextPage, onLoadMore]);

  if (isLoading) {
    return (
      <div>
        <PostSkeleton />
        <PostSkeleton withMedia />
        <PostSkeleton />
      </div>
    );
  }

  return (
    <div ref={listRef}>
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: "100%",
          position: "relative",
        }}
      >
        {items.map((virtualRow) => {
          const post = posts[virtualRow.index];
          if (!post) return null;
          return (
            <div
              key={virtualRow.key}
              data-index={virtualRow.index}
              ref={virtualizer.measureElement}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                transform: `translateY(${virtualRow.start - offset}px)`,
              }}
            >
              <PostCard
                post={post}
                onLike={onLike}
                likeError={likeErrors?.[post.id]}
                priority={virtualRow.index < 2}
              />
            </div>
          );
        })}
      </div>

      {isFetchingNextPage ? <PostSkeleton /> : null}
    </div>
  );
}
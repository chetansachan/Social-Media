"use client";

import * as React from "react";
import { extractTags } from "@/lib/format";
import { useFeed } from "./use-feed";

export type TrendingTag = { tag: string; count: number };

/**
 * Trending, derived from the only honest source the backend offers.
 *
 * There is no trending/tag-listing query and `Post` does not expose its
 * hashtag array, so this counts hashtags in the viewer's own feed. That is
 * labelled "In your feed" in the UI rather than pretending to be global.
 */
export function useTrendingTags(limit = 6): {
  tags: TrendingTag[];
  isLoading: boolean;
} {
  const feed = useFeed();

  const tags = React.useMemo(() => {
    const counts = new Map<string, number>();
    for (const post of feed.posts) {
      for (const raw of extractTags(post.content)) {
        const tag = raw.slice(1).toLowerCase();
        if (!tag) continue;
        counts.set(tag, (counts.get(tag) ?? 0) + 1);
      }
    }
    return [...counts.entries()]
      .map(([tag, count]) => ({ tag, count }))
      .sort((a, b) => b.count - a.count || a.tag.localeCompare(b.tag))
      .slice(0, limit);
  }, [feed.posts, limit]);

  return { tags, isLoading: feed.isLoading };
}

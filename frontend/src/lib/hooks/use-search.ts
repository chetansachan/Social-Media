"use client";

import { useQuery } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import type { Post, User } from "@/lib/types";
import { useSession } from "@/lib/providers/session-provider";

export const searchKeys = {
  posts: (query: string) => ["search", "posts", query] as const,
  people: (query: string) => ["search", "people", query] as const,
};

/**
 * Explore search.
 *
 * Both searches run server-side. `searchPosts` is not a text-only matcher: the
 * service extracts hashtags from the same string and ORs a GIN-indexed array
 * overlap on top of the tsvector and trigram paths. So `#golang` and `golang`
 * both work, and the client sends the raw string without parsing tags itself.
 */
export function useSearch(query: string, mode: "posts" | "people" = "posts") {
  const { status } = useSession();
  const trimmed = query.trim();
  const ready = trimmed.length > 0 && status !== "loading";
  const searchPosts = ready && mode === "posts";
  const searchPeople = ready && mode === "people";

  const posts = useQuery({
    queryKey: searchKeys.posts(trimmed),
    queryFn: async ({ signal }) => {
      const data = await gql<{ searchPosts: Post[] }>(
        operations.searchPosts,
        { query: trimmed, limit: 30 },
        signal,
      );
      return data.searchPosts;
    },
    enabled: searchPosts,
    staleTime: 15_000,
  });

  const people = useQuery({
    queryKey: searchKeys.people(trimmed),
    queryFn: async ({ signal }) => {
      const data = await gql<{ searchUsers: User[] }>(
        operations.searchUsers,
        { query: trimmed, limit: 20 },
        signal,
      );
      return data.searchUsers;
    },
    enabled: searchPeople,
    staleTime: 30_000,
  });

  return {
    posts: posts.data ?? [],
    people: people.data ?? [],
    // Only the active tab fetches; switching tabs reuses its cache when warm.
    isSearching: mode === "posts" ? posts.isFetching : people.isFetching,
    error: mode === "posts" ? posts.error : people.error,
    retry: () => {
      if (mode === "posts") void posts.refetch();
      else void people.refetch();
    },
  };
}

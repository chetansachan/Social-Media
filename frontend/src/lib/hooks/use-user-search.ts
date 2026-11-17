"use client";

import { useQuery } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import { useSession } from "@/lib/providers/session-provider";
import type { User } from "@/lib/types";

/** People search only, for the new-message picker. */
export function useUserSearch(query: string) {
  const { status } = useSession();
  const trimmed = query.trim().slice(0, 100);

  const result = useQuery({
    queryKey: ["search", "people", trimmed] as const,
    enabled: status === "authenticated" && trimmed.length > 0,
    queryFn: async ({ signal }) => {
      const data = await gql<{ searchUsers: User[] }>(
        operations.searchUsers,
        { query: trimmed, limit: 12 },
        signal,
      );
      return data.searchUsers;
    },
    staleTime: 30_000,
  });

  return {
    people: result.data ?? [],
    isSearching: result.isFetching,
    error: result.error,
  };
}

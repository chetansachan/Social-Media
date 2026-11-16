"use client";

import { useQuery } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import { useSession } from "@/lib/providers/session-provider";

export const unreadKey = ["notifications", "unread"] as const;

/** The badge count. Reads the same value the notifications screen updates. */
export function useUnreadCount() {
  const { status } = useSession();

  const query = useQuery({
    queryKey: unreadKey,
    enabled: status === "authenticated",
    queryFn: async ({ signal }) => {
      const data = await gql<{ getUnreadNotificationCount: number }>(
        operations.unreadCount,
        {},
        signal,
      );
      return data.getUnreadNotificationCount;
    },
    staleTime: 30_000,
  });

  return query.data ?? 0;
}

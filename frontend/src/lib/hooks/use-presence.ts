"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import { useSession } from "@/lib/providers/session-provider";

/** A heartbeat older than this is treated as offline. */
const ONLINE_WINDOW_MS = 90_000;

/**
 * Presence from `getPresence`, which returns a map of user id to the unix
 * millisecond of the last heartbeat. Refreshed on an interval because the app
 * does not yet subscribe to chat events.
 */
export function usePresence(userIds: string[]): {
  isOnline: (id: string | undefined) => boolean;
  isLoading: boolean;
} {
  const { status } = useSession();
  const key = React.useMemo(
    () => [...new Set(userIds.filter(Boolean))].sort().join(","),
    [userIds],
  );
  const ids = React.useMemo(() => (key ? key.split(",") : []), [key]);

  const query = useQuery({
    queryKey: ["presence", key],
    enabled: status === "authenticated" && ids.length > 0,
    queryFn: async ({ signal }) => {
      const data = await gql<{ getPresence: Record<string, number | string> }>(
        operations.presence,
        { userIds: ids },
        signal,
      );
      return data.getPresence ?? {};
    },
    refetchInterval: 30_000,
    staleTime: 15_000,
  });

  const isOnline = React.useCallback(
    (id: string | undefined) => {
      if (!id) return false;
      const raw = query.data?.[id];
      if (raw === undefined || raw === null) return false;
      const lastSeen = Number(raw);
      return Number.isFinite(lastSeen) && Date.now() - lastSeen < ONLINE_WINDOW_MS;
    },
    [query.data],
  );

  return { isOnline, isLoading: query.isLoading };
}

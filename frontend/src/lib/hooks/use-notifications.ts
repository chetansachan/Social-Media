"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import { useSession } from "@/lib/providers/session-provider";
import type { NotificationPage, NotificationType } from "@/lib/types";
import { unreadKey } from "./use-unread";

export const notificationKeys = {
  /** Prefix shared by every filtered notification list. */
  list: ["notifications", "list"] as const,
  unread: unreadKey,
  filtered: (filter: NotificationType | null) =>
    ["notifications", "list", filter ?? "ALL"] as const,
};

export function useNotifications(filter: NotificationType | null = null) {
  const { status } = useSession();

  const query = useQuery({
    queryKey: notificationKeys.filtered(filter),
    enabled: status === "authenticated",
    queryFn: async ({ signal }) => {
      const data = await gql<{ getNotifications: NotificationPage }>(
        operations.notifications,
        { limit: 50, filter },
        signal,
      );
      return data.getNotifications;
    },
  });

  return {
    ...query,
    items: query.data?.notifications ?? [],
    unreadCount: query.data?.unreadCount ?? 0,
  };
}

type ListSnapshot = [readonly unknown[], NotificationPage | undefined][];

function snapshotLists(queryClient: ReturnType<typeof useQueryClient>): ListSnapshot {
  return queryClient.getQueriesData<NotificationPage>({
    queryKey: notificationKeys.list,
  });
}

function restoreLists(
  queryClient: ReturnType<typeof useQueryClient>,
  snapshot: ListSnapshot,
): void {
  for (const [key, data] of snapshot) {
    queryClient.setQueryData(key, data);
  }
}

function patchLists(
  queryClient: ReturnType<typeof useQueryClient>,
  patcher: (page: NotificationPage) => NotificationPage,
): void {
  queryClient.setQueriesData<NotificationPage>(
    { queryKey: notificationKeys.list },
    (current) => (current ? patcher(current) : current),
  );
}

/** Mark one row read. Optimistic: the row dims immediately. */
export function useMarkRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (ids: string[]) => {
      await gql(operations.markNotificationsRead, { notificationIds: ids });
    },
    async onMutate(ids) {
      await queryClient.cancelQueries({ queryKey: notificationKeys.list });
      const snapshot = snapshotLists(queryClient);
      patchLists(queryClient, (page) => ({
        ...page,
        unreadCount: Math.max(0, page.unreadCount - ids.length),
        notifications: page.notifications.map((item) =>
          ids.includes(item.id) ? { ...item, isRead: true } : item,
        ),
      }));
      return { snapshot };
    },
    onError(_error, _ids, context) {
      if (context?.snapshot) restoreLists(queryClient, context.snapshot);
    },
    onSettled() {
      void queryClient.invalidateQueries({ queryKey: notificationKeys.list });
      void queryClient.invalidateQueries({ queryKey: unreadKey });
    },
  });
}

/** Mark every visible notification read. */
export function useMarkAllRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      await gql(operations.markAllNotificationsRead);
    },
    async onMutate() {
      await queryClient.cancelQueries({ queryKey: notificationKeys.list });
      const snapshot = snapshotLists(queryClient);
      patchLists(queryClient, (page) => ({
        ...page,
        unreadCount: 0,
        notifications: page.notifications.map((item) => ({
          ...item,
          isRead: true,
        })),
      }));
      return { snapshot };
    },
    onError(_error, _variables, context) {
      if (context?.snapshot) restoreLists(queryClient, context.snapshot);
    },
    onSettled() {
      void queryClient.invalidateQueries({ queryKey: notificationKeys.list });
      void queryClient.invalidateQueries({ queryKey: unreadKey });
    },
  });
}

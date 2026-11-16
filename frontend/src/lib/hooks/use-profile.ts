"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import { useSession } from "@/lib/providers/session-provider";
import type { FollowCounts, FollowStatus, Post, User } from "@/lib/types";

export const profileKeys = {
  user: (username: string) => ["profile", "user", username] as const,
  posts: (userId: string) => ["profile", "posts", userId] as const,
  counts: (userId: string) => ["profile", "counts", userId] as const,
  follow: (viewerId: string, targetId: string) =>
    ["profile", "follow", viewerId, targetId] as const,
};

export function useProfile(username: string, enabled = true) {
  return useQuery({
    queryKey: profileKeys.user(username),
    enabled: enabled && Boolean(username),
    queryFn: async ({ signal }) => {
      const data = await gql<{ getUserByUsername: User }>(
        operations.userByUsername,
        { username },
        signal,
      );
      return data.getUserByUsername;
    },
    retry: false,
  });
}

export function useUpdateProfile() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (input: {
      id: string;
      name?: string;
      username?: string;
      isPublic?: boolean;
    }) => {
      const data = await gql<{ UpdateUser: User }>(operations.updateProfile, {
        input,
      });
      return data.UpdateUser;
    },
    onSuccess: (user) => {
      queryClient.setQueryData(profileKeys.user(user.username), user);
      void queryClient.invalidateQueries({ queryKey: ["profile"] });
    },
  });
}

export function useAuthorPosts(userId: string | undefined) {
  return useQuery({
    queryKey: profileKeys.posts(userId ?? ""),
    enabled: Boolean(userId),
    queryFn: async ({ signal }) => {
      const data = await gql<{ getAllPost: Post[] }>(
        operations.postsByAuthor,
        { authorId: userId },
        signal,
      );
      return data.getAllPost;
    },
  });
}

export function useLikedPosts(userId: string | undefined, enabled = true) {
  return useQuery({
    queryKey: [...profileKeys.posts(userId ?? ""), "liked"] as const,
    enabled: Boolean(userId) && enabled,
    queryFn: async ({ signal }) => {
      const data = await gql<{ getAllLikedPosts: Post[] }>(
        operations.likedPosts,
        { userId },
        signal,
      );
      return data.getAllLikedPosts;
    },
  });
}

export function useFollowCounts(userId: string | undefined) {
  return useQuery({
    queryKey: profileKeys.counts(userId ?? ""),
    enabled: Boolean(userId),
    queryFn: async ({ signal }) => {
      const data = await gql<{ getFollowCounts: FollowCounts }>(
        operations.followCounts,
        { userId },
        signal,
      );
      return data.getFollowCounts;
    },
  });
}

export function useFollowStatus(targetId: string | undefined) {
  const { user } = useSession();
  const enabled = Boolean(user && targetId && user.id !== targetId);

  return useQuery({
    queryKey: profileKeys.follow(user?.id ?? "", targetId ?? ""),
    enabled,
    queryFn: async ({ signal }) => {
      const data = await gql<{ getFollowStatus: FollowStatus }>(
        operations.followStatus,
        { followerId: user!.id, followingId: targetId },
        signal,
      );
      return data.getFollowStatus;
    },
  });
}

export function useToggleFollow(targetId: string | undefined) {
  const queryClient = useQueryClient();
  const { user } = useSession();

  return useMutation({
    mutationFn: async (nextFollowing: boolean) => {
      if (!user || !targetId) throw new Error("Sign in to follow.");
      const query = nextFollowing
        ? operations.followUser
        : operations.unfollowUser;
      const input = nextFollowing
        ? { followerId: user.id, followingId: targetId }
        : { followerId: user.id, followingId: targetId };
      await gql(query, { input });
      return nextFollowing;
    },
    onMutate: async (nextFollowing) => {
      if (!user || !targetId) return {};
      const followKey = profileKeys.follow(user.id, targetId);
      const countsKey = profileKeys.counts(targetId);

      await queryClient.cancelQueries({ queryKey: followKey });
      await queryClient.cancelQueries({ queryKey: countsKey });

      const previousStatus = queryClient.getQueryData<FollowStatus>(followKey);
      const previousCounts = queryClient.getQueryData<FollowCounts>(countsKey);

      queryClient.setQueryData<FollowStatus>(followKey, {
        isFollowing: nextFollowing,
      });
      if (previousCounts) {
        queryClient.setQueryData<FollowCounts>(countsKey, {
          ...previousCounts,
          followersCount: Math.max(
            0,
            previousCounts.followersCount + (nextFollowing ? 1 : -1),
          ),
        });
      }
      return { previousStatus, previousCounts, followKey, countsKey };
    },
    onError: (_error, _next, context) => {
      if (!context?.followKey) return;
      if (context.previousStatus !== undefined) {
        queryClient.setQueryData(context.followKey, context.previousStatus);
      }
      if (context.previousCounts !== undefined) {
        queryClient.setQueryData(context.countsKey, context.previousCounts);
      }
    },
    onSettled: (_data, _error, _next, context) => {
      if (context?.followKey) {
        void queryClient.invalidateQueries({ queryKey: context.followKey });
        void queryClient.invalidateQueries({ queryKey: context.countsKey });
      }
      void queryClient.invalidateQueries({ queryKey: ["feed"] });
    },
  });
}

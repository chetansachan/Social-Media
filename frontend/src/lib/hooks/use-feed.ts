"use client";

import * as React from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { gql, isOffline, operations } from "@/lib/graphql/client";
import { useSession } from "@/lib/providers/session-provider";
import { ApiError, type FeedPage, type FeedPost, type LikePostResponse } from "@/lib/types";

export type FeedData = InfiniteData<FeedPage, string | null>;

/**
 * One feed, not two.
 *
 * `getFeed` resolves the viewer from the bearer token and reads the timeline
 * that fanout_service built from the accounts they follow. There is no
 * "everyone" feed in the backend, so there is no tab to switch between — the
 * old For you / Following pair was inventing a distinction the API cannot make.
 */
export const feedKey = ["feed"] as const;

type DemoPage = FeedPage & { __demo?: true };

export function useFeed() {
  const { status, user } = useSession();

  const query = useInfiniteQuery<
    FeedPage,
    ApiError,
    FeedData,
    readonly unknown[],
    string | null
  >({
    queryKey: feedKey,
    enabled: status === "authenticated",
    initialPageParam: null,
    queryFn: async ({ pageParam, signal }) => {
      try {
        const data = await gql<{ getFeed: FeedPage }>(
          operations.feed,
          { limit: 12, cursor: pageParam },
          signal,
        );
        return data.getFeed;
      } catch (error) {
        // A down gateway renders demo content plus a banner, never a blank
        // screen. The banner is driven by isDemo, not by the error state.
        if (isOffline(error)) {
          // The fixture is ~15 kB; load it only when the gateway is actually
          // unavailable rather than shipping it in the shared shell chunk.
          const { demoFollowingFeed } = await import("@/lib/mock");
          return {
            posts: demoFollowingFeed,
            nextCursor: null,
            hasMore: false,
            __demo: true,
          } as DemoPage;
        }
        throw error;
      }
    },
    getNextPageParam: (last) => (last.hasMore ? last.nextCursor : undefined),
  });

  const isDemo = Boolean((query.data?.pages[0] as DemoPage | undefined)?.__demo);

  const posts = React.useMemo(
    () => query.data?.pages.flatMap((page) => page.posts) ?? [],
    [query.data],
  );

  return { ...query, posts, isDemo, viewer: user };
}

/**
 * Optimistic like with rollback.
 *
 * The schema's input is { postId, userId } — the userId is required, so it
 * comes from the session rather than being omitted. On failure the snapshot is
 * restored *and* the reason is returned, so the button can announce it; a
 * silent revert is a fail.
 */
export function useLike() {
  const queryClient = useQueryClient();
  const { user } = useSession();

  return useMutation({
    mutationFn: async ({ post, liked }: { post: FeedPost; liked: boolean }) => {
      if (!user) throw new ApiError("Sign in to like a post.", "UNAUTHENTICATED");
      try {
        // There is no unlike mutation: likePost is a toggle keyed on the pair.
        const data = await gql<{ likePost: LikePostResponse }>(operations.likePost, {
          input: { postId: post.id, userId: user.id },
        });
        return data.likePost;
      } catch (error) {
        if (isOffline(error)) return null;
        throw error;
      }
    },

    onMutate: async ({ post, liked }) => {
      await queryClient.cancelQueries({ queryKey: feedKey });
      const snapshot = queryClient.getQueryData<FeedData>(feedKey);
      const detailKey = ["post", post.id] as const;
      const detailSnapshot = queryClient.getQueryData<{
        hasLiked: boolean;
        likeCount: number;
      }>(detailKey);

      queryClient.setQueryData<FeedData>(feedKey, (current) => {
        if (!current) return current;
        return {
          ...current,
          pages: current.pages.map((page) => ({
            ...page,
            posts: page.posts.map((item) =>
              item.id === post.id
                ? {
                    ...item,
                    hasLiked: liked,
                    likeCount: Math.max(0, item.likeCount + (liked ? 1 : -1)),
                  }
                : item,
            ),
          })),
        };
      });

      // A post opened from a permalink has its own cache entry; keep it in step
      // with the feed so the detail view does not show a stale heart.
      queryClient.setQueryData(detailKey, (current: { hasLiked: boolean; likeCount: number } | undefined) =>
        current
          ? {
              ...current,
              hasLiked: liked,
              likeCount: Math.max(0, current.likeCount + (liked ? 1 : -1)),
            }
          : current,
      );

      return { snapshot, detailKey, detailSnapshot };
    },

    onError: (_error, _variables, context) => {
      if (context?.snapshot) {
        queryClient.setQueryData(feedKey, context.snapshot);
      }
      if (context?.detailSnapshot) {
        queryClient.setQueryData(context.detailKey, context.detailSnapshot);
      }
    },
  });
}

/**
 * Delete one of the viewer's posts. Optimistic, because the row disappears
 * immediately and a failed delete restores it with a reason.
 */
export function useDeletePost() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (postId: string) => {
      const data = await gql<{ deletePost: { message: string } }>(
        operations.deletePost,
        { id: postId },
      );
      return data.deletePost;
    },
    onMutate: async (postId) => {
      await queryClient.cancelQueries({ queryKey: feedKey });
      const snapshot = queryClient.getQueryData<FeedData>(feedKey);
      queryClient.setQueryData<FeedData>(feedKey, (current) =>
        current
          ? {
              ...current,
              pages: current.pages.map((page) => ({
                ...page,
                posts: page.posts.filter((post) => post.id !== postId),
              })),
            }
          : current,
      );
      return { snapshot };
    },
    onError: (_error, _postId, context) => {
      if (context?.snapshot) queryClient.setQueryData(feedKey, context.snapshot);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["profile"] });
      void queryClient.invalidateQueries({ queryKey: ["search"] });
    },
  });
}

/** What the composer hands the mutation. Media is already an uploaded URL. */
export type PostDraft = {
  content: string;
  /** A Cloudinary URL, or empty for a text-only post. */
  mediaUrl?: string;
};

/**
 * Create a post.
 *
 * `CreatePostInput` requires authorId, content and mediaUrl. The author comes
 * from the token server-side, but the field is non-null in the schema, so it is
 * sent; mediaUrl is required-but-empty when the post carries no media, which is
 * how the feed knows to render text alone.
 */
export function useCreatePost() {
  const queryClient = useQueryClient();
  const { user } = useSession();

  return useMutation({
    mutationFn: async (draft: PostDraft) => {
      if (!user) throw new ApiError("Sign in to post.", "UNAUTHENTICATED");
      const data = await gql<{ CreatePost: FeedPost }>(operations.createPost, {
        input: {
          authorId: user.id,
          content: draft.content,
          mediaUrl: draft.mediaUrl ?? "",
          isPublic: true,
        },
      });
      return data.CreatePost;
    },
    onSuccess: (post) => {
      queryClient.setQueryData<FeedData>(feedKey, (current) => {
        if (!current) return current;
        const [first, ...rest] = current.pages;
        if (!first) return current;
        return {
          ...current,
          pages: [{ ...first, posts: [post, ...first.posts] }, ...rest],
        };
      });
    },
  });
}

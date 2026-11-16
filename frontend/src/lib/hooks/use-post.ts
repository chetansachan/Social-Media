"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { gql, operations } from "@/lib/graphql/client";
import { useSession } from "@/lib/providers/session-provider";
import type { Comment, CommentPage, Post } from "@/lib/types";

export const postKeys = {
  post: (id: string) => ["post", id] as const,
  comments: (id: string) => ["post", id, "comments"] as const,
  replies: (id: string, parentId: string) =>
    ["post", id, "replies", parentId] as const,
};

export function usePost(id: string) {
  return useQuery({
    queryKey: postKeys.post(id),
    enabled: Boolean(id),
    queryFn: async ({ signal }) => {
      const data = await gql<{ getPostById: Post }>(
        operations.postById,
        { id },
        signal,
      );
      return data.getPostById;
    },
    retry: false,
  });
}

export function useComments(postId: string) {
  return useQuery({
    queryKey: postKeys.comments(postId),
    enabled: Boolean(postId),
    queryFn: async ({ signal }) => {
      const data = await gql<{ getComments: CommentPage }>(
        operations.comments,
        { postId, limit: 50 },
        signal,
      );
      return data.getComments;
    },
  });
}

/** Replies for one comment, fetched only when its thread is opened. */
export function useReplies(postId: string, parentId: string, enabled: boolean) {
  return useQuery({
    queryKey: postKeys.replies(postId, parentId),
    enabled: enabled && Boolean(postId && parentId),
    queryFn: async ({ signal }) => {
      const data = await gql<{ getComments: CommentPage }>(
        operations.comments,
        { postId, parentId, limit: 50 },
        signal,
      );
      return data.getComments;
    },
  });
}

export function useCreateComment(postId: string, parentId?: string) {
  const queryClient = useQueryClient();
  const { status } = useSession();

  return useMutation({
    mutationFn: async (content: string) => {
      if (status !== "authenticated") throw new Error("Sign in to reply.");
      const data = await gql<{ createComment: Comment }>(
        operations.createComment,
        { input: { postId, parentId, content } },
      );
      return data.createComment;
    },
    onSuccess: (comment) => {
      if (!parentId) {
        queryClient.setQueryData<CommentPage>(
          postKeys.comments(postId),
          (current) =>
            current ? { ...current, comments: [comment, ...current.comments] } : current,
        );
      }
      void queryClient.invalidateQueries({ queryKey: postKeys.comments(postId) });
      if (parentId) {
        void queryClient.invalidateQueries({
          queryKey: postKeys.replies(postId, parentId),
        });
      }
    },
  });
}

/**
 * Comment likes. The schema exposes `likeComment` as a toggle; the response
 * carries the resulting state, and the optimistic patch updates whichever
 * cache holds the comment (top-level or a reply thread).
 */
export function useLikeComment(postId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ comment }: { comment: Comment; liked: boolean }) => {
      const data = await gql<{ likeComment: { isLiked: boolean } }>(
        operations.likeComment,
        { input: { commentId: comment.id } },
      );
      return data.likeComment;
    },
    onMutate: async ({ comment, liked }) => {
      await queryClient.cancelQueries({ queryKey: ["post", postId] });
      const snapshots = queryClient.getQueriesData<CommentPage>({
        queryKey: ["post", postId],
      });
      queryClient.setQueriesData<CommentPage>({ queryKey: ["post", postId] }, (page) =>
        page && Array.isArray(page.comments)
          ? {
              ...page,
              comments: page.comments.map((item) =>
                item.id === comment.id
                  ? {
                      ...item,
                      hasLiked: liked,
                      likeCount: Math.max(0, item.likeCount + (liked ? 1 : -1)),
                    }
                  : item,
              ),
            }
          : page,
      );
      return { snapshots };
    },
    onError: (_error, _variables, context) => {
      for (const [key, page] of context?.snapshots ?? []) {
        queryClient.setQueryData(key, page);
      }
    },
  });
}

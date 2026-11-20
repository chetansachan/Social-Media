"use client";

import * as React from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  CornerDownRight,
  FileQuestion,
  Heart,
  MessageCircle,
  Send,
} from "lucide-react";
import { notify } from "@/lib/toast";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { PostCard, PostSkeleton } from "@/components/feed/post-card";
import { cn } from "@/lib/utils";
import { compactCount, relativeTime } from "@/lib/format";
import {
  useComments,
  useCreateComment,
  useLikeComment,
  usePost,
  useReplies,
} from "@/lib/hooks/use-post";
import { useLike } from "@/lib/hooks/use-feed";
import { useSession } from "@/lib/providers/session-provider";
import { celebrate } from "@/lib/motion";
import type { Comment } from "@/lib/types";

export default function PostPage() {
  const params = useParams<{ id: string }>();
  const id = params?.id ?? "";
  const router = useRouter();
  const { user, status } = useSession();

  const post = usePost(id);
  const comments = useComments(id);
  const createComment = useCreateComment(id);
  const like = useLike();

  const [value, setValue] = React.useState("");
  const [likeError, setLikeError] = React.useState<string | undefined>();

  const submit = () => {
    const content = value.trim();
    if (!content) return;
    void addReply(createComment, content, () => setValue(""));
  };

  if (post.isLoading) {
    return (
      <>
        <div className="border-b border-line px-4 py-3">
          <div className="h-5 w-24 rounded bg-hover" />
        </div>
        <SkeletonPost />
      </>
    );
  }

  if (post.error || !post.data) {
    return (
      <>
        <EmptyState
          title="This post isn't available"
          body="It may have been deleted, or it belongs to a private account."
          action={
            <Button asChild variant="secondary" size="md">
              <Link href="/">Back home</Link>
            </Button>
          }
        />
      </>
    );
  }

  const item = post.data;

  return (
    <>
      <div className="sticky top-0 z-[calc(var(--z-sticky)-1)] flex items-center gap-3 border-b border-line bg-page/90 px-3 py-2 backdrop-blur-xl">
        <Button asChild variant="ghost" size="icon" aria-label="Back">
          <Link href="/">
            <ArrowLeft className="size-icon-sm" aria-hidden="true" />
          </Link>
        </Button>
        <h1 className="sr-only">Post</h1>
      </div>

      <PostCard
        post={item}
        variant="detail"
        onLike={(target, liked) => {
          setLikeError(undefined);
          like.mutate(
            { post: target as typeof item, liked },
            {
              onError: (error) => {
                setLikeError(error.message || "That did not go through.");
                notify.error("Could not update like", error.message);
              },
            },
          );
        }}
        likeError={likeError}
        replyCount={comments.data?.comments.length}
        onDeleted={() => router.push("/")}
      />

      {status === "authenticated" ? (
        <div className="flex gap-3 border-b border-line px-4 py-3">
          {user ? <Avatar name={user.name} handle={user.username} size="md" /> : null}
          <div className="min-w-0 flex-1">
            <label htmlFor="reply" className="sr-only">
              Write a reply
            </label>
            <textarea
              id="reply"
              value={value}
              onChange={(event) => setValue(event.target.value)}
              onKeyDown={(event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
                  event.preventDefault();
                  submit();
                }
              }}
              rows={2}
              maxLength={1000}
              placeholder="Post your reply"
              className="w-full resize-none bg-transparent pt-1 font-sans text-lg leading-snug text-primary placeholder:text-tertiary focus:outline-none"
            />
            <div className="mt-1 flex items-center justify-end gap-3 border-t border-line pt-2">
              <span className="font-mono text-xs text-tertiary">
                {value.trim().length}/1000
              </span>
              <Button
                size="sm"
                onClick={submit}
                disabled={!value.trim()}
                loading={createComment.isPending}
                className="px-5"
              >
                Reply
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <p className="border-b border-line px-4 py-4 text-sm text-secondary">
          <Link href="/login" className="font-semibold text-accent hover:underline">
            Sign in
          </Link>{" "}
          to join the discussion.
        </p>
      )}

      <div className="flex items-center gap-2 px-4 py-3">
        <CornerDownRight className="size-icon-xs text-tertiary" aria-hidden="true" />
        <h3 className="font-sans text-base font-semibold text-primary">
          Replies
        </h3>
        <span className="font-mono text-xs text-tertiary">
          {comments.data?.comments.length ?? 0}
        </span>
      </div>

      {comments.isLoading ? (
        <div aria-hidden="true">
          {[0, 1, 2].map((index) => (
            <div key={index} className="flex gap-3 px-4 py-3">
              <div className="sf-skeleton size-8 rounded-full" />
              <div className="flex-1 space-y-2">
                <div className="sf-skeleton h-3 w-1/3 rounded" />
                <div className="sf-skeleton h-4 w-5/6 rounded" />
              </div>
            </div>
          ))}
        </div>
      ) : comments.isError ? (
        <div className="border-t border-line px-6 py-12 text-center">
          <p className="font-display text-xl text-primary">
            Replies could not be loaded
          </p>
          <p className="mt-1 text-sm text-secondary">{comments.error.message}</p>
          <Button
            variant="secondary"
            size="sm"
            className="mt-4"
            onClick={() => comments.refetch()}
          >
            Try again
          </Button>
        </div>
      ) : comments.data && comments.data.comments.length > 0 ? (
        <ul>
          {comments.data.comments.map((comment) => (
            <li key={comment.id}>
              <CommentRow
                postId={id}
                comment={comment}
                canReply={status === "authenticated"}
              />
            </li>
          ))}
        </ul>
      ) : (
        <div className="border-t border-line px-6 py-14 text-center">
          <MessageCircle className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
          <h4 className="mt-3 font-display text-2xl text-primary">
            No replies yet
          </h4>
          <p className="mt-1 text-sm text-secondary">
            Be the first to add something useful.
          </p>
        </div>
      )}
    </>
  );
}

function CommentRow({
  postId,
  comment,
  canReply,
  depth = 0,
}: {
  postId: string;
  comment: Comment;
  canReply: boolean;
  depth?: number;
}) {
  const [open, setOpen] = React.useState(false);
  const [replying, setReplying] = React.useState(false);
  const [draft, setDraft] = React.useState("");
  const like = useLikeComment(postId);
  const createReply = useCreateComment(postId, comment.id);
  const replies = useReplies(postId, comment.id, open);
  const heartRef = React.useRef<HTMLSpanElement>(null);

  const submitReply = () => {
    const content = draft.trim();
    if (!content) return;
    createReply.mutate(content, {
      onSuccess: () => {
        setDraft("");
        setReplying(false);
        setOpen(true);
        notify.success("Reply posted");
      },
      onError: (error) => notify.error(error.message),
    });
  };

  const onLike = () => {
    if (!comment.hasLiked) celebrate(heartRef.current);
    like.mutate(
      { comment, liked: !comment.hasLiked },
      { onError: (error) => notify.error(error.message) },
    );
  };

  return (
    <article
      className={cn(
        "flex gap-3 py-3 transition-colors hover:bg-hover/30",
        depth === 0 ? "border-b border-line px-4" : "pr-0",
      )}
    >
      <Link
        href={`/profile/${comment.author.username}`}
        className="h-fit shrink-0 rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        aria-label={`${comment.author.name} profile`}
      >
        <Avatar
          name={comment.author.name}
          handle={comment.author.username}
          size={depth === 0 ? "sm" : "xs"}
        />
      </Link>

      <div className="min-w-0 flex-1">
        <header className="flex flex-wrap items-baseline gap-x-1.5">
          <Link
            href={`/profile/${comment.author.username}`}
            className="truncate font-sans text-sm font-semibold text-primary hover:underline"
          >
            {comment.author.name}
          </Link>
          <span className="truncate font-mono text-xs text-tertiary">
            @{comment.author.username}
          </span>
          <span aria-hidden="true" className="text-tertiary">
            ·
          </span>
          <time
            dateTime={comment.createdAt}
            className="font-mono text-xs text-tertiary"
          >
            {relativeTime(comment.createdAt)}
          </time>
        </header>

        <p className="mt-0.5 whitespace-pre-wrap break-words font-sans text-base leading-snug text-primary">
          {comment.isDeleted ? (
            <span className="italic text-tertiary">This reply was deleted.</span>
          ) : (
            comment.content
          )}
        </p>

        {!comment.isDeleted ? (
          <div className="mt-1 flex items-center gap-4">
            {canReply ? (
              <button
                type="button"
                onClick={() => setReplying((value) => !value)}
                aria-expanded={replying}
                className="inline-flex items-center gap-1.5 rounded-full py-1 text-xs text-tertiary transition-colors hover:text-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              >
                <MessageCircle className="size-icon-xs" aria-hidden="true" />
                Reply
              </button>
            ) : null}

            <button
              type="button"
              onClick={onLike}
              aria-pressed={comment.hasLiked}
              aria-label={
                comment.hasLiked
                  ? `Unlike reply, ${compactCount(comment.likeCount)} likes`
                  : `Like reply, ${compactCount(comment.likeCount)} likes`
              }
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full py-1 text-xs transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                comment.hasLiked ? "text-danger" : "text-tertiary hover:text-danger",
              )}
            >
              <span ref={heartRef} className="inline-flex">
                <Heart
                  className="size-icon-xs"
                  fill={comment.hasLiked ? "currentColor" : "none"}
                  aria-hidden="true"
                />
              </span>
              <span className="font-mono tabular-nums">
                {comment.likeCount > 0 ? compactCount(comment.likeCount) : ""}
              </span>
            </button>

            {comment.replyCount > 0 ? (
              <button
                type="button"
                onClick={() => setOpen((value) => !value)}
                aria-expanded={open}
                className="rounded-full py-1 text-xs font-medium text-accent transition-colors hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              >
                {open
                  ? "Hide replies"
                  : `View ${comment.replyCount} ${comment.replyCount === 1 ? "reply" : "replies"}`}
              </button>
            ) : null}
          </div>
        ) : null}

        {replying ? (
          <div className="mt-2 flex gap-2">
            <textarea
              aria-label={`Reply to ${comment.author.name}`}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
                  event.preventDefault();
                  submitReply();
                }
              }}
              rows={2}
              maxLength={1000}
              placeholder={`Reply to ${comment.author.name}`}
              className="w-full resize-none rounded-lg border border-line bg-page px-3 py-2 text-sm text-primary placeholder:text-tertiary focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring"
            />
            <Button
              size="sm"
              onClick={submitReply}
              disabled={!draft.trim()}
              loading={createReply.isPending}
              aria-label="Send reply"
              className="self-end"
            >
              <Send className="size-icon-xs" aria-hidden="true" />
            </Button>
          </div>
        ) : null}

        {open ? (
          <div className="mt-2 border-l border-line pl-3">
            {replies.isLoading ? (
              <p className="py-2 text-xs text-tertiary">Loading replies…</p>
            ) : replies.data && replies.data.comments.length > 0 ? (
              <ul>
                {replies.data.comments.map((child) => (
                  <li key={child.id}>
                    <CommentRow
                      postId={postId}
                      comment={child}
                      canReply={canReply}
                      depth={depth + 1}
                    />
                  </li>
                ))}
              </ul>
            ) : (
              <p className="py-2 text-xs text-tertiary">No replies yet.</p>
            )}
          </div>
        ) : null}
      </div>
    </article>
  );
}

function addReply(
  mutation: ReturnType<typeof useCreateComment>,
  content: string,
  clear: () => void,
) {
  mutation.mutate(content, {
    onSuccess: () => {
      clear();
      notify.success("Reply posted");
    },
    onError: (error) => notify.error(error.message),
  });
}

function EmptyState({
  title,
  body,
  action,
}: {
  title: string;
  body: string;
  action: React.ReactNode;
}) {
  return (
    <div className="px-8 py-20 text-center">
      <FileQuestion className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
      <h2 className="mt-4 font-display text-3xl text-primary">{title}</h2>
      <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">{body}</p>
      <div className="mt-6">{action}</div>
    </div>
  );
}

function SkeletonPost() {
  return (
    <div>
      <PostSkeleton />
      <PostSkeleton />
    </div>
  );
}
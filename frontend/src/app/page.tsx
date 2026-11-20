"use client";

import * as React from "react";
import Link from "next/link";
import { ArrowRight, RefreshCw, Sparkles } from "lucide-react";
import { notify } from "@/lib/toast";
import { DemoBanner } from "@/components/chrome/demo-banner";
import { Composer } from "@/components/feed/composer";
import { FeedList } from "@/components/feed/feed-list";
import { Button } from "@/components/ui/button";
import { useCreatePost, useFeed, useLike } from "@/lib/hooks/use-feed";
import { useSession } from "@/lib/providers/session-provider";
import type { FeedPost } from "@/lib/types";

/**
 * Home — one feed.
 *
 * `getFeed` is follow-only: the gateway resolves the viewer from the token and
 * reads the timeline fanout_service built from the accounts they follow. There
 * is no "everyone" feed to switch to, so there are no tabs.
 */
export default function HomePage() {
  const { status, user } = useSession();
  const feed = useFeed();
  const like = useLike();
  const createPost = useCreatePost();
  const [likeErrors, setLikeErrors] = React.useState<Record<string, string>>({});
  const lastError = React.useRef<string | null>(null);
  const announcedDemo = React.useRef(false);

  const handleLike = React.useCallback(
    (post: FeedPost, liked: boolean) => {
      setLikeErrors((current) => {
        if (!current[post.id]) return current;
        const next = { ...current };
        delete next[post.id];
        return next;
      });

      like.mutate(
        { post, liked },
        {
          onError: (error) => {
            setLikeErrors((current) => ({
              ...current,
              [post.id]: error.message || "That did not go through. Try again.",
            }));
            notify.error("Could not update like", error.message);
          },
        },
      );
    },
    [like],
  );

  const error = feed.error;

  React.useEffect(() => {
    if (!error) {
      lastError.current = null;
      return;
    }
    const key = `${error.code}:${error.message}`;
    if (lastError.current === key) return;
    lastError.current = key;
    notify.error(
      error.code === "RATE_LIMITED" ? "Slow down for a moment" : "Timeline unavailable",
      error.message,
    );
  }, [error]);

  React.useEffect(() => {
    if (!feed.isDemo || announcedDemo.current) return;
    announcedDemo.current = true;
    notify.warning(
      "Gateway offline",
      "Showing demo posts until the connection returns.",
    );
  }, [feed.isDemo]);

  if (status === "loading") {
    return (
      <>
        <div className="border-b border-line px-4 py-3">
          <div className="flex gap-3">
            <div className="sf-skeleton size-10 rounded-full" aria-hidden="true" />
            <div className="flex-1 space-y-2" aria-hidden="true">
              <div className="sf-skeleton h-4 w-1/3 rounded" />
              <div className="sf-skeleton h-4 w-3/4 rounded" />
            </div>
          </div>
        </div>
        <FeedList posts={[]} isLoading />
      </>
    );
  }

  if (status === "anonymous") {
    return (
      <>
        <section className="relative overflow-hidden border-b border-line px-6 pb-10 pt-14 sm:px-10 sm:pb-14 sm:pt-20">
          <div
            aria-hidden="true"
            className="pointer-events-none absolute -right-24 -top-28 size-80 rounded-full bg-accent-gradient opacity-20 blur-3xl"
          />
          <span className="inline-flex items-center gap-2 rounded-full border border-line bg-inset px-3 py-1 font-mono text-2xs uppercase tracking-mono text-secondary">
            <Sparkles className="size-icon-xs text-accent" aria-hidden="true" />
            Follow-only · chronological
          </span>
          <h2 className="mt-6 max-w-md font-display text-4xl leading-display tracking-display text-primary sm:text-display">
            Happening now.
          </h2>
          <p className="mt-4 max-w-sm text-lg leading-relaxed text-secondary">
            Join SocialFlow to follow the engineers and makers you learn from.
            One feed, newest first, no algorithm.
          </p>
          <div className="mt-8 flex flex-wrap items-center gap-3">
            <Button asChild size="lg" className="w-full sm:w-auto">
              <Link href="/login">
                Sign in
                <ArrowRight className="size-icon-xs" aria-hidden="true" />
              </Link>
            </Button>
            <Button asChild variant="secondary" size="lg" className="w-full sm:w-auto">
              <Link href="/login?mode=register">Create account</Link>
            </Button>
          </div>
          <p className="mt-6 font-mono text-2xs tracking-mono text-tertiary">
            Demo · chetan@example.com / password123
          </p>
        </section>

        <Composer />
      </>
    );
  }

  return (
    <>
      <Composer
        onSubmit={(draft) =>
          createPost.mutate(draft, {
            onError: (mutationError) => notify.error(mutationError.message),
            onSuccess: () => notify.success("Your post is live"),
          })
        }
        isPending={createPost.isPending}
        error={createPost.error?.message}
      />

      {feed.isDemo ? <DemoBanner /> : null}

      {error && !feed.isDemo ? (
        <div className="border-b border-line px-8 py-14 text-center">
          <p className="font-display text-2xl text-primary">
            {error.code === "RATE_LIMITED"
              ? "You are posting faster than the gateway allows."
              : error.code === "UNAUTHENTICATED"
                ? "Your session expired."
                : "The feed could not be loaded."}
          </p>
          <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">
            {error.code === "UNAUTHENTICATED"
              ? "Sign in again to read the timeline built from the people you follow."
              : "The gateway did not return a timeline. Try again in a moment."}
          </p>
          <Button
            variant="secondary"
            size="sm"
            className="mt-5"
            onClick={() => (error.code === "UNAUTHENTICATED" ? undefined : feed.refetch())}
            asChild={error.code === "UNAUTHENTICATED"}
          >
            {error.code === "UNAUTHENTICATED" ? (
              <Link href="/login">Sign in</Link>
            ) : (
              <>
                <RefreshCw className="size-icon-xs" aria-hidden="true" />
                {error.code === "RATE_LIMITED" && error.retryAfter
                  ? `Retry in ${error.retryAfter}s`
                  : "Try again"}
              </>
            )}
          </Button>
        </div>
      ) : feed.posts.length === 0 && !feed.isLoading ? (
        <div className="border-b border-line px-8 py-16 text-center">
          <h2 className="font-display text-3xl text-primary">
            Your feed is quiet
          </h2>
          <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-secondary">
            This feed only shows accounts you follow. Find people writing about
            what you are building — or post the first thing yourself.
          </p>
          <Button asChild size="md" className="mt-6">
            <Link href="/explore">
              Find people to follow
              <ArrowRight className="size-icon-xs" aria-hidden="true" />
            </Link>
          </Button>
        </div>
      ) : (
        <FeedList
          posts={feed.posts}
          onLike={handleLike}
          likeErrors={likeErrors}
          isLoading={feed.isLoading}
          isFetchingNextPage={feed.isFetchingNextPage}
          hasNextPage={feed.hasNextPage}
          onLoadMore={() => feed.fetchNextPage()}
        />
      )}

      {user && feed.posts.length > 0 ? (
        <p className="px-6 py-10 text-center font-mono text-xs text-tertiary">
          You&rsquo;re all caught up.
        </p>
      ) : null}
    </>
  );
}
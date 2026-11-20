"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Hash, Search, SearchX, Sparkles, TrendingUp } from "lucide-react";
import { PostCard, PostSkeleton } from "@/components/feed/post-card";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";
import { useSearch } from "@/lib/hooks/use-search";
import { notify } from "@/lib/toast";
import { useTrendingTags } from "@/lib/hooks/use-trending";
import { useSession } from "@/lib/providers/session-provider";
import { useFollowStatus, useToggleFollow } from "@/lib/hooks/use-profile";
import type { User } from "@/lib/types";

type Mode = "posts" | "people";

const SEARCH_LIMIT = 100;

export default function ExplorePage() {
  return (
    <React.Suspense
      fallback={
        <>
          <div className="px-4 py-4">
            <div className="sf-skeleton h-11 w-full rounded-full" />
          </div>
        </>
      }
    >
      <ExploreInner />
    </React.Suspense>
  );
}

function ExploreInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const urlQuery = searchParams?.get("q") ?? "";

  const [query, setQuery] = React.useState(urlQuery);
  const [debounced, setDebounced] = React.useState(urlQuery.trim());
  const [mode, setMode] = React.useState<Mode>("posts");
  const lastUrl = React.useRef(urlQuery);
  const inputRef = React.useRef<HTMLInputElement>(null);

  // External navigation (Back/Forward, a tag link) updates the field.
  React.useEffect(() => {
    if (urlQuery !== lastUrl.current) {
      lastUrl.current = urlQuery;
      setQuery(urlQuery);
    }
  }, [urlQuery]);

  // 250ms debounce, then mirror the term into the URL so it is shareable.
  React.useEffect(() => {
    const timer = setTimeout(() => {
      const next = query.trim().slice(0, SEARCH_LIMIT);
      setDebounced(next);
      if (next === lastUrl.current) return;
      lastUrl.current = next;
      router.replace(
        next ? `/explore?q=${encodeURIComponent(next)}` : "/explore",
        { scroll: false },
      );
    }, 250);
    return () => clearTimeout(timer);
  }, [query, router]);

  const { posts, people, isSearching, error, retry } = useSearch(debounced, mode);
  const trending = useTrendingTags(8);
  const { user } = useSession();
  const lastSearchError = React.useRef<string | null>(null);

  React.useEffect(() => {
    if (!error) {
      lastSearchError.current = null;
      return;
    }
    if (lastSearchError.current === error.message) return;
    lastSearchError.current = error.message;
    notify.error("Search failed", error.message);
  }, [error]);

  const hasQuery = debounced.length > 0;
  const results = mode === "posts" ? posts.length : people.length;

  return (
    <>
      <div className="sticky top-0 z-[calc(var(--z-sticky)-1)] border-b border-line bg-page/90 backdrop-blur-xl">
        <div className="relative px-4 py-2">
          <Search
            className="pointer-events-none absolute left-8 top-1/2 size-icon-xs -translate-y-1/2 text-tertiary"
            aria-hidden="true"
          />
          <label htmlFor="explore-search" className="sr-only">
            Search posts, people, or tags
          </label>
          <input
            ref={inputRef}
            id="explore-search"
            type="text"
            role="searchbox"
            aria-controls="explore-results"
            autoComplete="off"
            spellCheck={false}
            value={query}
            maxLength={SEARCH_LIMIT}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") {
                event.preventDefault();
                setQuery("");
                inputRef.current?.focus();
              }
            }}
            placeholder="Search posts, people, or #tags"
            className={cn(
              "h-11 w-full rounded-full border border-line bg-inset pl-11 pr-16 text-sm text-primary",
              "placeholder:text-tertiary transition-colors hover:border-line-interactive",
              "focus-visible:border-accent focus-visible:bg-page focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring",
            )}
          />
          {query ? (
            <kbd className="pointer-events-none absolute right-8 top-1/2 -translate-y-1/2 rounded border border-line px-1.5 py-0.5 font-mono text-2xs text-tertiary">
              Esc
            </kbd>
          ) : null}
        </div>

        <Tabs value={mode} onValueChange={(value) => setMode(value as Mode)}>
          <TabsList aria-label="Search scope">
            <TabsTrigger value="posts">
              Posts
              {hasQuery && posts.length > 0 ? (
                <span className="font-mono text-xs text-tertiary">{posts.length}</span>
              ) : null}
            </TabsTrigger>
            <TabsTrigger value="people">
              People
              {hasQuery && people.length > 0 ? (
                <span className="font-mono text-xs text-tertiary">{people.length}</span>
              ) : null}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      <div id="explore-results">
        {!hasQuery ? (
          <section aria-labelledby="explore-trending">
            <div className="border-b border-line px-4 py-4">
              <h2
                id="explore-trending"
                className="flex items-center gap-2 font-display text-3xl text-primary"
              >
                <TrendingUp className="size-icon-sm text-accent" aria-hidden="true" />
                Trending in your feed
              </h2>
              <p className="mt-1 text-sm text-secondary">
                Counted from the hashtags in the posts you already see. Pick one to
                search it.
              </p>
            </div>

            {trending.tags.length === 0 ? (
              <div className="px-8 py-16 text-center">
                <Sparkles className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
                <h3 className="mt-3 font-display text-2xl text-primary">
                  Nothing trending yet
                </h3>
                <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">
                  Follow a few people, or try a tag like{" "}
                  <span className="font-mono text-accent-alt">#postgres</span> in the
                  search field.
                </p>
              </div>
            ) : (
              <ul>
                {trending.tags.map((trend) => (
                  <li key={trend.tag}>
                    <button
                      type="button"
                      onClick={() => setQuery(`#${trend.tag}`)}
                      className="group flex w-full items-start gap-4 border-b border-line px-4 py-4 text-left transition-colors hover:bg-hover focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
                    >
                      <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-full bg-inset text-tertiary transition-colors group-hover:text-accent">
                        <Hash className="size-icon-sm" aria-hidden="true" />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-sans text-lg font-semibold text-primary">
                          #{trend.tag}
                        </span>
                        <span className="font-mono text-xs text-tertiary">
                          {trend.count} {trend.count === 1 ? "post" : "posts"} in your feed
                        </span>
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}

            <div className="px-4 py-6 text-sm text-secondary">
              Looking for a person? Type a name or handle above, then switch to{" "}
              <span className="font-semibold text-primary">People</span>.
            </div>
          </section>
        ) : error ? (
          <div className="px-8 py-16 text-center">
            <SearchX className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
            <h2 className="mt-3 font-display text-2xl text-primary">
              Search is unavailable
            </h2>
            <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">
              {error.message}
            </p>
            <Button variant="secondary" size="sm" className="mt-5" onClick={retry}>
              Try again
            </Button>
          </div>
        ) : isSearching && results === 0 ? (
          <div>
            <PostSkeleton />
            <PostSkeleton />
          </div>
        ) : results === 0 ? (
          <div className="px-8 py-16 text-center">
            <SearchX className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
            <h2 className="mt-3 font-display text-2xl text-primary">
              No {mode === "posts" ? "posts" : "people"} match &ldquo;{debounced}&rdquo;
            </h2>
            <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">
              Try a broader term, a single word, or a hashtag.
            </p>
            <Button variant="secondary" size="sm" className="mt-5" onClick={() => setQuery("")}>
              Clear search
            </Button>
          </div>
        ) : mode === "posts" ? (
          posts.map((post, index) => (
            <PostCard key={post.id} post={post} priority={index < 2} />
          ))
        ) : (
          <ul>
            {people.map((person) => (
              <li key={person.id}>
                <PersonRow person={person} isSelf={person.id === user?.id} />
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  );
}

function PersonRow({ person, isSelf }: { person: User; isSelf: boolean }) {
  const followStatus = useFollowStatus(person.id);
  const isFollowing = followStatus.data?.isFollowing ?? false;

  return (
    <div className="flex items-start gap-3 border-b border-line px-4 py-3 transition-colors hover:bg-hover/40">
      <Link
        href={`/profile/${person.username}`}
        className="shrink-0 rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        aria-label={`${person.name} profile`}
      >
        <Avatar name={person.name} handle={person.username} size="md" />
      </Link>
      <div className="min-w-0 flex-1">
        <Link
          href={`/profile/${person.username}`}
          className="block truncate font-sans text-base font-semibold text-primary hover:underline"
        >
          {person.name}
        </Link>
        <p className="truncate font-mono text-xs text-tertiary">@{person.username}</p>
        <p className="mt-0.5 text-xs text-tertiary">
          {person.isPublic ? "Public account" : "Private account"}
        </p>
      </div>
      <FollowToggle
        targetId={person.id}
        isSelf={isSelf}
        isFollowing={isFollowing}
        name={person.name}
      />
    </div>
  );
}

function FollowToggle({
  targetId,
  isSelf,
  isFollowing,
  name,
}: {
  targetId: string;
  isSelf: boolean;
  isFollowing: boolean;
  name: string;
}) {
  const follow = useToggleFollow(targetId);
  if (isSelf) {
    return <span className="text-xs text-tertiary">You</span>;
  }
  return (
    <Button
      variant={isFollowing ? "secondary" : "primary"}
      size="sm"
      loading={follow.isPending}
      onClick={() =>
        follow.mutate(!isFollowing, {
          onSuccess: () =>
            notify.success(
              isFollowing ? `Unfollowed ${name}` : `Following ${name}`,
            ),
          onError: (error) =>
            notify.error("Could not update follow", error.message),
        })
      }
      aria-label={`${isFollowing ? "Unfollow" : "Follow"} ${name}`}
      className="shrink-0"
    >
      {isFollowing ? "Following" : "Follow"}
    </Button>
  );
}
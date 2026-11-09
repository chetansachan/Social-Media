import { useCallback, useEffect, useState } from "react";
import { AppShell } from "../components/AppShell";
import { FeedList } from "../components/FeedList";
import { PostCard, PostCardSkeleton } from "../components/PostCard";
import { SearchField } from "../components/Field";
import { Link } from "../components/Link";
import { ApiError, operations, request } from "../lib/api";
import type { SearchPostsResponse, SearchUsersResponse } from "../lib/api";
import { demoAuthors, demoPosts } from "../lib/mock";

/* ==========================================================================
   Explore — search.
   --------------------------------------------------------------------------
   One input, two result kinds, and the combobox pattern. The empty state is
   not a failure: a query with no matches explains that plainly and offers the
   next thing to try, which is the difference between an empty result and a
   broken search.
   ========================================================================== */

type State =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "ready"; posts: typeof demoPosts; users: typeof demoAuthors; demo: boolean }
  | { kind: "empty"; query: string };

export function ExplorePage() {
  const [query, setQuery] = useState("");
  const [state, setState] = useState<State>({ kind: "idle" });

  const run = useCallback(async (value: string, signal?: AbortSignal) => {
    const trimmed = value.trim();
    if (!trimmed) {
      setState({ kind: "idle" });
      return;
    }
    setState({ kind: "loading" });
    try {
      const [posts, users] = await Promise.all([
        request<SearchPostsResponse>(operations.searchPosts, { query: trimmed, limit: 20 }, signal),
        request<SearchUsersResponse>(operations.searchUsers, { query: trimmed, limit: 10 }, signal),
      ]);
      if (posts.searchPosts.length === 0 && users.searchUsers.length === 0) {
        setState({ kind: "empty", query: trimmed });
        return;
      }
      setState({ kind: "ready", posts: posts.searchPosts, users: users.searchUsers, demo: false });
    } catch (cause) {
      if (signal?.aborted) return;
      const matchedPosts = demoPosts.filter((post) =>
        post.content.toLowerCase().includes(trimmed.toLowerCase()),
      );
      const matchedUsers = demoAuthors.filter((author) =>
        author.username.includes(trimmed.toLowerCase()),
      );
      if (matchedPosts.length === 0 && matchedUsers.length === 0) {
        setState({ kind: "empty", query: trimmed });
        return;
      }
      setState({ kind: "ready", posts: matchedPosts, users: matchedUsers, demo: true });
      if (cause instanceof ApiError) setState({ kind: "ready", posts: matchedPosts, users: matchedUsers, demo: true });
    }
  }, []);

  // Debounced search: the field submits at most once per 250ms while typing.
  useEffect(() => {
    if (!query.trim()) return;
    const timer = window.setTimeout(() => void run(query), 250);
    return () => window.clearTimeout(timer);
  }, [query, run]);

  return (
    <AppShell
      activeID="explore"
      header={<h1 className="shell__title">Explore</h1>}
    >
      <SearchField
        id="explore-search"
        label="Search posts and people"
        value={query}
        onChange={setQuery}
        onSubmit={(value) => void run(value)}
        loading={state.kind === "loading"}
      />

      {state.kind === "idle" && (
        <div className="feed feed--empty">
          <h2 className="feed__title">Search the timeline</h2>
          <p className="feed__body">
            Keywords and #hashtags search post text; two letters or more suggests people.
          </p>
        </div>
      )}

      {state.kind === "loading" && (
        <ul className="feed__list" aria-label="Searching">
          {[0, 1, 2].map((index) => (
            <li key={index}>
              <PostCardSkeleton />
            </li>
          ))}
        </ul>
      )}

      {state.kind === "empty" && (
        <div className="feed feed--empty">
          <h2 className="feed__title">No matches for “{state.query}”</h2>
          <p className="feed__body">
            Nothing in the timeline mentions that. Try a broader keyword, or a #hashtag such as
            #systemdesign.
          </p>
        </div>
      )}

      {state.kind === "ready" && (
        <>
          {state.demo && (
            <p className="banner" role="status">
              The gateway is unreachable, so these are demo matches.
            </p>
          )}

          {state.users.length > 0 && (
            <section className="results" aria-labelledby="people-heading">
              <h2 className="results__heading" id="people-heading">
                People
              </h2>
              <ul className="results__people" role="list">
                {state.users.map((user) => (
                  <li key={user.id} className="results__person">
                    <span className="results__avatar" aria-hidden="true">
                      {user.name.charAt(0)}
                    </span>
                    <span className="results__identity">
                      <span className="results__name">{user.name}</span>
                      <span className="results__handle">@{user.username}</span>
                    </span>
                    <Link href={`#/u/${user.username}`} variant="quiet">
                      View profile
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          )}

          <FeedList
            ariaLabel="Search results"
            items={state.posts}
            keyFor={(post) => post.id}
            renderItem={(post) => <PostCard post={post} />}
            emptyTitle="Nothing here yet"
          />
        </>
      )}
    </AppShell>
  );
}

import { useCallback, useEffect, useMemo, useState } from "react";
import { AppShell } from "../components/AppShell";
import { Composer } from "../components/Composer";
import { FeedList } from "../components/FeedList";
import { PostCard, PostCardSkeleton } from "../components/PostCard";
import { TabPanel, TabSet } from "../components/Navigation";
import { ApiError, operations, request } from "../lib/api";
import type { FeedResponse, Post } from "../lib/api";
import { demoPosts } from "../lib/mock";
import { useSession } from "../state/session";

const TABS = [
  { id: "for-you", label: "For you" },
  { id: "following", label: "Following" },
  { id: "topics", label: "Topics" },
];

type State =
  | { kind: "loading" }
  | { kind: "ready"; posts: Post[]; source: "gateway" | "demo" }
  | { kind: "error"; message: string };

/* ==========================================================================
   Home — the feed.
   --------------------------------------------------------------------------
   This page is the reference implementation of the four list states, the
   composer's state matrix, and the optimistic like. Optimistic means the
   like appears immediately and reverts with an explanation if the gateway says
   no; it never lies for longer than one round trip.
   ========================================================================== */

export function HomePage() {
  const { signedIn, userID } = useSession();
  const [state, setState] = useState<State>({ kind: "loading" });
  const [tab, setTab] = useState("for-you");
  const [liked, setLiked] = useState<Record<string, boolean>>({});
  const [likeError, setLikeError] = useState<string>();

  const load = useCallback(async (signal?: AbortSignal) => {
    setState({ kind: "loading" });
    try {
      const data = await request<FeedResponse>(operations.feed, { limit: 20 }, signal);
      setState({ kind: "ready", posts: data.getFeed.posts, source: "gateway" });
    } catch (cause) {
      if (signal?.aborted) return;
      // A demo fallback keeps the page reviewable without the backend; the
      // banner says which one is on screen so nobody mistakes it for live data.
      setState({
        kind: "ready",
        posts: demoPosts,
        source: "demo",
      });
      setLikeError(
        cause instanceof ApiError ? `${cause.message} Showing demo posts instead.` : undefined,
      );
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const onLike = useCallback(
    async (postID: string, next: boolean) => {
      setLikeError(undefined);
      setLiked((current) => ({ ...current, [postID]: next }));
      try {
        await request(operations.likePost, {
          input: { postId: postID, userId: userID || "u-self" },
        });
      } catch (cause) {
        // Revert, and say why. A like that silently stays on is a lie.
        setLiked((current) => ({ ...current, [postID]: !next }));
        setLikeError(cause instanceof Error ? cause.message : "Could not save that like.");
      }
    },
    [userID],
  );

  const onCreate = useCallback(
    async (text: string) => {
      if (!signedIn) {
        throw new Error("Sign in before posting. Running on demo data.");
      }
      const data = await request<{ CreatePost: Post }>(operations.createPost, {
        input: { authorId: userID, content: text, mediaUrl: "", isPublic: true },
      });
      setState((current) =>
        current.kind === "ready"
          ? { ...current, posts: [data.CreatePost, ...current.posts] }
          : current,
      );
    },
    [signedIn, userID],
  );

  const posts = useMemo(
    () => (state.kind === "ready" ? state.posts : []),
    [state],
  );

  return (
    <AppShell
      activeID="home"
      header={
        <>
          <h1 className="shell__title">Home</h1>
          <TabSet items={TABS} activeID={tab} onChange={setTab} ariaLabel="Feed timeline" />
        </>
      }
    >
      {state.kind === "ready" && state.source === "demo" && (
        <p className="banner" role="status">
          {likeError ?? "The gateway is unreachable, so this feed is demo data."}
        </p>
      )}
      {likeError && posts.length > 0 && state.kind === "ready" && state.source === "gateway" && (
        <p className="banner banner--error" role="alert">
          {likeError}
        </p>
      )}

      <Composer
        onSubmit={onCreate}
        disabledReason={signedIn ? undefined : "Sign in to publish a post."}
      />

      <TabPanel id={tab} active>
        <FeedList
          ariaLabel="Posts"
          items={posts}
          loading={state.kind === "loading"}
          error={state.kind === "error" ? state.message : undefined}
          onRetry={() => void load()}
          keyFor={(post) => post.id}
          renderItem={(post) => (
            <PostCard
              post={post}
              liked={liked[post.id] ?? post.hasLiked}
              onLike={(postID, next) => void onLike(postID, next)}
              onReply={() => setTab("for-you")}
            />
          )}
          renderSkeleton={() => <PostCardSkeleton />}
          emptyTitle="Nothing here yet"
          emptyBody="Posts from the people you follow will land here. Follow an author to fill this timeline."
          emptyAction={
            <a className="btn btn--primary btn--md" href="#/explore">
              Find people to follow
            </a>
          }
        />
      </TabPanel>
    </AppShell>
  );
}

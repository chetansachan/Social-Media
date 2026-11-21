"use client";

import * as React from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  CalendarDays,
  Lock,
  MailPlus,
  UserX,
} from "lucide-react";
import { notify } from "@/lib/toast";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PostCard, PostSkeleton } from "@/components/feed/post-card";
import { absoluteDate } from "@/lib/format";
import { countUp } from "@/lib/motion";
import {
  useAuthorPosts,
  useFollowCounts,
  useFollowStatus,
  useLikedPosts,
  useProfile,
  useToggleFollow,
} from "@/lib/hooks/use-profile";
import { useStartConversation } from "@/lib/hooks/use-chat";
import { EditProfileDialog } from "@/components/profile/edit-profile-dialog";
import { useSession } from "@/lib/providers/session-provider";

export default function ProfilePage() {
  const params = useParams<{ username: string }>();
  const username = params?.username ?? "";
  const router = useRouter();
  const { user: viewer, status } = useSession();

  // The viewer already is the profile for their own handle: skip the user
  // request and let posts, counts, and likes start immediately.
  const selfByName = Boolean(viewer && viewer.username === username);
  const profile = useProfile(username, !selfByName);
  const person = selfByName ? (viewer ?? undefined) : profile.data;
  const posts = useAuthorPosts(person?.id);
  const counts = useFollowCounts(person?.id);
  const followStatus = useFollowStatus(person?.id);
  const follow = useToggleFollow(person?.id);
  const startConversation = useStartConversation();

  const [tab, setTab] = React.useState<"posts" | "likes">("posts");

  const isSelf = Boolean(viewer && person && viewer.id === person.id);
  const liked = useLikedPosts(person?.id, isSelf);

  if (!selfByName && profile.isLoading) {
    return (
      <>
        <div>
          <div className="sf-skeleton h-44 w-full rounded-none" aria-hidden="true" />
          <div className="px-4">
            <div
              className="sf-skeleton -mt-[68px] size-28 rounded-full ring-4 ring-page"
              aria-hidden="true"
            />
            <div className="mt-3 space-y-2" aria-hidden="true">
              <div className="sf-skeleton h-6 w-48 rounded" />
              <div className="sf-skeleton h-3 w-32 rounded" />
            </div>
          </div>
          <div className="mt-6">
            <PostSkeleton />
            <PostSkeleton />
          </div>
        </div>
      </>
    );
  }

  if (!person || (!selfByName && profile.error)) {
    return (
      <>
        <div className="px-8 py-20 text-center">
          <UserX className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
          <h2 className="mt-4 font-display text-3xl text-primary">
            @{username} wasn&rsquo;t found
          </h2>
          <p className="mx-auto mt-2 max-w-sm text-sm text-secondary">
            The handle may have changed, or the account no longer exists.
          </p>
          <Button asChild variant="secondary" size="md" className="mt-6">
            <Link href="/explore">Search for people</Link>
          </Button>
        </div>
      </>
    );
  }

  const isFollowing = followStatus.data?.isFollowing ?? false;
  const onMessage = () => {
    startConversation.mutate(person.id, {
      onSuccess: (conversation) => router.push(`/messages/${conversation.id}`),
      onError: (error) =>
        notify.error("Could not open the conversation", error.message),
    });
  };

  return (
    <>
      <div className="sticky top-0 z-[calc(var(--z-sticky)-1)] flex items-center gap-3 border-b border-line bg-page/85 px-3 py-2 backdrop-blur-xl">
        <Button
          variant="ghost"
          size="icon"
          aria-label="Back"
          onClick={() => router.back()}
        >
          <ArrowLeft className="size-icon-sm" aria-hidden="true" />
        </Button>
        <div className="min-w-0">
          <h2 className="truncate font-sans text-lg font-semibold text-primary">{person.name}</h2>
          <p className="font-mono text-xs text-tertiary">
            {posts.data?.length ?? 0} posts
          </p>
        </div>
      </div>

      <div
        aria-hidden="true"
        className="relative z-0 h-44 w-full overflow-hidden bg-surface sm:h-52"
      >
        <div className="absolute inset-0 bg-accent-gradient opacity-70" />
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_18%_20%,rgba(255,255,255,0.32),transparent_42%)]" />
        <div className="absolute inset-0 bg-page/15" />
        <div className="absolute -bottom-24 right-8 size-56 rounded-full bg-page/30 blur-3xl" />
      </div>

      <div className="relative z-[1] px-4">
        {/* Avatar and actions share a row, the way X lays a profile out: the
            name then sits immediately under the avatar rather than below a
            second row of its own.

            The negative margin belongs to the avatar alone -- it is the only
            element meant to overlap the cover. The actions sit in normal flow at
            the row's end, so they can never land on the cover, which is how the
            cover used to show through the secondary button's transparent
            background. */}
        <div className="flex items-end justify-between gap-3">
          <div className="-mt-[68px] sm:-mt-[76px]">
            <Avatar
              name={person.name}
              handle={person.username}
              size="2xl"
              className="ring-4 ring-page"
            />
          </div>

          <div className="mb-2 flex shrink-0 flex-wrap items-center justify-end gap-2">
            {isSelf ? (
              <EditProfileDialog user={person} />
            ) : status !== "authenticated" ? (
              <Button asChild size="md">
                <Link href="/login">Sign in to follow</Link>
              </Button>
            ) : (
              <>
                <Button
                  variant="secondary"
                  size="icon"
                  aria-label={`Message ${person.name}`}
                  onClick={onMessage}
                  loading={startConversation.isPending}
                >
                  <MailPlus className="size-icon-sm" aria-hidden="true" />
                </Button>
                <Button
                  variant={isFollowing ? "secondary" : "primary"}
                  size="md"
                  loading={follow.isPending}
                  aria-label={`${isFollowing ? "Unfollow" : "Follow"} ${person.name}`}
                  onClick={() =>
                    follow.mutate(!isFollowing, {
                      onSuccess: () =>
                        notify.success(
                          isFollowing
                            ? `Unfollowed ${person.name}`
                            : `Following ${person.name}`,
                        ),
                      onError: (error) =>
                        notify.error("Could not update follow", error.message),
                    })
                  }
                >
                  {isFollowing ? "Following" : "Follow"}
                </Button>
              </>
            )}
          </div>
        </div>

        <div className="mt-3">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="font-display text-4xl leading-none text-primary">
              {person.name}
            </h2>
            {!person.isPublic ? (
              <span className="inline-flex items-center gap-1 rounded-full border border-line px-2 py-0.5 font-mono text-2xs uppercase tracking-mono text-tertiary">
                <Lock className="size-3" aria-hidden="true" />
                Private
              </span>
            ) : null}
          </div>
          <p className="mt-1 font-mono text-sm text-tertiary">@{person.username}</p>

          <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-tertiary">
            <span className="inline-flex items-center gap-1.5">
              <CalendarDays className="size-icon-xs" aria-hidden="true" />
              Joined {absoluteDate(person.createdAt)}
            </span>
            <span className="inline-flex items-center gap-1.5 font-mono text-xs">
              SocialFlow
            </span>
          </div>

          <dl className="mt-4 flex items-center gap-6">
            <div>
              <dt className="sr-only">Posts</dt>
              <dd className="font-sans text-sm text-secondary">
                <AnimatedCount value={posts.data?.length ?? 0} />
                <span className="ml-1 text-tertiary">Posts</span>
              </dd>
            </div>
            <div>
              <dt className="sr-only">Following</dt>
              <dd className="font-sans text-sm text-secondary">
                <AnimatedCount value={counts.data?.followingCount ?? 0} />
                <span className="ml-1 text-tertiary">Following</span>
              </dd>
            </div>
            <div>
              <dt className="sr-only">Followers</dt>
              <dd className="font-sans text-sm text-secondary">
                <AnimatedCount value={counts.data?.followersCount ?? 0} />
                <span className="ml-1 text-tertiary">Followers</span>
              </dd>
            </div>
          </dl>
        </div>
      </div>

      <div className="mt-4">
        <Tabs
          value={tab}
          onValueChange={(value) => setTab(value as "posts" | "likes")}
        >
          <TabsList aria-label="Profile sections">
            <TabsTrigger value="posts">Posts</TabsTrigger>
            {isSelf ? <TabsTrigger value="likes">Likes</TabsTrigger> : null}
          </TabsList>
        </Tabs>
      </div>

      {tab === "posts" ? (
        posts.isLoading ? (
          <div>
            <PostSkeleton />
            <PostSkeleton />
          </div>
        ) : posts.data && posts.data.length > 0 ? (
          <div>
            {posts.data.map((post, index) => (
              <PostCard key={post.id} post={post} priority={index < 2} />
            ))}
          </div>
        ) : (
          <div className="px-8 py-16 text-center">
            <h4 className="font-display text-2xl text-primary">No posts yet</h4>
            <p className="mt-2 text-sm text-secondary">
              {isSelf
                ? "Your first post is one thought away."
                : `${person.name} hasn’t posted yet.`}
            </p>
            {isSelf ? (
              <Button asChild size="md" className="mt-5">
                <Link href="/">Write a post</Link>
              </Button>
            ) : null}
          </div>
        )
      ) : liked.isLoading ? (
        <div>
          <PostSkeleton />
          <PostSkeleton />
        </div>
      ) : liked.data && liked.data.length > 0 ? (
        <div>
          {liked.data.map((post, index) => (
            <PostCard key={post.id} post={post} priority={index < 2} />
          ))}
        </div>
      ) : (
        <div className="px-8 py-16 text-center">
          <h4 className="font-display text-2xl text-primary">No likes yet</h4>
          <p className="mt-2 text-sm text-secondary">
            Posts you like will show up here.
          </p>
        </div>
      )}
    </>
  );
}

/** A number that ticks up once when the data lands. Reduced motion is instant. */
function AnimatedCount({ value }: { value: number }) {
  const ref = React.useRef<HTMLSpanElement>(null);
  React.useEffect(() => {
    countUp(ref.current, value);
  }, [value]);
  return (
    <span ref={ref} className="font-sans font-semibold text-primary tabular-nums">
      {value.toLocaleString()}
    </span>
  );
}
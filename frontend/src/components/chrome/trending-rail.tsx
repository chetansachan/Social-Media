"use client";

import Link from "next/link";
import { ArrowRight, Hash, Search, TrendingUp } from "lucide-react";
import { useTrendingTags } from "@/lib/hooks/use-trending";
import { SearchBox } from "./search-box";

/**
 * The right rail.
 *
 * The backend has no trending endpoint, so this is explicitly scoped: it counts
 * hashtags in the viewer's own feed and labels itself "in your feed". That is
 * honest and useful; a global list would have to be invented.
 */
export function TrendingRail({ showSearch = true }: { showSearch?: boolean }) {
  const { tags, isLoading } = useTrendingTags(6);

  return (
    <div className="space-y-4">
      {showSearch ? (
        <div className="sticky top-2 z-[var(--z-sticky)] bg-page pb-1">
          <SearchBox />
        </div>
      ) : null}

      <section
        aria-labelledby="trending-heading"
        className="overflow-hidden rounded-lg bg-surface"
      >
        <h2
          id="trending-heading"
          className="flex items-center gap-2 px-4 pb-2 pt-4 font-sans text-pane font-semibold text-primary"
        >
          <TrendingUp className="size-icon-xs text-accent" aria-hidden="true" />
          Trending
          <span className="ml-auto font-mono text-2xs font-normal uppercase tracking-mono text-tertiary">
            in your feed
          </span>
        </h2>

        {isLoading ? (
          <div className="space-y-2 p-4" aria-hidden="true">
            <div className="sf-skeleton h-10 rounded-md" />
            <div className="sf-skeleton h-10 rounded-md" />
            <div className="sf-skeleton h-10 rounded-md" />
          </div>
        ) : tags.length === 0 ? (
          <p className="px-4 pb-5 pt-1 text-sm leading-relaxed text-secondary">
            Tags show up here once the people you follow post with hashtags.
          </p>
        ) : (
          <ul className="pb-2">
            {tags.map((trend) => (
              <li key={trend.tag}>
                <Link
                  href={`/explore?q=%23${trend.tag}`}
                  className="group flex items-start gap-3 px-4 py-3 transition-colors hover:bg-hover focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
                >
                  <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-full bg-inset text-tertiary transition-colors group-hover:text-accent">
                    <Hash className="size-icon-xs" aria-hidden="true" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-sans text-base font-medium text-primary">
                      #{trend.tag}
                    </span>
                    <span className="font-mono text-xs text-tertiary">
                      {trend.count} {trend.count === 1 ? "post" : "posts"}
                    </span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}

        <Link
          href="/explore"
          className="flex items-center justify-between px-4 py-4 text-base text-accent transition-colors hover:bg-hover focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
        >
          Show more
          <ArrowRight className="size-icon-xs" aria-hidden="true" />
        </Link>
      </section>

      <section className="rounded-lg bg-surface p-4">
        <h2 className="flex items-center gap-2 font-sans text-pane font-semibold text-primary">
          <Search className="size-icon-xs text-accent" aria-hidden="true" />
          Find your people
        </h2>
        <p className="mt-1.5 text-sm leading-relaxed text-secondary">
          Search by handle, name, or a tag like{" "}
          <span className="font-mono text-xs text-accent-alt">#postgres</span>. Follow
          the people whose work you want in your timeline.
        </p>
        <Link
          href="/explore"
          className="mt-4 inline-flex h-9 items-center gap-1.5 rounded-full bg-accent-solid px-4 text-sm font-semibold text-on-accent transition-colors hover:bg-accent-solid-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          Explore
          <ArrowRight className="size-icon-xs" aria-hidden="true" />
        </Link>
      </section>

      <footer className="px-2 pb-6">
        <p className="font-mono text-2xs leading-relaxed tracking-mono text-tertiary">
          SocialFlow · Go, gRPC, GraphQL
          <br />
          <Link href="/states" className="transition-colors hover:text-secondary">
            Component states
          </Link>
          {" · "}
          <Link href="/login" className="transition-colors hover:text-secondary">
            Sign in
          </Link>
        </p>
      </footer>
    </div>
  );
}
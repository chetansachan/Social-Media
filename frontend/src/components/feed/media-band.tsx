"use client";

import * as React from "react";
import { ImageOff, Play } from "lucide-react";
import { cn } from "@/lib/utils";
import { mediaKindOf } from "@/lib/media";
import type { MediaKind } from "@/lib/types";

/**
 * Media height cap. A post should not become a slideshow because one attachment
 * happens to be very tall, so anything taller than this is cropped into the
 * container rather than allowed to run down the feed.
 */
const MAX_MEDIA_HEIGHT = "max-h-[560px]";

/**
 * A post's media.
 *
 * A post carries exactly one `mediaUrl`, and the schema has no type field, so the
 * kind is inferred from the URL. Three renderers, because the kinds need
 * different elements:
 *
 *   image  an <img>.
 *   gif    an <img> too, but never next/image: it transcodes a GIF to a still
 *          first frame, which silently stops the animation the sender chose.
 *   video  <video controls>.
 *
 * Every one of them sizes itself from the media's own dimensions -- `w-full
 * h-auto` -- rather than being forced into a fixed box. That is what makes a
 * 4:5 portrait, a 16:9 wide shot and a vertical clip each occupy the space they
 * actually need, instead of every post reserving the same rectangle. `max-h`
 * caps only the outliers, and `object-cover` crops those so nothing distorts.
 *
 * next/image is deliberately not used here. It cannot size an unknown image
 * without known width/height, it cannot render a video at all, and it would
 * flatten a GIF. A plain element with intrinsic sizing is the one thing all
 * three kinds share.
 *
 * Text posts render nothing at all: `mediaUrl` is required-but-empty, and a
 * placeholder here would be inventing media the sender never attached.
 */
export function MediaBand({
  src,
  alt,
  priority = false,
  className,
}: {
  src: string;
  alt: string;
  priority?: boolean;
  className?: string;
}) {
  const kind: MediaKind | false = mediaKindOf(src);
  const [failed, setFailed] = React.useState(false);

  if (!kind) return null;

  if (failed) {
    return (
      <figure
        className={cn(
          "mt-3 overflow-hidden rounded-lg border border-line bg-inset",
          className,
        )}
      >
        <div className="grid aspect-[16/9] w-full place-items-center gap-2 p-4 text-center">
          {kind === "video" ? (
            <Play className="size-icon-md text-tertiary" aria-hidden="true" />
          ) : (
            <ImageOff className="size-icon-md text-tertiary" aria-hidden="true" />
          )}
          <p role="alert" className="text-xs text-tertiary">
            {kind === "video"
              ? "This video could not be loaded."
              : "This image could not be loaded."}
          </p>
        </div>
      </figure>
    );
  }

  return (
    <figure
      className={cn(
        "mt-3 overflow-hidden rounded-lg border border-line bg-inset",
        className,
      )}
    >
      {kind === "video" ? (
        <video
          src={src}
          controls
          // metadata, not auto: enough to expose a duration and the first
          // frame without pulling every byte of every video in the feed.
          preload="metadata"
          playsInline
          className={cn("block w-full bg-black", MAX_MEDIA_HEIGHT, "object-cover")}
          onError={() => setFailed(true)}
        >
          <track kind="captions" />
          Your browser cannot play this video.
        </video>
      ) : (
        // eslint-disable-next-line @next/next/no-img-element -- intrinsic sizing
        // is the point, and a GIF would lose its animation through next/image.
        <img
          src={src}
          alt={alt}
          loading={priority ? "eager" : "lazy"}
          decoding="async"
          className={cn("block w-full", MAX_MEDIA_HEIGHT, "object-cover")}
          onError={() => setFailed(true)}
        />
      )}
    </figure>
  );
}

/** A tombstone keeps the row so thread rhythm survives a deletion. */
export function Tombstone({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "border-b border-line px-[var(--space-card-x)] py-[var(--space-card-y)]",
        className,
      )}
    >
      <p className="text-sm italic text-tertiary">
        This post was removed by its author.
      </p>
    </div>
  );
}
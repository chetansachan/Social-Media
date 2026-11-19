"use client";

import * as React from "react";
import Link from "next/link";
import {
  ArrowUpRight,
  FileVideo,
  ImagePlus,
  PenLine,
  Smile,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Avatar } from "@/components/ui/avatar";
import { useSession } from "@/lib/providers/session-provider";
import { useMediaUpload } from "@/lib/hooks/use-media";
import { mediaKindOfFile, mediaLimitError } from "@/lib/media";
import type { MediaDraft, MediaKind } from "@/lib/types";
import type { PostDraft } from "@/lib/hooks/use-feed";

const MAX = 1000;

/** Which filter the file picker should apply. Each button owns one. */
type MediaChoice = "image" | "video" | "gif";

const ACCEPT: Record<MediaChoice, string> = {
  image: "image/png,image/jpeg,image/webp,image/avif",
  video: "video/mp4,video/webm,video/quicktime",
  gif: "image/gif",
};

/**
 * The composer.
 *
 * Signed out it explains itself instead of showing a dead textarea. Signed in,
 * the field grows with content up to a ceiling, the counter appears only when
 * it matters, and Command/Ctrl + Enter posts. The submit control is a real
 * button with a visible pending state, never an icon-only mystery.
 *
 * Media is attached, not linked: the file uploads to Cloudinary through a
 * signature post-service hands out, and the resulting URL is what the post
 * carries. That keeps the API secret server-side and the post's `media_url`
 * well inside its column.
 */
export function Composer({
  onSubmit,
  isPending,
  error,
  autoFocus = false,
  compact = false,
  placeholder = "What is happening?!",
  onSubmitted,
}: {
  onSubmit?: (draft: PostDraft) => void;
  isPending?: boolean;
  error?: string;
  autoFocus?: boolean;
  compact?: boolean;
  placeholder?: string;
  onSubmitted?: () => void;
}) {
  const { user, status } = useSession();
  const [value, setValue] = React.useState("");
  const [media, setMedia] = React.useState<MediaDraft | null>(null);
  const [mediaError, setMediaError] = React.useState<string | null>(null);
  const textareaRef = React.useRef<HTMLTextAreaElement>(null);
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const choiceRef = React.useRef<MediaChoice>("image");

  // A blob URL is a live handle on the picked file. It is revoked outside the
  // state updater -- React may run an updater twice in development, and a
  // revoked URL cannot be un-revoked, so the side effect does not belong there.
  const mediaRef = React.useRef<MediaDraft | null>(null);

  const upload = useMediaUpload();
  const isUploading = upload.isPending;

  const trimmed = value.trim();
  const remaining = MAX - value.length;
  const canSubmit = trimmed.length > 0 && remaining >= 0 && !isPending && !isUploading;
  const showCounter = remaining <= 180;

  // The only path that changes the attachment, so the ref and the state cannot
  // drift apart.
  const replaceMedia = React.useCallback((next: MediaDraft | null) => {
    if (mediaRef.current) URL.revokeObjectURL(mediaRef.current.previewUrl);
    mediaRef.current = next;
    setMedia(next);
  }, []);

  React.useEffect(() => {
    const node = textareaRef.current;
    if (!node) return;
    node.style.height = "auto";
    node.style.height = `${Math.min(node.scrollHeight, compact ? 320 : 240)}px`;
  }, [value, compact]);

  // The composer unmounts with the dialog, so the last blob URL goes with it.
  React.useEffect(
    () => () => {
      if (mediaRef.current) URL.revokeObjectURL(mediaRef.current.previewUrl);
      mediaRef.current = null;
    },
    [],
  );

  const clearMedia = React.useCallback(() => {
    setMediaError(null);
    replaceMedia(null);
  }, [replaceMedia]);

  const openPicker = React.useCallback((choice: MediaChoice) => {
    setMediaError(null);
    choiceRef.current = choice;
    // The input is re-pointed before it is clicked, so the accept filter
    // matches the button that was pressed rather than the previous one.
    const input = fileInputRef.current;
    if (input) input.accept = ACCEPT[choice];
    input?.click();
  }, []);

  const onFilePicked = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    // Clearing the value lets the same file be picked again after a removal.
    event.target.value = "";
    if (!file) return;

    // The kind is read from the file, never from the button that opened the
    // picker: a user can pick an MP4 from the image picker on some platforms.
    const kind: MediaKind | false = mediaKindOfFile(file);
    if (!kind) {
      setMediaError(`${file.name || "That file"} is not an image, GIF, or video.`);
      return;
    }
    const limitProblem = mediaLimitError(file, kind);
    if (limitProblem) {
      setMediaError(limitProblem);
      return;
    }

    setMediaError(null);
    replaceMedia({ previewUrl: URL.createObjectURL(file), kind, file });
  };

  const submit = async (event?: React.FormEvent) => {
    event?.preventDefault();
    if (!canSubmit) return;

    // The upload happens here rather than on pick, so an abandoned draft never
    // costs a round trip. A failure keeps the text and the attachment so the
    // author can retry without retyping.
    let mediaUrl: string | undefined;
    if (media) {
      try {
        mediaUrl = await upload.mutateAsync({ file: media.file, kind: media.kind });
      } catch (uploadError) {
        setMediaError(
          uploadError instanceof Error ? uploadError.message : "The upload failed.",
        );
        return;
      }
    }

    onSubmit?.({ content: trimmed, mediaUrl });
    setValue("");
    clearMedia();
    onSubmitted?.();
  };

  if (status !== "authenticated" || !user) {
    return (
      <div className="mx-4 my-4 rounded-lg border border-line bg-surface px-6 py-8">
        <span className="grid size-11 place-items-center rounded-full bg-accent-soft text-accent">
          <PenLine className="size-icon-md" aria-hidden="true" />
        </span>
        <h2 className="mt-4 font-display text-2xl text-primary">
          Join the conversation
        </h2>
        <p className="mt-1 max-w-sm text-sm leading-relaxed text-secondary">
          Sign in to post, reply, like, follow, and send messages.
        </p>
        <div className="mt-5 flex flex-wrap items-center gap-2">
          <Button asChild size="md">
            <Link href="/login">
              Sign in
              <ArrowUpRight className="size-icon-xs" aria-hidden="true" />
            </Link>
          </Button>
          <Button asChild variant="secondary" size="md">
            <Link href="/explore">Explore the feed</Link>
          </Button>
        </div>
      </div>
    );
  }

  const busy = isPending || isUploading;

  return (
    <div
      className={cn(
        "bg-page transition-colors duration-200",
        compact ? "px-4 pb-4 pt-1" : "px-4 py-2.5",
        !compact && "border-b border-line",
      )}
    >
      <form
        onSubmit={(event) => {
          void submit(event);
        }}
      >
        <div className="flex gap-3">
          <Link
            href={`/profile/${user.username}`}
            aria-label="Your profile"
            className="h-fit shrink-0 rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            <Avatar name={user.name} handle={user.username} size="md" />
          </Link>

          <div className="min-w-0 flex-1">
            <label htmlFor={compact ? "composer-modal" : "composer"} className="sr-only">
              What is happening?
            </label>
            <textarea
              id={compact ? "composer-modal" : "composer"}
              ref={textareaRef}
              value={value}
              autoFocus={autoFocus}
              onChange={(event) => setValue(event.target.value)}
              onKeyDown={(event) => {
                if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
                  event.preventDefault();
                  void submit();
                }
                if (event.key === "Escape" && compact) {
                  setValue("");
                }
              }}
              placeholder={placeholder}
              rows={compact ? 3 : 2}
              aria-describedby="composer-count"
              className={cn(
                "w-full resize-none border-0 bg-transparent pt-1 font-sans text-xl leading-snug text-primary",
                "placeholder:text-tertiary focus:outline-none",
              )}
              maxLength={MAX + 50}
            />

            <AttachmentStrip
              media={media}
              onRemove={clearMedia}
              error={mediaError}
              isUploading={isUploading}
            />

            <div className="mt-2 flex items-center gap-1 border-t border-line pt-3">
              <input
                ref={fileInputRef}
                type="file"
                accept={ACCEPT.image}
                onChange={onFilePicked}
                className="hidden"
                aria-hidden="true"
                tabIndex={-1}
              />

              {/* One control per kind. A GIF is an image under every hood, but
                  the sender asked for it, so it gets its own entry rather than
                  being buried in the image picker. */}
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Attach an image"
                title="Image"
                disabled={busy || Boolean(media)}
                onClick={() => openPicker("image")}
              >
                <ImagePlus className="size-icon-md" aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Attach a GIF"
                title="GIF"
                disabled={busy || Boolean(media)}
                onClick={() => openPicker("gif")}
              >
                <Smile className="size-icon-md" aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label="Attach a video"
                title="Video"
                disabled={busy || Boolean(media)}
                onClick={() => openPicker("video")}
              >
                <FileVideo className="size-icon-md" aria-hidden="true" />
              </Button>

              <span className="hidden font-mono text-2xs tracking-mono text-tertiary sm:inline">
                ⌘ ↵ to post
              </span>

              <span
                id="composer-count"
                aria-live="polite"
                className={cn(
                  "ml-auto font-mono text-xs tabular-nums tracking-mono",
                  remaining < 0
                    ? "font-semibold text-danger"
                    : remaining < 60
                      ? "text-accent-alt"
                      : "text-tertiary",
                )}
              >
                {remaining < 0 ? remaining : showCounter ? remaining : ""}
              </span>

              <Button
                type="submit"
                size="sm"
                disabled={!canSubmit}
                loading={busy}
                className="min-w-[76px] px-5"
              >
                {isUploading ? "Uploading" : "Post"}
              </Button>
            </div>

            {error ? (
              <p role="alert" className="mt-2 text-xs text-danger">
                {error}
              </p>
            ) : null}
          </div>
        </div>
      </form>
    </div>
  );
}

/**
 * The pending attachment, with its own error and remove control.
 *
 * A preview is a blob URL, so it shows what was actually picked — not a
 * thumbnail the sender has to trust. Only one attachment is possible, because a
 * post carries one `mediaUrl`.
 */
function AttachmentStrip({
  media,
  onRemove,
  error,
  isUploading,
}: {
  media: MediaDraft | null;
  onRemove: () => void;
  error: string | null;
  isUploading: boolean;
}) {
  if (!media && !error) return null;

  return (
    <div className="mt-2">
      {media ? (
        <div className="relative overflow-hidden rounded-lg border border-line bg-inset">
          {/* The preview keeps the file's own proportions, so the sender sees
              the shape everyone else will see. */}
          {media.kind === "video" ? (
            <video
              src={media.previewUrl}
              controls
              preload="metadata"
              playsInline
              className="block max-h-[320px] w-full object-contain"
            />
            ) : (
              // A blob URL is same-origin and short-lived, so a plain <img> is
              // the honest element here: next/image cannot optimise it.
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={media.previewUrl}
                alt={`Selected ${media.kind === "gif" ? "GIF" : media.kind}`}
                className="block max-h-[320px] w-full object-contain"
              />
            )}

          <div className="absolute inset-x-0 bottom-0 flex items-center gap-2 bg-page/80 px-3 py-1.5 backdrop-blur-sm">
            <span className="font-mono text-2xs uppercase tracking-mono text-secondary">
              {media.kind === "gif" ? "GIF" : media.kind}
            </span>
            <span className="truncate text-2xs text-tertiary">
              {(media.file.size / (1024 * 1024)).toFixed(1)} MB
            </span>
            <button
              type="button"
              onClick={onRemove}
              aria-label="Remove attachment"
              className="ml-auto grid size-6 shrink-0 place-items-center rounded-full text-secondary transition-colors hover:bg-hover hover:text-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
            >
              <X className="size-4" aria-hidden="true" />
            </button>
          </div>
        </div>
      ) : null}

      {isUploading ? (
        <p className="mt-1.5 text-xs text-tertiary">Uploading your media…</p>
      ) : null}

      {error ? (
        <p role="alert" className="mt-1.5 text-xs text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
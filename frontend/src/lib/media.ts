/* ==========================================================================
   Post media.
   --------------------------------------------------------------------------
   A post carries exactly one `mediaUrl`. The schema has no media-type field and
   the posts table has no multi-attachment column, so the kind is inferred from
   the URL and a post holds one item. This file owns both halves: inferring a
   kind for rendering, and turning a picked file into a stored URL.

   Uploads go straight to Cloudinary. post-service hands out a signature for
   `folder`, `public_id` and `timestamp` only, and the API secret stays
   server-side; the browser posts the file itself to `uploadUrl` and gets a
   secure URL back, which is small enough for the `media_url VARCHAR(500)` column.
   ========================================================================== */

import type { MediaKind, UploadSignature } from "./types";

/** Cloudinary signs these three, and nothing else may be added to the request. */
const SIGNED_PARAMS = ["folder", "public_id", "timestamp"] as const;

/** Extensions Cloudinary and browsers both treat as video. */
const VIDEO_EXTENSIONS = [
  "mp4",
  "webm",
  "mov",
  "m4v",
  "mkv",
  "avi",
  "mpeg",
  "mpg",
  "ogv",
  "3gp",
] as const;

/** Extensions that name a still image. GIF is separate: it is an image, but it
    is the one image kind that animates, so it gets its own label. */
const STILL_EXTENSIONS = [
  "jpg",
  "jpeg",
  "png",
  "webp",
  "avif",
  "bmp",
  "svg",
  "heic",
  "heif",
] as const;

/**
 * Reads the extension off a URL's path, ignoring any query string.
 *
 * Cloudinary delivers transformations in the path (`/w_600/v123/…`), so the
 * extension is not necessarily the last path segment's; it is still the last
 * segment once the query is stripped.
 */
function extensionOf(rawUrl: string): string {
  try {
    const pathname = new URL(rawUrl).pathname;
    const last = pathname.slice(pathname.lastIndexOf("/") + 1);
    const dot = last.lastIndexOf(".");
    if (dot === -1) return "";
    return last.slice(dot + 1).toLowerCase();
  } catch {
    return "";
  }
}

/**
 * The Cloudinary resource type in the URL's path, when there is one.
 * `/image/upload/…` and `/video/upload/…` are the two that matter.
 */
function cloudinaryResource(rawUrl: string): string {
  const match = /:\/\/res\.cloudinary\.com\/[^/]+\/([^/]+)\//.exec(rawUrl);
  return match?.[1]?.toLowerCase() ?? "";
}

/**
 * Which kind a media URL is.
 *
 * A heuristic, deliberately so: there is no type field in the schema, so the
 * renderer has to guess from what the URL says about itself. The order is
 * specific-to-general — an explicit video path beats an extension, which beats
 * the still-image list — so unknown hosts fall back to image rather than
 * rendering a broken video element.
 *
 * Returns false when there is no media to render, which is how a text post
 * (`mediaUrl: ""`) tells the feed to stand alone.
 */
export function mediaKindOf(mediaUrl: string | null | undefined): MediaKind | false {
  const url = (mediaUrl ?? "").trim();
  if (!url) return false;

  const extension = extensionOf(url);
  const resource = cloudinaryResource(url);

  if (resource === "video") return "video";

  // A GIF is an image that animates. Cloudinary marks animated delivery either
  // in the path or with `f_gif`, so both are caught before the still check.
  if (extension === "gif" || /\/animated\b/.test(url) || /[?&]f=gif\b/i.test(url)) {
    return "gif";
  }

  if (extension && (VIDEO_EXTENSIONS as readonly string[]).includes(extension)) {
    return "video";
  }

  if (extension && (STILL_EXTENSIONS as readonly string[]).includes(extension)) {
    return "image";
  }

  // An empty extension with no recognisable path is almost always an image
  // behind a CDN that hides its name. Rendering <img> degrades to a broken
  // image with a ratio preserved; <video> would render nothing at all.
  return "image";
}

/** The Cloudinary resource type a file should be uploaded as. */
export function resourceTypeFor(kind: MediaKind): "image" | "video" {
  return kind === "video" ? "video" : "image";
}

/** Caps that keep a picked file inside what a post should carry. */
export const MEDIA_LIMITS = {
  /** A GIF is a video in disguise: the same budget, different renderer. */
  imageBytes: 10 * 1024 * 1024,
  videoBytes: 64 * 1024 * 1024,
} as const;

/**
 * The kind a picked file is, from its MIME type.
 *
 * A GIF is reported as `gif` rather than `image` so the composer can label it
 * and the feed can pick the right element without re-reading the bytes.
 */
export function mediaKindOfFile(file: File): MediaKind | false {
  const type = file.type.toLowerCase();
  if (!type) return false;
  if (type === "image/gif") return "gif";
  if (type.startsWith("image/")) return "image";
  if (type.startsWith("video/")) return "video";
  return false;
}

/** Human-readable rejection reasons, so the composer can show one string. */
export function mediaLimitError(file: File, kind: MediaKind): string | null {
  const limit = kind === "video" ? MEDIA_LIMITS.videoBytes : MEDIA_LIMITS.imageBytes;
  if (file.size > limit) {
    const megabytes = Math.round(limit / (1024 * 1024));
    return `That file is ${(file.size / (1024 * 1024)).toFixed(1)} MB. ${kind === "video" ? "Video" : "Images"} are limited to ${megabytes} MB.`;
  }
  return null;
}

/**
 * Posts a file directly to Cloudinary and returns its secure URL.
 *
 * The request carries exactly the three signed parameters, plus `api_key`,
 * `signature` and the file. Cloudinary excludes `api_key`, `resource_type` and
 * the file from signature checking, so adding a fourth parameter — a width, an
 * eager transformation — would invalidate it and 400.
 */
export async function uploadToCloudinary(
  file: File,
  kind: MediaKind,
  signature: UploadSignature,
): Promise<string> {
  const form = new FormData();
  form.append("file", file);
  form.append("folder", signature.folder);
  form.append("public_id", signature.publicId);
  form.append("timestamp", signature.timestamp);
  form.append("api_key", signature.apiKey);
  form.append("signature", signature.signature);

  const response = await fetch(signature.uploadUrl, {
    method: "POST",
    body: form,
  });

  if (!response.ok) {
    // Cloudinary reports its own reason; surface it rather than a generic
    // "upload failed", because the common cause is an expired signature.
    const detail = await response.text().catch(() => "");
    const reason = detail ? ` ${detail.slice(0, 180)}` : "";
    throw new Error(`Cloudinary rejected the upload (${response.status}).${reason}`);
  }

  const body = (await response.json().catch(() => null)) as {
    secure_url?: string;
    url?: string;
  } | null;

  const url = body?.secure_url || body?.url;
  if (!url) throw new Error("Cloudinary returned no URL for the upload.");
  return url;
}

/**
 * Reads a signature's age from its own `expiresAt`, so an expired one can be
 * refetched instead of failing on Cloudinary's side.
 *
 * `expiresAt` is an ISO string; a signature past it has already 400'd.
 */
export function isSignatureExpired(signature: UploadSignature): boolean {
  const expiry = Date.parse(signature.expiresAt);
  if (Number.isNaN(expiry)) return false;
  // Ten seconds of slack: a signature about to expire mid-upload is as good as
  // expired, because Cloudinary checks at completion time.
  return Date.now() >= expiry - 10_000;
}

export { SIGNED_PARAMS };
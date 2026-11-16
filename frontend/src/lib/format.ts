/* ==========================================================================
   Formatting — shared by every screen so wording never drifts.
   ========================================================================== */

/** Relative time. Deterministic given a reference instant, so it is testable. */
export function relativeTime(iso: string, now: number = Date.now()): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "";
  const seconds = Math.max(0, Math.round((now - then) / 1000));
  if (seconds < 45) return "now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.round(hours / 24);
  if (days < 7) return `${days}d`;
  const weeks = Math.round(days / 7);
  if (weeks < 5) return `${weeks}w`;
  const months = Math.round(days / 30);
  if (months < 12) return `${months}mo`;
  return `${Math.round(days / 365)}y`;
}

/** Long form, for screen readers and profile metadata. */
export function absoluteDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "long",
    day: "numeric",
  });
}

/** Compact count: 1.2K, 4.5M. */
export function compactCount(value: number): string {
  if (!Number.isFinite(value)) return "0";
  if (value < 1000) return String(value);
  if (value < 1_000_000) {
    const n = value / 1000;
    return `${n < 10 ? n.toFixed(1).replace(/\.0$/, "") : Math.round(n)}K`;
  }
  const n = value / 1_000_000;
  return `${n < 10 ? n.toFixed(1).replace(/\.0$/, "") : Math.round(n)}M`;
}

/**
 * Splits post content into a display headline and the remaining body.
 *
 * The API returns one `content` string; the card wants a Newsreader headline
 * and Inter body. This is a presentation-only transform: it never rewrites the
 * text, it only decides where the split falls. The first sentence (ending at
 * `.`, `!` or `?` followed by whitespace) is the headline when it is short
 * enough to be one; otherwise the headline is the first 120 characters cut on
 * a word boundary and the body keeps everything.
 */
export function splitHeadline(content: string): {
  headline: string;
  body: string;
} {
  const text = content.trim();
  if (!text) return { headline: "", body: "" };

  const MAX_HEADLINE = 120;
  const sentenceEnd = /[.!?](?=\s)/.exec(text);

  if (sentenceEnd) {
    const end = sentenceEnd.index + 1;
    if (end <= MAX_HEADLINE) {
      return {
        headline: text.slice(0, end),
        body: text.slice(end).trim(),
      };
    }
  }

  if (text.length <= MAX_HEADLINE) {
    return { headline: text, body: "" };
  }

  // Cut on the last word boundary inside the budget so no word is split.
  const window = text.slice(0, MAX_HEADLINE);
  const lastSpace = window.lastIndexOf(" ");
  const cut = lastSpace > MAX_HEADLINE * 0.6 ? lastSpace : MAX_HEADLINE;

  return {
    headline: text.slice(0, cut).trimEnd(),
    body: text.slice(cut).trim(),
  };
}

/** Extracts #hashtags without duplicates, in order of appearance. */
export function extractTags(content: string): string[] {
  const matches = content.match(/#[\p{L}\p{N}_]+/gu);
  if (!matches) return [];
  return [...new Set(matches)];
}

/**
 * Splits text into plain and hashtag segments so a renderer can style the tags
 * inline.
 *
 * This is why there is no separate chip row: the tags are already in the text,
 * so rendering them again as chips shows every tag twice. Styling them where
 * they already are keeps the text exactly as written — nothing is stripped,
 * moved, or duplicated.
 */
export type TextSegment = { text: string; isTag: boolean };

export function segmentTags(content: string): TextSegment[] {
  const segments: TextSegment[] = [];
  const pattern = /#[\p{L}\p{N}_]+/gu;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = pattern.exec(content)) !== null) {
    if (match.index > lastIndex) {
      segments.push({ text: content.slice(lastIndex, match.index), isTag: false });
    }
    segments.push({ text: match[0], isTag: true });
    lastIndex = match.index + match[0].length;
  }

  if (lastIndex < content.length) {
    segments.push({ text: content.slice(lastIndex), isTag: false });
  }

  return segments;
}

/** Initials for an avatar fallback. */
export function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0]!.slice(0, 2).toUpperCase();
  return (parts[0]![0]! + parts[parts.length - 1]![0]!).toUpperCase();
}

/** A stable hue per handle, so avatars are distinguishable without photos. */
export function handleHue(handle: string): number {
  let hash = 0;
  for (let i = 0; i < handle.length; i += 1) {
    hash = (hash * 31 + handle.charCodeAt(i)) % 360;
  }
  return hash;
}

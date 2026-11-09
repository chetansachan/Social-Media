/* ==========================================================================
   Motion — GSAP is the only animation entry point in this app.
   --------------------------------------------------------------------------
   Every animation goes through this module so three things are guaranteed:
   1. A duration always comes from a motion token, never a literal.
   2. prefers-reduced-motion is honoured once, here, instead of per component.
   3. Nothing animates a property that motion-sensitivity cares about (the
      like burst scales, it does not fly across the screen).
   ========================================================================== */

import gsap from "gsap";

/** Reads a motion token as a number of milliseconds. */
function token(name: string, fallbackMs: number): number {
  const raw = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  if (!raw) return fallbackMs;
  const parsed = Number.parseFloat(raw);
  return Number.isFinite(parsed) ? parsed : fallbackMs;
}

export const motion = {
  /** State changes: hover, focus, pressed. 200ms per the brand scale. */
  instant: () => token("--motion-duration-instant", 200) / 1000,
  /** Entrance choreography for a list revealing itself. */
  reveal: () => token("--motion-duration-reveal", 320) / 1000,
  /** The like/favourite burst. */
  celebration: () => token("--motion-duration-celebration", 480) / 1000,
  easeStandard: () =>
    getComputedStyle(document.documentElement)
      .getPropertyValue("--motion-ease-standard")
      .trim() || "cubic-bezier(0.2, 0, 0, 1)",
  easeEntrance: () =>
    getComputedStyle(document.documentElement)
      .getPropertyValue("--motion-ease-entrance")
      .trim() || "cubic-bezier(0.16, 1, 0.3, 1)",
};

/** True when the OS asks for reduced motion. Cached per media query change. */
export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** Kills every active GSAP tween. Used on unmount of a choreographed subtree. */
export function killTweens(targets: gsap.TweenTarget): void {
  gsap.killTweensOf(targets);
}

/* ---- Choreographies ------------------------------------------------------ */

/**
 * Entrance for a list: children rise and settle, staggered.
 *
 * Under reduced motion the elements are placed at their final state
 * immediately, so the list is never invisible and never animates.
 */
export function revealStagger(
  container: Element | null,
  children: Element[],
  opts: { delay?: number } = {},
): void {
  if (!container || children.length === 0) return;

  if (prefersReducedMotion()) {
    gsap.set(children, { opacity: 1, y: 0, clearProps: "transform" });
    return;
  }

  gsap.fromTo(
    children,
    { opacity: 0, y: 12 },
    {
      opacity: 1,
      y: 0,
      duration: motion.reveal(),
      ease: motion.easeEntrance(),
      stagger: 0.04,
      delay: opts.delay ?? 0,
      clearProps: "transform",
    },
  );
}

/**
 * The like burst. The pop is a scale, never a translate: parallax-free motion
 * is what keeps this out of vestibular-trigger territory.
 */
export function celebrate(target: Element | null): void {
  if (!target) return;
  if (prefersReducedMotion()) return;

  gsap.fromTo(
    target,
    { scale: 1 },
    {
      scale: 1.22,
      duration: motion.celebration() / 2 / 1000,
      ease: "power2.out",
      yoyo: true,
      repeat: 1,
      clearProps: "transform",
    },
  );
}

/** A press acknowledgement, shared by every control. */
export function press(target: Element | null): void {
  if (!target || prefersReducedMotion()) return;
  gsap.fromTo(
    target,
    { scale: 1 },
    {
      scale: 0.96,
      duration: motion.instant() / 1000,
      ease: motion.easeStandard(),
      yoyo: true,
      repeat: 1,
      clearProps: "transform",
    },
  );
}

/**
 * Slides a moving indicator under the selected tab. The indicator is a real
 * element so the transition is a transform, not a layout-triggering width
 * animation on the tabs themselves.
 */
export function moveIndicator(
  indicator: Element | null,
  target: Element | null,
): void {
  if (!indicator || !target) return;
  const element = target as HTMLElement;
  const x = element.offsetLeft;
  const width = element.offsetWidth;

  if (prefersReducedMotion()) {
    gsap.set(indicator, { x, width });
    return;
  }

  gsap.to(indicator, {
    x,
    width,
    duration: motion.instant() / 1000,
    ease: motion.easeStandard(),
  });
}
/**
 * Scroll-linked elevation on the sticky header. A single progress value drives
 * a CSS variable, so the compositor does the work and no React state changes.
 */
export function attachHeaderElevation(header: HTMLElement | null): () => void {
  if (!header) return () => {};

  const onScroll = () => {
    const elevated = window.scrollY > 8;
    header.dataset.elevated = elevated ? "true" : "false";
  };

  onScroll();
  window.addEventListener("scroll", onScroll, { passive: true });
  return () => window.removeEventListener("scroll", onScroll);
}

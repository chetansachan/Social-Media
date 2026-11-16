/* ==========================================================================
   Motion — the only module in the app that touches GSAP.
   --------------------------------------------------------------------------
   Three guarantees come from funnelling every animation through here:
   1. Durations come from motion tokens, never literals.
   2. prefers-reduced-motion is honoured once, here, not per component.
   3. Nothing animates a property that triggers vestibular discomfort — the
      like burst scales, it does not fly across the screen.

   GSAP is loaded lazily, on the first animation that actually runs. It is
   ~50 kB gzipped and nothing above the fold needs it before interaction, so
   keeping it out of the initial chunk is the difference between a 210 kB and a
   160 kB first load on `/`.
   ========================================================================== */

type GsapModule = typeof import("gsap");

let gsapPromise: Promise<GsapModule["default"]> | null = null;

function loadGsap(): Promise<GsapModule["default"]> {
  if (!gsapPromise) {
    gsapPromise = import("gsap").then((module) => module.default);
  }
  return gsapPromise;
}

function token(name: string, fallbackMs: number): number {
  if (typeof window === "undefined") return fallbackMs;
  const raw = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  const parsed = Number.parseFloat(raw);
  return Number.isFinite(parsed) ? parsed : fallbackMs;
}

export const motion = {
  /** Seconds, for GSAP. */
  instant: () => token("--motion-duration-instant", 200) / 1000,
  reveal: () => token("--motion-duration-reveal", 320) / 1000,
  celebration: () => token("--motion-duration-celebration", 480) / 1000,
  easeStandard: () =>
    (typeof window !== "undefined" &&
      getComputedStyle(document.documentElement)
        .getPropertyValue("--motion-ease-standard")
        .trim()) ||
    "cubic-bezier(0.2, 0, 0, 1)",
  easeEntrance: () =>
    (typeof window !== "undefined" &&
      getComputedStyle(document.documentElement)
        .getPropertyValue("--motion-ease-entrance")
        .trim()) ||
    "cubic-bezier(0.16, 1, 0.3, 1)",
};

export function prefersReducedMotion(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/* ---- Choreographies ------------------------------------------------------ */

/**
 * Entrance for a list: children rise and settle, staggered.
 * Under reduced motion they are placed at their final state immediately, so
 * nothing is ever left invisible (V10).
 */
export function revealStagger(
  container: Element | null,
  children: Element[],
  opts: { delay?: number } = {},
): void {
  if (!container || children.length === 0) return;

  if (prefersReducedMotion()) {
    for (const child of children) {
      (child as HTMLElement).style.opacity = "1";
      (child as HTMLElement).style.transform = "none";
    }
    return;
  }

  void loadGsap().then((gsap) => {
    gsap.fromTo(
      children,
      { opacity: 0, y: 14 },
      {
        opacity: 1,
        y: 0,
        duration: motion.reveal(),
        ease: motion.easeEntrance(),
        stagger: 0.045,
        delay: opts.delay ?? 0,
        clearProps: "transform",
      },
    );
  });
}

/** The like burst: a scale pop, never a translate. */
export function celebrate(target: Element | null): void {
  if (!target || prefersReducedMotion()) return;
  void loadGsap().then((gsap) => {
    gsap.fromTo(
      target,
      { scale: 1 },
      {
        scale: 1.28,
        duration: motion.celebration() / 2,
        ease: "power2.out",
        yoyo: true,
        repeat: 1,
        clearProps: "transform",
      },
    );
  });
}

/** A press acknowledgement, shared by every control. */
export function press(target: Element | null): void {
  if (!target || prefersReducedMotion()) return;
  void loadGsap().then((gsap) => {
    gsap.fromTo(
      target,
      { scale: 1 },
      {
        scale: 0.96,
        duration: motion.instant() / 2,
        ease: motion.easeStandard(),
        yoyo: true,
        repeat: 1,
        clearProps: "transform",
      },
    );
  });
}

/**
 * Slides a moving indicator under the selected tab. The indicator is a real
 * element, so the transition is a transform rather than a layout-triggering
 * width animation on the tabs themselves.
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
    (indicator as HTMLElement).style.transform = `translateX(${x}px)`;
    (indicator as HTMLElement).style.width = `${width}px`;
    return;
  }

  void loadGsap().then((gsap) => {
    gsap.to(indicator, {
      x,
      width,
      duration: motion.instant(),
      ease: motion.easeStandard(),
    });
  });
}

/** A count that ticks up on first paint, for the profile stats. */
export function countUp(target: Element | null, to: number): void {
  if (!target) return;
  const node = target as HTMLElement;
  if (prefersReducedMotion()) {
    node.textContent = to.toLocaleString();
    return;
  }
  void loadGsap().then((gsap) => {
    const state = { value: 0 };
    gsap.to(state, {
      value: to,
      duration: motion.reveal() * 2,
      ease: motion.easeStandard(),
      onUpdate: () => {
        node.textContent = Math.round(state.value).toLocaleString();
      },
    });
  });
}

/**
 * Scroll-linked elevation on the sticky header. A single boolean drives a data
 * attribute, so the compositor does the work and no React state changes. This
 * one needs no GSAP at all.
 */
export function attachHeaderElevation(header: HTMLElement | null): () => void {
  if (!header) return () => {};
  const onScroll = () => {
    header.dataset.elevated = window.scrollY > 8 ? "true" : "false";
  };
  onScroll();
  window.addEventListener("scroll", onScroll, { passive: true });
  return () => window.removeEventListener("scroll", onScroll);
}

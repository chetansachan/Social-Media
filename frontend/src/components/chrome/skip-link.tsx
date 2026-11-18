import { cn } from "@/lib/utils";

/** First tab stop on every page. Visually hidden until focused. */
export function SkipLink({ target = "#main" }: { target?: string }) {
  return (
    <a
      href={target}
      className={cn(
        "sr-only focus:not-sr-only",
        "focus:fixed focus:left-4 focus:top-4 focus:z-[var(--z-toast)]",
        "focus:rounded-md focus:bg-accent-solid focus:px-4 focus:py-2",
        "focus:text-sm focus:font-medium focus:text-on-accent focus:shadow-2",
      )}
    >
      Skip to content
    </a>
  );
}
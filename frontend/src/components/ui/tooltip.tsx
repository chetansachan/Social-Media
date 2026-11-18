"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * A tooltip with no runtime dependency.
 *
 * The Radix tooltip is ~12 kB gzipped and the only thing this app needs from it
 * is "show a label on hover and on keyboard focus". That is a wrapper span and
 * two CSS rules, so the dependency was dropped from the initial chunk rather
 * than paid for on every route.
 *
 * The label is duplicated into the trigger's aria-label by the caller, so the
 * tooltip is an enhancement: it is `aria-hidden` and never the only place a
 * name exists (A4).
 */
export function Tooltip({
  children,
  label,
  side = "right",
  className,
}: {
  children: React.ReactNode;
  label: string;
  side?: "top" | "right" | "bottom" | "left";
  className?: string;
}) {
  const position =
    side === "right"
      ? "left-full top-1/2 ml-2 -translate-y-1/2"
      : side === "left"
        ? "right-full top-1/2 mr-2 -translate-y-1/2"
        : side === "top"
          ? "bottom-full left-1/2 mb-2 -translate-x-1/2"
          : "top-full left-1/2 mt-2 -translate-x-1/2";

  return (
    <span className={cn("group/tooltip relative inline-flex", className)}>
      {children}
      <span
        aria-hidden="true"
        role="presentation"
        className={cn(
          "pointer-events-none absolute z-[var(--z-overlay)] whitespace-nowrap",
          "rounded-md border border-line bg-overlay px-2.5 py-1.5 shadow-2",
          "font-mono text-2xs tracking-mono text-primary",
          "opacity-0 transition-opacity duration-150",
          "group-hover/tooltip:opacity-100 group-focus-within/tooltip:opacity-100",
          position,
        )}
      >
        {label}
      </span>
    </span>
  );
}
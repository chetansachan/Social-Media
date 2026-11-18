"use client";

import * as React from "react";
import { Moon, Sun } from "lucide-react";
import { useTheme } from "@/lib/providers/theme-provider";
import { cn } from "@/lib/utils";

/**
 * The theme switch.
 *
 * aria-pressed carries the state, so the change is perceivable without relying
 * on colour (A16). The icon swap is instant rather than animated, which keeps
 * A17 satisfied without a special case.
 */
export function ThemeToggle({ className }: { className?: string }) {
  const { theme, toggle, mounted } = useTheme();
  const isLight = theme === "light";

  return (
    <button
      type="button"
      onClick={toggle}
      aria-pressed={mounted ? isLight : undefined}
      aria-label={isLight ? "Switch to dark theme" : "Switch to light theme"}
      className={cn(
        "grid size-[var(--control-height-md)] place-items-center rounded-full",
        "border border-line-interactive text-secondary",
        "transition-colors duration-200 ease-standard",
        "hover:bg-hover hover:text-primary",
        "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
        className,
      )}
    >
      {mounted && isLight ? (
        <Moon className="size-icon-sm" aria-hidden="true" />
      ) : (
        <Sun className="size-icon-sm" aria-hidden="true" />
      )}
    </button>
  );
}
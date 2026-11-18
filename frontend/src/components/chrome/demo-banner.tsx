"use client";

import * as React from "react";
import { AlertTriangle, X } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * The demo-data banner (§8.2).
 *
 * Dismissible but returning: dismissing hides it for this session only, and it
 * comes back on reload because the condition that produced it has not changed.
 * That is deliberate — a banner you can permanently silence is a banner that
 * lies about the state of the system.
 */
export function DemoBanner({ className }: { className?: string }) {
  const [dismissed, setDismissed] = React.useState(false);
  if (dismissed) return null;

  return (
    <div
      role="status"
      className={cn(
        "flex items-start gap-2.5 border-b border-line bg-inset px-4 py-2.5",
        className,
      )}
    >
      <AlertTriangle className="mt-0.5 size-4 shrink-0 text-accent-alt" aria-hidden="true" />
      <p className="flex-1 text-xs leading-snug text-secondary">
        Cannot reach the gateway. Showing demo posts.
      </p>
      <button
        type="button"
        onClick={() => setDismissed(true)}
        aria-label="Dismiss demo notice"
        className="-m-1 grid size-6 shrink-0 place-items-center rounded-full text-tertiary transition-colors hover:bg-hover hover:text-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      >
        <X className="size-icon-xs" aria-hidden="true" />
      </button>
    </div>
  );
}
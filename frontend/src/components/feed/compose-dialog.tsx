"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import { PenLine } from "lucide-react";
import { cn } from "@/lib/utils";

const ComposeDialogContent = dynamic(
  () => import("./compose-dialog-content"),
  { ssr: false, loading: () => null },
);

/**
 * The rail's Post button.
 *
 * The modal is the same Composer the home column renders inline, so there is
 * one character limit, one validation rule, and one optimistic insert path.
 * Its chunk is loaded only when the button is pressed.
 */
export function PostButton({
  compact = false,
  className,
}: {
  compact?: boolean;
  className?: string;
}) {
  const [open, setOpen] = React.useState(false);

  return (
    <>
      <button
        type="button"
        aria-label={compact ? "Create a post" : undefined}
        onClick={() => setOpen(true)}
        className={cn(
          "inline-flex items-center justify-center gap-2 bg-accent-solid font-sans font-semibold text-on-accent",
          "transition-colors duration-200 hover:bg-accent-solid-hover active:opacity-90",
          "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
          compact ? "size-[var(--control-height-xl)] rounded-full" : "rounded-full",
          className,
        )}
      >
        <PenLine className={compact ? "size-icon-md" : "size-icon-sm"} aria-hidden="true" />
        {compact ? null : "Post"}
      </button>

      {open ? (
        <ComposeDialogContent open={open} onOpenChange={setOpen} />
      ) : null}
    </>
  );
}

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 font-sans text-2xs font-medium",
  {
    variants: {
      tone: {
        neutral: "bg-inset text-secondary",
        accent: "bg-accent-soft text-accent",
        danger: "bg-danger/15 text-danger",
        success: "bg-success/15 text-success",
        outline: "border border-line-strong text-secondary",
      },
    },
    defaultVariants: { tone: "neutral" },
  },
);

export function Badge({
  className,
  tone,
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & VariantProps<typeof badgeVariants>) {
  return <span className={cn(badgeVariants({ tone }), className)} {...props} />;
}

/** A hashtag chip. Mono, so tags read as data rather than prose. */
export function TagChip({ tag, className }: { tag: string; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-xs bg-inset px-1.5 py-0.5",
        "font-mono text-2xs tracking-mono text-accent-alt",
        className,
      )}
    >
      {tag}
    </span>
  );
}
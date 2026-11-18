"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

const fieldBase = [
  "w-full bg-page text-primary placeholder:text-tertiary",
  "border border-line-strong rounded-md",
  "px-4 py-3",
  "text-base leading-snug",
  "transition-[border-color,box-shadow,background-color] duration-200 ease-standard",
  "hover:border-line-interactive",
  "focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring",
  "disabled:opacity-50 disabled:cursor-not-allowed",
  "aria-[invalid=true]:border-danger",
].join(" ");

export const Input = React.forwardRef<
  HTMLInputElement,
  React.InputHTMLAttributes<HTMLInputElement>
>(({ className, type = "text", ...props }, ref) => (
  <input
    ref={ref}
    type={type}
    className={cn(fieldBase, "h-[var(--control-height-xl)] rounded-lg px-4", className)}
    {...props}
  />
));
Input.displayName = "Input";

export const InputBase = React.forwardRef<
  HTMLInputElement,
  React.InputHTMLAttributes<HTMLInputElement>
>(({ className, type = "text", ...props }, ref) => (
  <input ref={ref} type={type} className={cn(fieldBase, className)} {...props} />
));
InputBase.displayName = "InputBase";

export const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.TextareaHTMLAttributes<HTMLTextAreaElement>
>(({ className, ...props }, ref) => (
  <textarea
    ref={ref}
    className={cn(fieldBase, "min-h-[96px] resize-none rounded-lg", className)}
    {...props}
  />
));
Textarea.displayName = "Textarea";

/** A labelled field wrapper that owns the error wiring, so no screen forgets it. */
export function Field({
  id,
  label,
  hint,
  error,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2">
      <label htmlFor={id} className="text-xs font-medium text-secondary">
        {label}
      </label>
      {children}
      {hint && !error ? (
        <p id={`${id}-hint`} className="text-xs text-tertiary">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={`${id}-error`} role="alert" className="text-xs text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
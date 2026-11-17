"use client";

import { toast } from "sonner";

/**
 * One vocabulary for notifications.
 *
 * Screens call `notify` rather than Sonner directly, so every success, failure,
 * and informational message shares duration, wording shape, and dismissal.
 * Toasts complement inline states; they never replace the explanation a failed
 * control renders in place.
 */
export const notify = {
  success(title: string, description?: string) {
    toast.success(title, description ? { description } : undefined);
  },
  error(title: string, description?: string) {
    toast.error(title, description ? { description } : undefined);
  },
  info(title: string, description?: string) {
    toast.info(title, description ? { description } : undefined);
  },
  warning(title: string, description?: string) {
    toast.warning(title, description ? { description } : undefined);
  },
  message(title: string, description?: string) {
    toast(title, description ? { description } : undefined);
  },
  dismiss(id?: string | number) {
    toast.dismiss(id);
  },
};

/** Human wording for an unknown thrown value. */
export function messageFrom(error: unknown, fallback: string): string {
  if (error instanceof Error && error.message.trim()) return error.message;
  if (typeof error === "string" && error.trim()) return error;
  return fallback;
}

"use client";

import * as React from "react";
import { notify } from "@/lib/toast";

/**
 * Announces a query failure once, then stays quiet until the message changes.
 * Screens keep their inline error state; this only makes the failure visible
 * when the failing region is off-screen or the page first opens.
 */
export function useErrorToast(
  error: { message?: string } | null | undefined,
  title: string,
): void {
  const last = React.useRef<string | null>(null);

  React.useEffect(() => {
    if (!error) {
      last.current = null;
      return;
    }
    const message = error.message ?? "Something went wrong.";
    if (last.current === message) return;
    last.current = message;
    notify.error(title, message);
  }, [error, title]);
}

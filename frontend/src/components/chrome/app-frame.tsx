"use client";

import { usePathname } from "next/navigation";
import { AppShell } from "./app-shell";

/**
 * Keeps the navigation shell mounted across route changes.
 *
 * The root layout owns this frame, so switching from the feed to explore does
 * not unmount the rails, the unread query, or the conversation surface. Login
 * is the one route that owns the full viewport.
 */
export function AppFrame({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  if (pathname === "/login") return <>{children}</>;
  return <AppShell>{children}</AppShell>;
}

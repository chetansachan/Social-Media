"use client";

import * as React from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { User } from "@/lib/types";

type Status = "loading" | "authenticated" | "anonymous";

type SessionValue = {
  user: User | null;
  status: Status;
  /** Re-reads the httpOnly cookie server-side. Call after sign-in. */
  refresh: () => Promise<User | null>;
  signOut: () => Promise<void>;
};

const SessionContext = React.createContext<SessionValue | null>(null);

async function readSession(): Promise<User | null> {
  try {
    const response = await fetch("/api/auth/session", {
      cache: "no-store",
      credentials: "same-origin",
    });
    if (!response.ok) return null;
    const body = (await response.json()) as { user?: User | null };
    return body.user ?? null;
  } catch {
    return null;
  }
}

/**
 * Session state.
 *
 * The profile is resolved once per app load from /api/auth/session, which reads
 * the httpOnly cookie. Every screen reads the viewer from here instead of
 * hardcoding a demo id, so composer authorship, like attribution, and the
 * profile link all follow the signed-in account.
 */
export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = React.useState<User | null>(null);
  const [status, setStatus] = React.useState<Status>("loading");
  const queryClient = useQueryClient();

  React.useEffect(() => {
    let cancelled = false;
    void readSession().then((next) => {
      if (cancelled) return;
      setUser(next);
      setStatus(next ? "authenticated" : "anonymous");
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const refresh = React.useCallback(async () => {
    const next = await readSession();
    setUser(next);
    setStatus(next ? "authenticated" : "anonymous");
    return next;
  }, []);

  const signOut = React.useCallback(async () => {
    try {
      await fetch("/api/auth/logout", {
        method: "POST",
        credentials: "same-origin",
      });
    } finally {
      setUser(null);
      setStatus("anonymous");
      queryClient.clear();
    }
  }, [queryClient]);

  const value = React.useMemo(
    () => ({ user, status, refresh, signOut }),
    [user, status, refresh, signOut],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionValue {
  const context = React.useContext(SessionContext);
  if (!context) throw new Error("useSession must be used inside SessionProvider");
  return context;
}
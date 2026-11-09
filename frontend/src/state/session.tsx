import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { getAccessToken, request, setAccessToken } from "../lib/api";
import { operations } from "../lib/api";
import type { LoginResponse } from "../lib/api";

/* ==========================================================================
   Session
   --------------------------------------------------------------------------
   The token lives in memory and in sessionStorage so a reload does not sign the
   user out mid-review, but nothing sensitive is persisted beyond the token
   itself. A revoked token is indistinguishable from a missing one: the API
   client clears the session on UNAUTHENTICATED, and every screen responds by
   rendering its signed-out state rather than an error.
   ========================================================================== */

export interface SessionValue {
  signedIn: boolean;
  userID: string;
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => void;
}

const SessionContext = createContext<SessionValue | null>(null);
const STORAGE_KEY = "homex.token";

export function SessionProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(() => {
    try {
      return window.sessionStorage.getItem(STORAGE_KEY);
    } catch {
      return null;
    }
  });

  useEffect(() => {
    setAccessToken(token);
    try {
      if (token) window.sessionStorage.setItem(STORAGE_KEY, token);
      else window.sessionStorage.removeItem(STORAGE_KEY);
    } catch {
      // A privacy mode that blocks storage still leaves the app usable for the
      // session; it simply re-authenticates on reload.
    }
  }, [token]);

  // The token is a JWT whose payload carries the user id. Reading it here avoids
  // a round trip to the gateway just to render the avatar.
  const userID = useMemo(() => subjectFromToken(token), [token]);

  const signIn = useCallback(async (email: string, password: string) => {
    const data = await request<LoginResponse>(operations.login, {
      input: { email, password },
    });
    setToken(data.Login.accessToken);
  }, []);

  const signOut = useCallback(() => {
    setToken(null);
    setAccessToken(null);
  }, []);

  const value = useMemo<SessionValue>(
    () => ({ signedIn: Boolean(token), userID, signIn, signOut }),
    [token, userID, signIn, signOut],
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionValue {
  const value = useContext(SessionContext);
  if (!value) throw new Error("useSession must be used inside SessionProvider");
  return value;
}

function subjectFromToken(token: string | null): string {
  if (!token) return "";
  const parts = token.split(".");
  if (parts.length < 2) return "";
  try {
    const padded = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const json = atob(padded.padEnd(padded.length + ((4 - (padded.length % 4)) % 4), "="));
    const claims = JSON.parse(json) as { user_id?: string };
    return claims.user_id ?? "";
  } catch {
    return "";
  }
}

export { getAccessToken };

import { NextRequest, NextResponse } from "next/server";
import {
  ACCESS_COOKIE,
  REFRESH_COOKIE,
  clearAuthCookies,
  loadViewer,
  refreshSession,
  setAuthCookies,
} from "@/lib/server/auth";

/**
 * The session endpoint.
 *
 * The access token lives in an httpOnly cookie, so the browser cannot read the
 * `user_id` claim itself. This handler decodes it server-side, resolves the
 * profile through the gateway, and transparently rotates an expired token with
 * the refresh cookie before giving up.
 */
export async function GET(request: NextRequest) {
  const accessToken = request.cookies.get(ACCESS_COOKIE)?.value;
  const refreshToken = request.cookies.get(REFRESH_COOKIE)?.value;

  if (!accessToken && !refreshToken) {
    return NextResponse.json({ user: null });
  }

  if (accessToken) {
    const user = await loadViewer(accessToken);
    if (user) return NextResponse.json({ user });
  }

  if (!refreshToken) {
    return clearAuthCookies(NextResponse.json({ user: null }));
  }

  const tokens = await refreshSession(refreshToken);
  if (!tokens) {
    return clearAuthCookies(NextResponse.json({ user: null }));
  }

  const user = await loadViewer(tokens.accessToken);
  const response = NextResponse.json({ user });
  return setAuthCookies(response, tokens.accessToken, tokens.refreshToken);
}

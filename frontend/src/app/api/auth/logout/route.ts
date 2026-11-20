import { NextRequest, NextResponse } from "next/server";
import {
  ACCESS_COOKIE,
  clearAuthCookies,
  gatewayFetch,
} from "@/lib/server/auth";

const LOGOUT = /* GraphQL */ `
  mutation Logout {
    logout
  }
`;

/**
 * Sign-out revokes the access token at the gateway (the jti is blacklisted for
 * its remaining lifetime) and then clears both cookies. If the gateway is
 * unreachable the cookies are still cleared: a local sign-out must never fail.
 */
export async function POST(request: NextRequest) {
  const token = request.cookies.get(ACCESS_COOKIE)?.value;
  if (token) {
    await gatewayFetch<{ logout?: boolean }>(LOGOUT, {}, token);
  }
  return clearAuthCookies(NextResponse.json({ ok: true }));
}

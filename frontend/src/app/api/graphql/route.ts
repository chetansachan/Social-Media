import { NextRequest, NextResponse } from "next/server";
import {
  ACCESS_COOKIE,
  REFRESH_COOKIE,
  clearAuthCookies,
  refreshSession,
  setAuthCookies,
} from "@/lib/server/auth";

const GATEWAY = process.env.GATEWAY_URL ?? "http://localhost:8080/query";

/**
 * The GraphQL proxy.
 *
 * The gateway mounts GraphQL on /query with no CORS middleware, so a browser
 * page on :3000 cannot call :8080 cross-origin. Servers are not subject to
 * CORS, so this handler forwards the body and attaches the bearer token from
 * the httpOnly cookie. The token therefore never reaches JavaScript.
 *
 * Access tokens live 15 minutes. When the gateway answers 401, this handler
 * rotates the pair with the refresh cookie and replays the request once, so a
 * long-lived tab does not silently degrade into "signed out" mid-session.
 */
export async function POST(request: NextRequest) {
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { code: "INTERNAL", message: "Malformed request body." },
      { status: 400 },
    );
  }

  const accessToken = request.cookies.get(ACCESS_COOKIE)?.value;
  const refreshToken = request.cookies.get(REFRESH_COOKIE)?.value;

  const attempt = (token: string | undefined) =>
    fetch(GATEWAY, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(body),
      cache: "no-store",
      signal: AbortSignal.timeout(10_000),
    });

  let upstream: Response;
  try {
    upstream = await attempt(accessToken);
  } catch {
    return NextResponse.json(
      { code: "GATEWAY_DOWN", message: "Cannot reach the gateway." },
      { status: 503 },
    );
  }

  let rotated: { accessToken: string; refreshToken: string } | null = null;

  if (upstream.status === 401 && refreshToken) {
    rotated = await refreshSession(refreshToken);
    if (rotated) {
      try {
        upstream = await attempt(rotated.accessToken);
      } catch {
        return NextResponse.json(
          { code: "GATEWAY_DOWN", message: "Cannot reach the gateway." },
          { status: 503 },
        );
      }
    }
  }

  const withRotation = (response: NextResponse) => {
    if (rotated) setAuthCookies(response, rotated.accessToken, rotated.refreshToken);
    return response;
  };

  if (upstream.status === 401) {
    return clearAuthCookies(
      NextResponse.json(
        { code: "UNAUTHENTICATED", message: "Your session has expired." },
        { status: 401 },
      ),
    );
  }

  if (upstream.status === 429) {
    const retryAfter = upstream.headers.get("Retry-After") ?? "30";
    const response = NextResponse.json(
      {
        code: "RATE_LIMITED",
        message: "Too many requests.",
        retryAfter: Number(retryAfter) || 30,
      },
      { status: 429 },
    );
    response.headers.set("Retry-After", retryAfter);
    return withRotation(response);
  }

  if (upstream.status === 404) {
    return NextResponse.json(
      { code: "GATEWAY_DOWN", message: "The gateway is not answering." },
      { status: 503 },
    );
  }

  if (!upstream.ok) {
    return NextResponse.json(
      { code: "TRANSPORT", message: `The gateway returned ${upstream.status}.` },
      { status: 502 },
    );
  }

  const payload = await upstream.json().catch(() => null);
  if (!payload) {
    return NextResponse.json(
      { code: "TRANSPORT", message: "The gateway returned no body." },
      { status: 502 },
    );
  }
  return withRotation(NextResponse.json(payload, { status: 200 }));
}

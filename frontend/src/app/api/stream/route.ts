import { NextRequest, NextResponse } from "next/server";
import {
  ACCESS_COOKIE,
  REFRESH_COOKIE,
  clearAuthCookies,
  refreshSession,
  setAuthCookies,
} from "@/lib/server/auth";

/**
 * The gateway's SSE endpoint, derived from `GATEWAY_URL` so the two proxies
 * cannot drift apart. Rewriting the path rather than string-replacing `/query`
 * keeps this correct when `GATEWAY_URL` is a bare origin.
 */
const GATEWAY_STREAM = (() => {
  const raw = process.env.GATEWAY_URL ?? "http://localhost:8080/query";
  try {
    const url = new URL(raw);
    url.pathname = "/stream";
    url.search = "";
    return url.toString();
  } catch {
    return "http://localhost:8080/stream";
  }
})();

/**
 * The realtime stream proxy.
 *
 * The gateway's `/stream` endpoint is SSE + bearer token, but the browser holds
 * that token in an httpOnly cookie and cannot set an `Authorization` header on
 * an `EventSource`. This handler opens the upstream stream server-side, attaches
 * the bearer token, and pipes the bytes through. The token therefore never
 * reaches JavaScript, exactly as for the GraphQL proxy.
 *
 * Only an opened stream and its close are forwarded — the gateway sends
 * keep-alive comments every 15s, so the response stays a long-lived ReadableStream.
 */
export async function GET(request: NextRequest) {
  const accessToken = request.cookies.get(ACCESS_COOKIE)?.value;
  const refreshToken = request.cookies.get(REFRESH_COOKIE)?.value;

  const attempt = (token: string | undefined) =>
    fetch(GATEWAY_STREAM, {
      headers: {
        Accept: "text/event-stream",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      cache: "no-store",
      signal: request.signal,
    });

  let upstream: Response;
  try {
    upstream = await attempt(accessToken);
  } catch {
    // A fetch abort is the client disconnecting, which is normal. Only a
    // genuinely failed connect is an error worth reporting.
    if (request.signal.aborted) return new NextResponse(null, { status: 499 });
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
        if (request.signal.aborted) return new NextResponse(null, { status: 499 });
        return NextResponse.json(
          { code: "GATEWAY_DOWN", message: "Cannot reach the gateway." },
          { status: 503 },
        );
      }
    }
  }

  if (upstream.status === 401) {
    return clearAuthCookies(
      NextResponse.json(
        { code: "UNAUTHENTICATED", message: "Your session has expired." },
        { status: 401 },
      ),
    );
  }

  if (!upstream.ok) {
    return NextResponse.json(
      {
        code: "GATEWAY_DOWN",
        message: `The gateway returned ${upstream.status}.`,
      },
      { status: 503 },
    );
  }

  if (!upstream.body) {
    return NextResponse.json(
      { code: "TRANSPORT", message: "The gateway opened no stream." },
      { status: 502 },
    );
  }

  const response = new NextResponse(upstream.body, { status: 200 });
  response.headers.set("Content-Type", "text/event-stream");
  response.headers.set("Cache-Control", "no-cache, no-transform");
  response.headers.set("Connection", "keep-alive");
  // A proxy in front of Next must forward frames as they arrive, not batch them.
  response.headers.set("X-Accel-Buffering", "no");
  if (rotated) setAuthCookies(response, rotated.accessToken, rotated.refreshToken);
  return response;
}
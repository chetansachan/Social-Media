import type { NextResponse } from "next/server";
import type { User } from "@/lib/types";

export const GATEWAY_URL =
  process.env.GATEWAY_URL ?? "http://localhost:8080/query";

export const ACCESS_COOKIE = "sf_access";
export const REFRESH_COOKIE = "sf_refresh";

const ACCESS_MAX_AGE = 15 * 60;
const REFRESH_MAX_AGE = 7 * 24 * 60 * 60;

export const USER_BY_ID = /* GraphQL */ `
  query GetUserById($id: ID!) {
    getUserById(id: $id) {
      id
      username
      name
      email
      isPublic
      createdAt
      updatedAt
    }
  }
`;

export const REFRESH_TOKENS = /* GraphQL */ `
  query RefreshTokens($refresh: String!) {
    tokenGenration(refressToken: $refresh) {
      accessToken
      refressToken
    }
  }
`;

type GatewayResult<T> = {
  status: number;
  data?: T;
  message?: string;
  retryAfter?: number;
};

/** A server-side GraphQL call. Never throws: transport failures become a status. */
export async function gatewayFetch<T>(
  query: string,
  variables: Record<string, unknown>,
  token?: string,
  signal?: AbortSignal,
): Promise<GatewayResult<T>> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token) headers.Authorization = `Bearer ${token}`;

  let response: Response;
  try {
    response = await fetch(GATEWAY_URL, {
      method: "POST",
      headers,
      body: JSON.stringify({ query, variables }),
      cache: "no-store",
      signal: signal ?? AbortSignal.timeout(10_000),
    });
  } catch {
    return { status: 503, message: "Cannot reach the gateway." };
  }

  if (!response.ok) {
    return {
      status: response.status,
      message: `The gateway returned ${response.status}.`,
      retryAfter: Number(response.headers.get("Retry-After") ?? "") || undefined,
    };
  }

  const payload = (await response.json().catch(() => null)) as {
    data?: T;
    errors?: { message: string; extensions?: { code?: string } }[];
  } | null;

  if (!payload) return { status: 502, message: "The gateway returned no body." };
  if (payload.errors?.length) {
    // Only an auth failure is a 401. Validation and conflict errors are 400s;
    // mapping them all to 401 made a duplicate handle look like a expired
    // session to every caller.
    const code = payload.errors[0]?.extensions?.code;
    return {
      status: code === "UNAUTHENTICATED" ? 401 : 400,
      message: payload.errors[0]?.message ?? "Request failed.",
    };
  }
  return { status: 200, data: payload.data };
}

/** Reads the `user_id` claim. Signature verification happens at the gateway. */
export function userIdFromToken(token: string): string | null {
  const part = token.split(".")[1];
  if (!part) return null;
  try {
    const padded = part
      .replace(/-/g, "+")
      .replace(/_/g, "/")
      .padEnd(Math.ceil(part.length / 4) * 4, "=");
    const claims = JSON.parse(Buffer.from(padded, "base64").toString("utf8")) as {
      user_id?: unknown;
    };
    return typeof claims.user_id === "string" ? claims.user_id : null;
  } catch {
    return null;
  }
}

export async function refreshSession(
  refreshToken: string,
): Promise<{ accessToken: string; refreshToken: string } | null> {
  const result = await gatewayFetch<{
    tokenGenration?: { accessToken: string; refressToken: string };
  }>(REFRESH_TOKENS, { refresh: refreshToken });

  const tokens = result.data?.tokenGenration;
  if (result.status !== 200 || !tokens?.accessToken) return null;
  return { accessToken: tokens.accessToken, refreshToken: tokens.refressToken };
}

export function setAuthCookies(
  response: NextResponse,
  accessToken: string,
  refreshToken: string,
): NextResponse {
  const secure = process.env.NODE_ENV === "production";
  response.cookies.set(ACCESS_COOKIE, accessToken, {
    httpOnly: true,
    sameSite: "lax",
    secure,
    path: "/",
    maxAge: ACCESS_MAX_AGE,
  });
  response.cookies.set(REFRESH_COOKIE, refreshToken, {
    httpOnly: true,
    sameSite: "lax",
    secure,
    path: "/",
    maxAge: REFRESH_MAX_AGE,
  });
  return response;
}

export function clearAuthCookies(response: NextResponse): NextResponse {
  response.cookies.delete(ACCESS_COOKIE);
  response.cookies.delete(REFRESH_COOKIE);
  return response;
}

export async function loadViewer(accessToken: string): Promise<User | null> {
  const id = userIdFromToken(accessToken);
  if (!id) return null;
  const result = await gatewayFetch<{ getUserById?: User }>(
    USER_BY_ID,
    { id },
    accessToken,
  );
  return result.data?.getUserById ?? null;
}

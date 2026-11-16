/* ==========================================================================
   GraphQL client.
   --------------------------------------------------------------------------
   Queries and mutations go through the Next route handler at /api/graphql,
   which reads the httpOnly cookie and attaches the bearer token server-side.
   The token therefore never reaches JavaScript.

   Failures are typed results, never thrown strings, because every screen has
   a defined error state.
   ========================================================================== */

import { ApiError, type ErrorCode } from "../types";

const ENDPOINT = "/api/graphql";

type GraphQLResponse<T> = {
  data?: T;
  errors?: { message: string; extensions?: { code?: string } }[];
};

function toErrorCode(code: string | undefined): ErrorCode {
  switch (code) {
    case "GATEWAY_DOWN":
    case "UNAUTHENTICATED":
    case "RATE_LIMITED":
    case "NETWORK":
    case "TRANSPORT":
    case "EMPTY":
      return code;
    default:
      return "INTERNAL";
  }
}

export async function gql<T>(
  query: string,
  variables: Record<string, unknown> = {},
  signal?: AbortSignal,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(ENDPOINT, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query, variables }),
      signal,
      credentials: "same-origin",
    });
  } catch (cause) {
    if (signal?.aborted) throw cause;
    throw new ApiError("Cannot reach the gateway.", "NETWORK");
  }

  // The proxy returns a typed envelope for transport-level failures.
  if (!response.ok) {
    const retryAfter = Number(response.headers.get("Retry-After") ?? "") || undefined;
    const body = (await response.json().catch(() => ({}))) as {
      code?: string;
      message?: string;
    };
    throw new ApiError(
      body.message ?? `The gateway returned ${response.status}.`,
      toErrorCode(body.code),
      retryAfter,
    );
  }

  const payload = (await response.json()) as GraphQLResponse<T>;
  const error = payload.errors?.[0];
  if (error) {
    throw new ApiError(error.message, toErrorCode(error.extensions?.code));
  }
  if (!payload.data) {
    throw new ApiError("The gateway returned no data.", "EMPTY");
  }
  return payload.data;
}

/** True when the failure means "no backend", which every screen renders as demo. */
export function isOffline(error: unknown): boolean {
  const code = (error as ApiError | undefined)?.code;
  return code === "GATEWAY_DOWN" || code === "NETWORK" || code === "TRANSPORT";
}

export { operations } from "./operations";

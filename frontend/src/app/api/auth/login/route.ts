import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { gatewayFetch, setAuthCookies } from "@/lib/server/auth";

const LoginSchema = z.object({
  email: z.string().trim().email(),
  password: z.string().min(8),
});

const LOGIN = /* GraphQL */ `
  mutation Login($input: LoginInput!) {
    Login(input: $input) {
      accessToken
      refressToken
      message
    }
  }
`;

/** Login writes both tokens as httpOnly cookies; neither is returned in the body. */
export async function POST(request: NextRequest) {
  const parsed = LoginSchema.safeParse(await request.json().catch(() => null));
  if (!parsed.success) {
    return NextResponse.json(
      { code: "INVALID", message: "Check your email and password." },
      { status: 400 },
    );
  }

  const result = await gatewayFetch<{
    Login?: { accessToken: string; refressToken: string; message: string };
  }>(LOGIN, {
    input: { email: parsed.data.email, password: parsed.data.password },
  });

  if (result.status === 503) {
    return NextResponse.json(
      { code: "GATEWAY_DOWN", message: result.message },
      { status: 503 },
    );
  }

  const login = result.data?.Login;
  if (!login?.accessToken) {
    return NextResponse.json(
      {
        code: result.status === 429 ? "RATE_LIMITED" : "UNAUTHENTICATED",
        message: result.message ?? "Those credentials did not work.",
      },
      { status: result.status === 429 ? 429 : 401 },
    );
  }

  return setAuthCookies(
    NextResponse.json({ ok: true }),
    login.accessToken,
    login.refressToken,
  );
}

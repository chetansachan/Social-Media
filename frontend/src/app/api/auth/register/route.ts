import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { gatewayFetch, setAuthCookies } from "@/lib/server/auth";
import type { User } from "@/lib/types";

const RegisterSchema = z.object({
  username: z
    .string()
    .trim()
    .min(3, "Use at least 3 characters.")
    .max(30, "Keep it under 30 characters.")
    .regex(/^[a-zA-Z0-9_]+$/, "Letters, numbers, and underscores only."),
  name: z.string().trim().min(1, "Tell us your name.").max(60),
  email: z.string().trim().email("That email does not look right."),
  password: z.string().min(8, "Use at least 8 characters."),
});

const REGISTER = /* GraphQL */ `
  mutation Register($input: RegisterUserInput!) {
    register(input: $input) {
      id
      username
      name
      email
    }
  }
`;

const LOGIN = /* GraphQL */ `
  mutation Login($input: LoginInput!) {
    Login(input: $input) {
      accessToken
      refressToken
      message
    }
  }
`;

/**
 * Registration is two gateway calls: `register` creates the account but returns
 * no tokens, so the handler immediately signs the new user in and writes the
 * cookies itself. The client never has to know about the split.
 */
export async function POST(request: NextRequest) {
  const parsed = RegisterSchema.safeParse(await request.json().catch(() => null));
  if (!parsed.success) {
    return NextResponse.json(
      {
        code: "INVALID",
        message: parsed.error.issues[0]?.message ?? "Check the form.",
      },
      { status: 400 },
    );
  }

  const input = { ...parsed.data, isPublic: true };
  const created = await gatewayFetch<{ register?: User }>(REGISTER, { input });

  if (created.status !== 200 || !created.data?.register) {
    const message = created.message ?? "Could not create the account.";
    const taken = /exists|duplicate|taken|unique/i.test(message);
    return NextResponse.json(
      { code: taken ? "EXISTS" : "INVALID", message },
      { status: taken ? 409 : created.status === 503 ? 503 : 400 },
    );
  }

  const login = await gatewayFetch<{
    Login?: { accessToken: string; refressToken: string };
  }>(LOGIN, {
    input: { email: parsed.data.email, password: parsed.data.password },
  });

  const tokens = login.data?.Login;
  if (login.status !== 200 || !tokens?.accessToken) {
    return NextResponse.json(
      {
        code: "AUTH",
        message: "Account created, but sign-in failed. Try signing in.",
      },
      { status: 502 },
    );
  }

  const response = NextResponse.json({
    ok: true,
    user: created.data.register,
  });
  return setAuthCookies(response, tokens.accessToken, tokens.refressToken);
}

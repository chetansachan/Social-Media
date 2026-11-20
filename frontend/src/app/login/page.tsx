"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, ArrowRight, Loader2, Sparkles } from "lucide-react";
import { notify } from "@/lib/toast";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { LogoMark } from "@/components/chrome/logo";
import { useSession } from "@/lib/providers/session-provider";

type Mode = "signin" | "register";

const DEMO = { email: "chetan@example.com", password: "password123" };

function LoginInner() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { refresh, status } = useSession();

  const initialMode: Mode = searchParams?.get("mode") === "register" ? "register" : "signin";
  const [mode, setMode] = React.useState<Mode>(initialMode);
  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [username, setUsername] = React.useState("");
  const [name, setName] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  const [pending, setPending] = React.useState(false);

  React.useEffect(() => {
    if (status === "authenticated") router.replace("/");
  }, [status, router]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);
    setPending(true);

    const endpoint = mode === "signin" ? "/api/auth/login" : "/api/auth/register";
    const payload =
      mode === "signin"
        ? { email, password }
        : { email, password, username, name };

    try {
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify(payload),
      });
      const body = (await response.json().catch(() => ({}))) as { message?: string };

      if (!response.ok) {
        const reason = body.message ?? "That did not work. Check the form and try again.";
        setError(reason);
        notify.error(
          mode === "signin" ? "Sign in failed" : "Account creation failed",
          reason,
        );
        return;
      }

      await refresh();
      notify.success(
        mode === "signin" ? "Welcome back" : "Account created",
        mode === "signin" ? "Your timeline is ready." : "Start by following a few people.",
      );
      router.push("/");
      router.refresh();
    } catch {
      setError("Cannot reach the server. Is the gateway running?");
      notify.error("Cannot reach the server", "Check that the gateway is running.");
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="grid min-h-dvh bg-page lg:grid-cols-[1.05fr_1fr]">
      {/* Brand panel — decorative; dropped on phones. */}
      <aside className="relative hidden overflow-hidden border-r border-line lg:flex lg:flex-col lg:justify-between lg:p-14">
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -left-32 -top-32 size-[34rem] rounded-full bg-accent-gradient opacity-15 blur-3xl"
        />
        <Link href="/" className="relative flex items-center gap-3">
          <LogoMark size={44} />
          <span className="sf-wordmark text-2xl text-primary">SocialFlow</span>
        </Link>

        <div className="relative max-w-md">
          <p className="inline-flex items-center gap-2 rounded-full border border-line bg-inset px-3 py-1 font-mono text-2xs uppercase tracking-mono text-secondary">
            <Sparkles className="size-icon-xs text-accent" aria-hidden="true" />
            Follow-only · chronological
          </p>
          <h1 className="mt-6 font-display text-display leading-display tracking-display text-primary">
            The feed you chose.
          </h1>
          <p className="mt-5 text-lg leading-relaxed text-secondary">
            A small social network for engineers: posts, threads, follows, and
            direct messages on a Go microservice backend.
          </p>
        </div>

        <p className="relative font-mono text-2xs uppercase tracking-mono text-tertiary">
          Go · gRPC · GraphQL · Postgres · Redis
        </p>
      </aside>

      <main className="flex items-center justify-center px-5 py-12 sm:px-10">
        <div className="w-full max-w-[364px]">
          <div className="flex items-center justify-between lg:hidden">
            <Link href="/" className="flex items-center gap-2.5">
              <LogoMark size={38} />
              <span className="sf-wordmark text-xl text-primary">SocialFlow</span>
            </Link>
            <Button asChild variant="ghost" size="sm">
              <Link href="/">Browse</Link>
            </Button>
          </div>

          <div className="mt-10 lg:mt-0">
            <h2 className="font-display text-4xl leading-tight text-primary">
              {mode === "signin" ? "Sign in to SocialFlow" : "Create your account"}
            </h2>
            <p className="mt-2 text-sm text-secondary">
              {mode === "signin"
                ? "See the people you follow, newest first."
                : "Pick a handle. You can change your name later."}
            </p>
          </div>

          <div
            role="tablist"
            aria-label="Authentication mode"
            className="mt-7 grid grid-cols-2 gap-1 rounded-full border border-line bg-inset p-1"
          >
            {(["signin", "register"] as const).map((value) => (
              <button
                key={value}
                type="button"
                role="tab"
                aria-selected={mode === value}
                onClick={() => {
                  setMode(value);
                  setError(null);
                }}
                className={
                  mode === value
                    ? "rounded-full bg-page px-4 py-2 text-sm font-semibold text-primary shadow-1"
                    : "rounded-full px-4 py-2 text-sm font-medium text-secondary transition-colors hover:text-primary"
                }
              >
                {value === "signin" ? "Sign in" : "Create account"}
              </button>
            ))}
          </div>

          <form onSubmit={submit} className="mt-6 space-y-4">
            {mode === "register" ? (
              <>
                <Field id="name" label="Name">
                  <Input
                    id="name"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    autoComplete="name"
                    placeholder="Chetan Sachan"
                    required
                  />
                </Field>
                <Field
                  id="username"
                  label="Handle"
                  hint="Letters, numbers, and underscores."
                >
                  <Input
                    id="username"
                    value={username}
                    onChange={(event) => setUsername(event.target.value)}
                    autoComplete="username"
                    placeholder="chetan"
                    pattern="[A-Za-z0-9_]+"
                    minLength={3}
                    required
                  />
                </Field>
              </>
            ) : null}

            <Field id="email" label="Email">
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                autoComplete="email"
                placeholder="you@example.com"
                required
              />
            </Field>

            <Field
              id="password"
              label="Password"
              hint={mode === "register" ? "At least 8 characters." : undefined}
            >
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete={mode === "signin" ? "current-password" : "new-password"}
                placeholder="••••••••"
                minLength={8}
                required
              />
            </Field>

            {error ? (
              <p
                role="alert"
                className="rounded-md border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger"
              >
                {error}
              </p>
            ) : null}

            <Button type="submit" size="lg" className="w-full" disabled={pending}>
              {pending ? (
                <Loader2 className="size-icon-xs animate-spin" aria-hidden="true" />
              ) : null}
              {mode === "signin" ? "Sign in" : "Create account"}
              {!pending ? <ArrowRight className="size-icon-xs" aria-hidden="true" /> : null}
            </Button>
          </form>

          {mode === "signin" ? (
            <button
              type="button"
              onClick={() => {
                setEmail(DEMO.email);
                setPassword(DEMO.password);
                setError(null);
              }}
              className="mt-3 w-full rounded-md border border-dashed border-line-interactive px-3 py-2 text-center font-mono text-2xs tracking-mono text-tertiary transition-colors hover:text-secondary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
            >
              Use demo account · chetan@example.com / password123
            </button>
          ) : null}

          <Button asChild variant="ghost" size="sm" className="mt-6 w-full">
            <Link href="/">
              <ArrowLeft className="size-icon-xs" aria-hidden="true" />
              Back to the feed
            </Link>
          </Button>
        </div>
      </main>
    </div>
  );
}

export default function LoginPage() {
  return (
    <React.Suspense fallback={<div className="min-h-dvh bg-page" />}>
      <LoginInner />
    </React.Suspense>
  );
}
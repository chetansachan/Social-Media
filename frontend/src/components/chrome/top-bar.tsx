"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { LogIn, LogOut, Moon, Search, Sun, UserRound } from "lucide-react";
import { notify } from "@/lib/toast";
import { attachHeaderElevation } from "@/lib/motion";
import { Avatar } from "@/components/ui/avatar";
import { useSession } from "@/lib/providers/session-provider";
import { useTheme } from "@/lib/providers/theme-provider";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const titles: Record<string, string> = {
  "/": "Home",
  "/explore": "Explore",
  "/notifications": "Notifications",
  "/messages": "Messages",
  "/login": "Sign in",
  "/states": "Component states",
};

function titleFor(pathname: string | null): string {
  if (!pathname) return "SocialFlow";
  if (titles[pathname]) return titles[pathname]!;
  if (pathname.startsWith("/profile")) return "Profile";
  if (pathname.startsWith("/messages")) return "Messages";
  if (pathname.startsWith("/post")) return "Post";
  return "SocialFlow";
}

/**
 * The sticky top bar.
 *
 * On phones it carries the viewer's avatar and a search shortcut because the
 * rails are gone at that width. On wider screens it carries the page title and
 * a theme switch. Elevation is a data attribute toggled by a scroll listener,
 * so no React state changes on scroll.
 */
export function TopBar() {
  const pathname = usePathname();
  const router = useRouter();
  const ref = React.useRef<HTMLElement>(null);
  const { user, status, signOut } = useSession();
  const { theme, toggle, mounted } = useTheme();

  React.useEffect(() => attachHeaderElevation(ref.current), []);

  const onSignOut = React.useCallback(async () => {
    await signOut();
    notify.success("Signed out");
    router.push("/");
  }, [router, signOut]);

  return (
    <header
      ref={ref}
      data-elevated="false"
      className="sticky top-0 z-[var(--z-sticky)] flex h-[var(--layout-topbar)] items-center gap-2 border-b border-line bg-page/85 px-4 backdrop-blur-xl"
    >
      {user ? (
        <div className="shrink-0 md:hidden">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                aria-label="Account menu"
                className="rounded-full p-1.5 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
              >
                <Avatar name={user.name} handle={user.username} size="sm" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-[240px]">
              <div className="px-2.5 py-2">
                <p className="truncate text-sm font-semibold text-primary">{user.name}</p>
                <p className="truncate font-mono text-xs text-tertiary">@{user.username}</p>
              </div>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <Link href={`/profile/${user.username}`}>
                  <UserRound className="size-icon-xs" aria-hidden="true" />
                  Profile
                </Link>
              </DropdownMenuItem>
              <DropdownMenuItem onSelect={() => toggle()}>
                {theme === "dark" ? (
                  <Sun className="size-icon-xs" aria-hidden="true" />
                ) : (
                  <Moon className="size-icon-xs" aria-hidden="true" />
                )}
                {theme === "dark" ? "Light theme" : "Dark theme"}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => void onSignOut()}>
                <LogOut className="size-icon-xs" aria-hidden="true" />
                Sign out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ) : null}

      <h1 className="min-w-0 flex-1 truncate font-sans text-pane font-semibold tracking-ui text-primary">
        {titleFor(pathname)}
      </h1>

      <Link
        href="/explore"
        aria-label="Search"
        className="grid size-11 place-items-center rounded-full text-secondary transition-colors hover:bg-hover hover:text-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring md:hidden"
      >
        <Search className="size-icon-sm" aria-hidden="true" />
      </Link>

      <button
        type="button"
        onClick={toggle}
        aria-pressed={mounted ? theme === "light" : undefined}
        aria-label={theme === "light" ? "Switch to dark theme" : "Switch to light theme"}
        className="grid size-11 place-items-center rounded-full text-secondary transition-colors hover:bg-hover hover:text-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      >
        {mounted && theme === "light" ? (
          <Moon className="size-icon-sm" aria-hidden="true" />
        ) : (
          <Sun className="size-icon-sm" aria-hidden="true" />
        )}
      </button>

      {status === "anonymous" ? (
        <Link
          href="/login"
          className="inline-flex h-9 items-center gap-1.5 rounded-full bg-accent-solid px-4 text-sm font-semibold text-on-accent transition-colors hover:bg-accent-solid-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring md:hidden"
        >
          <LogIn className="size-icon-xs" aria-hidden="true" />
          Sign in
        </Link>
      ) : null}
    </header>
  );
}
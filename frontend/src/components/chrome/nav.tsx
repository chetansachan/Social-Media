"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  Bell,
  Home,
  LogIn,
  LogOut,
  Mail,
  Moon,
  MoreHorizontal,
  Search,
  Sun,
  UserRound,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Avatar } from "@/components/ui/avatar";
import { Tooltip } from "@/components/ui/tooltip";
import { useSession } from "@/lib/providers/session-provider";
import { useTheme } from "@/lib/providers/theme-provider";
import { LogoLink } from "./logo";
import { PostButton } from "@/components/feed/compose-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export type NavItem = {
  href: string;
  label: string;
  icon: React.ElementType;
  badge?: number;
};

function navItemsFor(profileHref: string): NavItem[] {
  return [
    { href: "/", label: "Home", icon: Home },
    { href: "/explore", label: "Explore", icon: Search },
    { href: "/notifications", label: "Notifications", icon: Bell },
    { href: "/messages", label: "Messages", icon: Mail },
    { href: profileHref, label: "Profile", icon: UserRound },
  ];
}

function isActive(pathname: string | null, href: string): boolean {
  if (!pathname) return false;
  if (href === "/") return pathname === "/";
  if (href === "/login") return pathname === "/login";
  return pathname === href || pathname.startsWith(`${href}/`);
}

function useNavItems(): NavItem[] {
  const { user } = useSession();
  return React.useMemo(
    () => navItemsFor(user ? `/profile/${user.username}` : "/login"),
    [user],
  );
}

function UnreadBadge({ unread, className }: { unread: number; className?: string }) {
  if (unread <= 0) return null;
  return (
    <span
      aria-hidden="true"
      className={cn(
        "absolute -right-1.5 -top-1 grid min-w-[18px] place-items-center rounded-full",
        "bg-accent-solid px-1 font-mono text-2xs font-semibold leading-4 text-on-accent",
        className,
      )}
    >
      {unread > 9 ? "9+" : unread}
    </span>
  );
}

/** The labelled rail, ≥1280px. */
export function SideRail({ unread = 0, messagesUnread = 0 }: { unread?: number; messagesUnread?: number }) {
  const pathname = usePathname();
  const items = useNavItems();

  return (
    <nav aria-label="Primary" className="flex h-full flex-col px-3 py-3">
      <div className="px-2 py-1.5">
        <LogoLink />
      </div>

      <ul className="mt-2 flex flex-col gap-0.5">
        {items.map((item) => {
          const active = isActive(pathname, item.href);
          const Icon = item.icon;
          return (
            <li key={item.label}>
              <Link
                href={item.href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "group inline-flex items-center gap-5 rounded-full px-3 py-3",
                  "text-nav transition-colors duration-200",
                  "hover:bg-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                  active
                    ? "font-semibold text-primary"
                    : "font-medium text-secondary hover:text-primary",
                )}
              >
                <span className="relative">
                  <Icon
                    className="size-icon-lg"
                    strokeWidth={active ? 2.6 : 1.9}
                    aria-hidden="true"
                  />
                  {item.href === "/notifications" ? (
                    <UnreadBadge unread={unread} />
                  ) : null}
                  {item.href === "/messages" ? (
                    <UnreadBadge unread={messagesUnread} />
                  ) : null}
                </span>
                <span className="pr-2">{item.label}</span>
                {item.href === "/notifications" && unread > 0 ? (
                  <span className="sr-only">{unread} unread</span>
                ) : null}
                {item.href === "/messages" && messagesUnread > 0 ? (
                  <span className="sr-only">{messagesUnread} unread</span>
                ) : null}
              </Link>
            </li>
          );
        })}
      </ul>

      <div className="mt-4 px-1">
        <PostButton className="h-[var(--control-height-xl)] w-full text-lg" />
      </div>

      <div className="mt-auto">
        <AccountChip />
        <Link
          href="/states"
          className="mt-2 block rounded-md px-3 py-1.5 font-mono text-2xs uppercase tracking-mono text-tertiary transition-colors hover:text-secondary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          Component states
        </Link>
      </div>
    </nav>
  );
}

/** The icon rail, 768–1279px. */
export function IconRail({ unread = 0, messagesUnread = 0 }: { unread?: number; messagesUnread?: number }) {
  const pathname = usePathname();
  const items = useNavItems();

  return (
    <nav aria-label="Primary" className="flex h-full flex-col items-center py-4">
      <LogoLink compact />

      <ul className="mt-5 flex flex-col items-center gap-1.5">
        {items.map((item) => {
          const active = isActive(pathname, item.href);
          const Icon = item.icon;
          const label =
            item.href === "/notifications" && unread > 0
              ? `${item.label}, ${unread} unread`
              : item.href === "/messages" && messagesUnread > 0
                ? `${item.label}, ${messagesUnread} unread`
                : item.label;

          return (
            <li key={item.label}>
              <Tooltip label={item.label}>
                <Link
                  href={item.href}
                  aria-label={label}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "relative grid size-[var(--control-height-xl)] place-items-center rounded-full",
                    "transition-colors duration-200",
                    "hover:bg-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                    active ? "text-primary" : "text-secondary",
                  )}
                >
                  <Icon
                    className="size-icon-lg"
                    strokeWidth={active ? 2.6 : 1.9}
                    aria-hidden="true"
                  />
                  {item.href === "/notifications" ? (
                    <UnreadBadge unread={unread} />
                  ) : null}
                  {item.href === "/messages" ? (
                    <UnreadBadge unread={messagesUnread} />
                  ) : null}
                </Link>
              </Tooltip>
            </li>
          );
        })}
      </ul>

      <div className="mt-4">
        <PostButton compact />
      </div>

      <div className="mt-auto flex flex-col items-center gap-3">
        <IconAccountChip />
        <Tooltip label="Component states">
          <Link
            href="/states"
            aria-label="Component states"
            className="grid size-10 place-items-center rounded-full font-mono text-2xs text-tertiary transition-colors hover:bg-hover hover:text-secondary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          >
            QA
          </Link>
        </Tooltip>
      </div>
    </nav>
  );
}

/** The bottom bar, ≤767px. Five destinations, 44px+ targets, safe-area inset. */
export function BottomNav({ unread = 0, messagesUnread = 0 }: { unread?: number; messagesUnread?: number }) {
  const pathname = usePathname();
  const items = useNavItems();

  return (
    <nav
      aria-label="Primary"
      className="fixed inset-x-0 bottom-0 z-[var(--z-sticky)] border-t border-line bg-page/95 backdrop-blur-xl safe-bottom"
    >
      <ul className="flex h-[var(--layout-bottom-bar)] items-stretch">
        {items.map((item) => {
          const active = isActive(pathname, item.href);
          const Icon = item.icon;
          return (
            <li key={item.label} className="flex-1">
              <Link
                href={item.href}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "relative flex h-full flex-col items-center justify-center gap-1",
                  "transition-colors duration-200",
                  "hover:bg-hover focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
                  active ? "font-semibold text-primary" : "font-medium text-secondary",
                )}
              >
                <span className="relative">
                  <Icon
                    className="size-icon-md"
                    strokeWidth={active ? 2.6 : 1.9}
                    aria-hidden="true"
                  />
                  {item.href === "/notifications" ? (
                    <UnreadBadge unread={unread} className="-right-2 -top-1.5" />
                  ) : null}
                  {item.href === "/messages" ? (
                    <UnreadBadge unread={messagesUnread} className="-right-2 -top-1.5" />
                  ) : null}
                </span>
                <span className="text-2xs font-medium leading-none">
                  {item.label}
                </span>
                {item.href === "/notifications" && unread > 0 ? (
                  <span className="sr-only">{unread} unread</span>
                ) : null}
                {item.href === "/messages" && messagesUnread > 0 ? (
                  <span className="sr-only">{messagesUnread} unread</span>
                ) : null}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

function useAccountMenu() {
  const { user, status, signOut } = useSession();
  const { theme, toggle } = useTheme();
  const router = useRouter();

  const onSignOut = React.useCallback(async () => {
    await signOut();
    router.push("/");
  }, [router, signOut]);

  return { user, status, signOut: onSignOut, theme, toggle };
}

function AccountChip() {
  const { user, status, signOut, theme, toggle } = useAccountMenu();

  if (status === "loading") {
    return <div className="sf-skeleton h-12 w-full rounded-full" aria-hidden="true" />;
  }

  if (!user) {
    return (
      <Link
        href="/login"
        className="flex h-[var(--control-height-xl)] w-full items-center justify-center gap-2 rounded-full bg-accent-solid text-base font-semibold text-on-accent transition-colors hover:bg-accent-solid-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      >
        <LogIn className="size-icon-sm" aria-hidden="true" />
        Sign in
      </Link>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="Account menu"
          className="flex w-full items-center gap-3 rounded-full p-2 text-left transition-colors hover:bg-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <Avatar name={user.name} handle={user.username} size="md" />
          <span className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-primary">
              {user.name}
            </span>
            <span className="block truncate font-mono text-xs text-tertiary">
              @{user.username}
            </span>
          </span>
          <MoreHorizontal className="size-icon-sm text-secondary" aria-hidden="true" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-[260px]">
        <DropdownMenuItem asChild>
          <Link href={`/profile/${user.username}`}>
            <UserRound className="size-icon-xs" aria-hidden="true" />
            Profile
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={toggle}>
          {theme === "dark" ? (
            <Sun className="size-icon-xs" aria-hidden="true" />
          ) : (
            <Moon className="size-icon-xs" aria-hidden="true" />
          )}
          {theme === "dark" ? "Light theme" : "Dark theme"}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void signOut()}>
          <LogOut className="size-icon-xs" aria-hidden="true" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function IconAccountChip() {
  const { user, theme, toggle, signOut } = useAccountMenu();

  if (!user) {
    return (
      <Tooltip label="Sign in">
        <Link
          href="/login"
          aria-label="Sign in"
          className="grid size-11 place-items-center rounded-full bg-accent-solid text-on-accent transition-colors hover:bg-accent-solid-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <LogIn className="size-icon-sm" aria-hidden="true" />
        </Link>
      </Tooltip>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="Account menu"
          className="rounded-full focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <Avatar name={user.name} handle={user.username} size="md" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" side="top" className="w-[240px]">
        <DropdownMenuItem asChild>
          <Link href={`/profile/${user.username}`}>
            <UserRound className="size-icon-xs" aria-hidden="true" />
            Profile
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={toggle}>
          {theme === "dark" ? (
            <Sun className="size-icon-xs" aria-hidden="true" />
          ) : (
            <Moon className="size-icon-xs" aria-hidden="true" />
          )}
          {theme === "dark" ? "Light theme" : "Dark theme"}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void signOut()}>
          <LogOut className="size-icon-xs" aria-hidden="true" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
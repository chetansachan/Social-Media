"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { LogOut, Moon, MoreHorizontal, SlidersHorizontal, Sun } from "lucide-react";
import { notify } from "@/lib/toast";
import { useSession } from "@/lib/providers/session-provider";
import { useTheme } from "@/lib/providers/theme-provider";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

/**
 * Account settings for phones, where the left rail is gone.
 *
 * The theme switch and sign-out live here rather than in a global top bar, so
 * every page can stay title-free without losing access to account controls.
 */
export function ProfileMenu() {
  const router = useRouter();
  const { signOut } = useSession();
  const { theme, toggle } = useTheme();

  const onSignOut = async () => {
    await signOut();
    notify.success("Signed out");
    router.push("/");
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label="Account settings"
          className="grid size-9 place-items-center rounded-full border border-line-interactive text-primary transition-colors hover:bg-hover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <MoreHorizontal className="size-icon-sm" aria-hidden="true" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[220px]">
        <DropdownMenuItem onSelect={() => toggle()}>
          {theme === "dark" ? (
            <Sun className="size-icon-xs" aria-hidden="true" />
          ) : (
            <Moon className="size-icon-xs" aria-hidden="true" />
          )}
          {theme === "dark" ? "Light theme" : "Dark theme"}
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link href="/states">
            <SlidersHorizontal className="size-icon-xs" aria-hidden="true" />
            Component states
          </Link>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void onSignOut()}>
          <LogOut className="size-icon-xs" aria-hidden="true" />
          Sign out
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

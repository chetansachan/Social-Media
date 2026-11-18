"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";
import { cn } from "@/lib/utils";

/**
 * The rail search. Submitting navigates to Explore with the term in the URL,
 * which keeps results shareable and the back button meaningful. `/` focuses it
 * from anywhere except an editable field.
 */
export function SearchBox({ className }: { className?: string }) {
  const router = useRouter();
  const [value, setValue] = React.useState("");
  const inputRef = React.useRef<HTMLInputElement>(null);

  React.useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey) return;
      const target = event.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable)
      ) {
        return;
      }
      event.preventDefault();
      inputRef.current?.focus();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  return (
    <form
      role="search"
      className={cn("relative", className)}
      onSubmit={(event) => {
        event.preventDefault();
        const query = value.trim();
        if (!query) return;
        router.push(`/explore?q=${encodeURIComponent(query)}`);
      }}
    >
      <Search
        className="pointer-events-none absolute left-4 top-1/2 size-icon-xs -translate-y-1/2 text-tertiary"
        aria-hidden="true"
      />
      <label htmlFor="rail-search" className="sr-only">
        Search posts, people, or tags
      </label>
      <input
        id="rail-search"
        ref={inputRef}
        type="text"
        autoComplete="off"
        value={value}
        onChange={(event) => setValue(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Escape") setValue("");
        }}
        placeholder="Search"
        maxLength={100}
        className="h-11 w-full rounded-full border border-line bg-inset pl-11 pr-10 text-sm text-primary placeholder:text-tertiary transition-colors focus-visible:border-accent focus-visible:bg-page focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring"
      />
      {value ? (
        <kbd className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 rounded border border-line px-1.5 py-0.5 font-mono text-2xs text-tertiary">
          ↵
        </kbd>
      ) : null}
    </form>
  );
}
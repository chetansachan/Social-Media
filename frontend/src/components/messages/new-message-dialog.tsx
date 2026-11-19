"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { PenLine, Search, UserRound } from "lucide-react";
import { cn } from "@/lib/utils";
import { notify } from "@/lib/toast";
import { useUserSearch } from "@/lib/hooks/use-user-search";
import { useStartConversation } from "@/lib/hooks/use-chat";
import { useSession } from "@/lib/providers/session-provider";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";

/**
 * Start a direct conversation from people search.
 *
 * `searchUsers` is the only people lookup the backend exposes; picking a result
 * calls `createConversation`, which returns the existing direct thread when one
 * already exists, then opens it.
 */
export function NewMessageDialog() {
  const router = useRouter();
  const { user } = useSession();
  const [open, setOpen] = React.useState(false);
  const [query, setQuery] = React.useState("");
  const [debounced, setDebounced] = React.useState("");
  const start = useStartConversation();

  React.useEffect(() => {
    const timer = setTimeout(() => setDebounced(query.trim()), 250);
    return () => clearTimeout(timer);
  }, [query]);

  const { people, isSearching } = useUserSearch(debounced);
  const results = people.filter((person) => person.id !== user?.id);

  const pick = (personId: string, name: string) => {
    start.mutate(personId, {
      onSuccess: (conversation) => {
        setOpen(false);
        setQuery("");
        router.push(`/messages/${conversation.id}`);
      },
      onError: (error) =>
        notify.error(`Could not start a conversation with ${name}`, error.message),
    });
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setQuery("");
          setDebounced("");
        }
      }}
    >
      <DialogTrigger asChild>
        <Button
          variant="secondary"
          size="sm"
          className="shrink-0"
          aria-label="New message"
        >
          <PenLine className="size-icon-xs" aria-hidden="true" />
          <span className="hidden sm:inline" aria-hidden="true">
            New message
          </span>
        </Button>
      </DialogTrigger>

      <DialogContent aria-describedby="new-message-description">
        <div className="sticky top-0 z-[1] flex h-[var(--layout-topbar)] items-center border-b border-line bg-page/90 px-4 backdrop-blur-xl">
          <DialogTitle>New message</DialogTitle>
        </div>
        <DialogDescription id="new-message-description" className="sr-only">
          Search for a person by name or handle to start a direct conversation.
        </DialogDescription>

        <div className="px-4 py-3">
          <div className="relative">
            <Search
              className="pointer-events-none absolute left-4 top-1/2 size-icon-xs -translate-y-1/2 text-tertiary"
              aria-hidden="true"
            />
            <label htmlFor="message-people-search" className="sr-only">
              Search people
            </label>
            <input
              id="message-people-search"
              autoFocus
              type="text"
              autoComplete="off"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search people"
              maxLength={100}
              className="h-11 w-full rounded-full border border-line bg-inset pl-11 pr-4 text-sm text-primary placeholder:text-tertiary focus-visible:border-accent focus-visible:bg-page focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring"
            />
          </div>
        </div>

        <div className="max-h-[50dvh] overflow-y-auto border-t border-line">
          {!debounced ? (
            <div className="px-8 py-10 text-center">
              <UserRound className="mx-auto size-icon-lg text-tertiary" aria-hidden="true" />
              <p className="mt-2 text-sm text-secondary">
                Search by name or handle to start a conversation.
              </p>
            </div>
          ) : isSearching && results.length === 0 ? (
            <div className="space-y-2 p-4" aria-hidden="true">
              <div className="sf-skeleton h-12 rounded-lg" />
              <div className="sf-skeleton h-12 rounded-lg" />
            </div>
          ) : results.length === 0 ? (
            <p className="px-6 py-10 text-center text-sm text-secondary">
              No people match &ldquo;{debounced}&rdquo;.
            </p>
          ) : (
            <ul>
              {results.map((person) => (
                <li key={person.id}>
                  <button
                    type="button"
                    onClick={() => pick(person.id, person.name)}
                    disabled={start.isPending}
                    className={cn(
                      "flex w-full items-center gap-3 border-b border-line px-4 py-3 text-left",
                      "transition-colors hover:bg-hover focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
                      "disabled:opacity-60",
                    )}
                  >
                    <Avatar name={person.name} handle={person.username} size="md" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-sans text-base font-semibold text-primary">
                        {person.name}
                      </span>
                      <span className="block truncate font-mono text-xs text-tertiary">
                        @{person.username}
                      </span>
                    </span>
                    <span className="shrink-0 text-xs font-semibold text-accent">
                      Message
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

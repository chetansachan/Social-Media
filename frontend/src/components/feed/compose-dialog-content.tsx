"use client";

import * as React from "react";
import { notify } from "@/lib/toast";
import { useCreatePost, type PostDraft } from "@/lib/hooks/use-feed";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { Composer } from "./composer";

/**
 * The compose modal body. Loaded lazily the first time Post is pressed so the
 * dialog, composer, and create-post mutation stay out of the shared shell
 * chunk on every route.
 */
export default function ComposeDialogContent({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const createPost = useCreatePost();

  const submit = React.useCallback(
    (draft: PostDraft) => {
      createPost.mutate(draft, {
        onSuccess: () => {
          onOpenChange(false);
          notify.success("Your post is live");
        },
        onError: (error) => notify.error(error.message),
      });
    },
    [createPost, onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby="compose-description">
        <div className="sticky top-0 z-[1] flex h-[var(--layout-topbar)] items-center border-b border-line bg-page/90 px-4 backdrop-blur-xl">
          <DialogTitle className="text-lg font-semibold">New post</DialogTitle>
        </div>
        <DialogDescription id="compose-description" className="sr-only">
          Write a post of up to 1000 characters and publish it to your followers.
        </DialogDescription>
        <Composer
          compact
          autoFocus
          onSubmit={submit}
          isPending={createPost.isPending}
          error={createPost.error?.message}
          placeholder="What is happening?!"
        />
      </DialogContent>
    </Dialog>
  );
}

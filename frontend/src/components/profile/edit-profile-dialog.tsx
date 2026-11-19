"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { notify } from "@/lib/toast";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useUpdateProfile } from "@/lib/hooks/use-profile";
import { useSession } from "@/lib/providers/session-provider";
import type { User } from "@/lib/types";

/**
 * Edit profile.
 *
 * Wires the real `UpdateUser` mutation. Only the fields the schema accepts are
 * shown; the handle is validated against the same pattern registration uses.
 */
export function EditProfileDialog({ user }: { user: User }) {
  const router = useRouter();
  const { refresh } = useSession();
  const update = useUpdateProfile();
  const [open, setOpen] = React.useState(false);
  const [name, setName] = React.useState(user.name);
  const [username, setUsername] = React.useState(user.username);
  const [isPublic, setIsPublic] = React.useState(user.isPublic);

  React.useEffect(() => {
    if (!open) return;
    setName(user.name);
    setUsername(user.username);
    setIsPublic(user.isPublic);
  }, [open, user]);

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    update.mutate(
      {
        id: user.id,
        name: name.trim(),
        username: username.trim(),
        isPublic,
      },
      {
        onSuccess: async (next) => {
          setOpen(false);
          notify.success("Profile updated");
          await refresh();
          if (next.username !== user.username) {
            router.replace(`/profile/${next.username}`);
          }
        },
        onError: (error) => notify.error(error.message),
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="secondary" size="md">
          Edit profile
        </Button>
      </DialogTrigger>
      <DialogContent aria-describedby="edit-profile-description">
        <div className="sticky top-0 z-[1] flex h-[var(--layout-topbar)] items-center justify-between border-b border-line bg-page/90 px-4 backdrop-blur-xl">
          <div className="flex items-center gap-6">
            <DialogTitle className="text-lg font-semibold">Edit profile</DialogTitle>
          </div>
        </div>
        <DialogDescription id="edit-profile-description" className="sr-only">
          Change your display name, handle, or account visibility.
        </DialogDescription>

        <form onSubmit={submit} className="space-y-4 px-4 py-4">
          <Field id="edit-name" label="Name">
            <Input
              id="edit-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoComplete="name"
              required
              maxLength={80}
            />
          </Field>
          <Field
            id="edit-username"
            label="Handle"
            hint="Letters, numbers, and underscores."
          >
            <Input
              id="edit-username"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="username"
              pattern="[A-Za-z0-9_]+"
              minLength={3}
              maxLength={30}
              required
            />
          </Field>

          <div className="flex items-start justify-between gap-4 rounded-lg border border-line px-4 py-3">
            <div>
              <label htmlFor="edit-private" className="text-sm font-semibold text-primary">
                Private account
              </label>
              <p id="edit-private-hint" className="mt-0.5 text-xs text-secondary">
                Only approved followers see your posts in their timeline.
              </p>
            </div>
            <Switch
              id="edit-private"
              checked={isPublic === false}
              onCheckedChange={(checked) => setIsPublic(!checked)}
              aria-describedby="edit-private-hint"
            />
          </div>

          {update.error ? (
            <p role="alert" className="text-xs text-danger">
              {update.error.message}
            </p>
          ) : null}

          <div className="flex justify-end gap-2 border-t border-line pt-4">
            <Button type="button" variant="secondary" size="sm" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" size="sm" loading={update.isPending} className="px-5">
              Save
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
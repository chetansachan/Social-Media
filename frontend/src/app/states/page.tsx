"use client";

import * as React from "react";
import { Button } from "@/components/ui/button";
import { Badge, TagChip } from "@/components/ui/badge";
import { Avatar } from "@/components/ui/avatar";
import { Skeleton } from "@/components/ui/skeleton";
import { Input, Textarea, Field } from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";
import { PostCard, PostSkeleton } from "@/components/feed/post-card";
import { Tombstone } from "@/components/feed/media-band";
import { demoPosts } from "@/lib/mock";
import { cn } from "@/lib/utils";

/**
 * /states — the seven-state QA surface (§5, V5).
 *
 * Every component in default, hover, focus-visible, active, disabled, loading
 * and error. Hover and active are CSS-driven, so they are demonstrated by
 * instruction rather than by a forced class: forcing them would show a state
 * the user can never actually reach.
 */
export default function StatesPage() {
  const [error, setError] = React.useState<string | undefined>(undefined);

  return (
    <>
      <div className="mx-auto w-full max-w-[var(--layout-feed-max)] space-y-8 p-4 pb-16">
        <header>
          <h1 className="font-display text-3xl text-primary">
            Component states
          </h1>
          <p className="mt-1.5 max-w-prose text-sm text-secondary">
            Every component in its seven states. Hover and active are driven by
            real interaction; focus is reachable with Tab.
          </p>
        </header>

        <Section title="Buttons">
          <Row label="Default">
            <Button>Primary</Button>
            <Button variant="secondary">Secondary</Button>
            <Button variant="outline">Outline</Button>
            <Button variant="ghost">Ghost</Button>
            <Button variant="danger">Danger</Button>
            <Button variant="link">Link</Button>
          </Row>
          <Row label="Sizes">
            <Button size="sm">Small</Button>
            <Button size="md">Medium</Button>
            <Button size="lg">Large</Button>
            <Button size="icon" aria-label="Icon button">
              <span aria-hidden="true">+</span>
            </Button>
          </Row>
          <Row label="Disabled">
            <Button disabled>Primary</Button>
            <Button variant="secondary" disabled>
              Secondary
            </Button>
            <Button variant="outline" disabled>
              Outline
            </Button>
          </Row>
          <Row label="Loading">
            <Button loading>Posting</Button>
            <Button variant="secondary" loading>
              Saving
            </Button>
          </Row>
        </Section>

        <Section title="Fields">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="s-default" label="Default">
              <Input id="s-default" placeholder="you@example.com" />
            </Field>
            <Field id="s-hint" label="With hint" hint="We never share this.">
              <Input id="s-hint" placeholder="you@example.com" />
            </Field>
            <Field id="s-error" label="Error" error="That email is already taken.">
              <Input id="s-error" aria-invalid="true" defaultValue="taken@example.com" />
            </Field>
            <Field id="s-disabled" label="Disabled">
              <Input id="s-disabled" disabled defaultValue="Locked" />
            </Field>
          </div>
          <div className="mt-4">
            <Field id="s-textarea" label="Textarea">
              <Textarea id="s-textarea" placeholder="What are you working on?" />
            </Field>
          </div>
        </Section>

        <Section title="Toggles">
          <div className="flex flex-wrap items-center gap-6">
            <label className="flex items-center gap-3 text-sm text-secondary">
              <Switch defaultChecked />
              On
            </label>
            <label className="flex items-center gap-3 text-sm text-secondary">
              <Switch />
              Off
            </label>
            <label className="flex items-center gap-3 text-sm text-tertiary">
              <Switch disabled defaultChecked />
              Disabled
            </label>
          </div>
        </Section>

        <Section title="Badges and tags">
          <Row label="Tones">
            <Badge tone="neutral">Neutral</Badge>
            <Badge tone="accent">Accent</Badge>
            <Badge tone="danger">Danger</Badge>
            <Badge tone="success">Success</Badge>
            <Badge tone="outline">Outline</Badge>
          </Row>
          <Row label="Tags">
            <TagChip tag="#systemdesign" />
            <TagChip tag="#golang" />
            <TagChip tag="#postgres" />
          </Row>
        </Section>

        <Section title="Avatars">
          <Row label="Sizes">
            <Avatar name="System Design" handle="systemdesign" size="xs" />
            <Avatar name="System Design" handle="systemdesign" size="sm" />
            <Avatar name="System Design" handle="systemdesign" size="md" />
            <Avatar name="System Design" handle="systemdesign" size="lg" />
            <Avatar name="System Design" handle="systemdesign" size="xl" />
          </Row>
        </Section>

        <Section title="Skeletons">
          <Row label="Primitives">
            <Skeleton className="h-3 w-24" />
            <Skeleton className="h-3 w-40" />
            <Skeleton className="size-10 rounded-full" />
          </Row>
          <div className="mt-3 space-y-3">
            <PostSkeleton />
            <PostSkeleton withMedia />
          </div>
        </Section>

        <Section title="Post card">
          <div className="space-y-3">
            <PostCard post={demoPosts[0]!} />
            <PostCard post={demoPosts[1]!} />
            <PostCard post={demoPosts[3]!} />
            <PostCard
              post={demoPosts[2]!}
              likeError="That did not go through. Try again."
            />
            <Tombstone />
          </div>
        </Section>

        <Section title="Error announcement">
          <p className="mb-3 text-sm text-secondary">
            A failed action explains itself on the control, not only in a toast.
          </p>
          <Button
            variant="secondary"
            onClick={() => setError(error ? undefined : "The gateway refused the request.")}
          >
            {error ? "Clear error" : "Trigger error"}
          </Button>
          {error ? (
            <p role="alert" className="mt-2 text-xs text-danger">
              {error}
            </p>
          ) : null}
        </Section>
      </div>
    </>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  const heading = title;
  return (
    <section aria-labelledby={`section-${heading}`}>
      <h2
        id={`section-${heading}`}
        className="mb-3 border-b border-line pb-2 font-mono text-2xs uppercase tracking-mono text-tertiary"
      >
        {title}
      </h2>
      {children}
    </section>
  );
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className={cn("mb-3 flex flex-wrap items-center gap-2.5")}>
      <span className="w-full font-mono text-2xs tracking-mono text-tertiary sm:w-20">
        {label}
      </span>
      {children}
    </div>
  );
}
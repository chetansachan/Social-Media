import { useState } from "react";
import { AppShell } from "../components/AppShell";
import { Button, IconButton } from "../components/Button";
import { Composer } from "../components/Composer";
import { FeedList } from "../components/FeedList";
import { Link } from "../components/Link";
import { PostCard, PostCardSkeleton } from "../components/PostCard";
import { SearchField, TextField } from "../components/Field";
import { TabPanel, TabSet } from "../components/Navigation";
import { demoPosts } from "../lib/mock";

/* ==========================================================================
   States — every component in every state.
   --------------------------------------------------------------------------
   This page exists because the component rules require each state to be
   inspectable, and because "it looks right" is not a test. Each block below is
   one component with its full state matrix side by side, so a reviewer can
   compare default / hover / focus-visible / disabled / loading / error without
   driving the app to trigger them.

   Acceptance criteria for this page are in frontend/design/Home-X-Guidelines.md.
   ========================================================================== */

export function StatesPage() {
  const [tab, setTab] = useState("states-default");
  const [query, setQuery] = useState("");
  const [loadingButton, setLoadingButton] = useState(false);

  return (
    <AppShell
      activeID="states"
      header={<h1 className="shell__title">Component states</h1>}
    >
      <TabSet
        ariaLabel="State groups"
        activeID={tab}
        onChange={setTab}
        items={[
          { id: "states-default", label: "Buttons" },
          { id: "states-fields", label: "Fields" },
          { id: "states-cards", label: "Cards" },
          { id: "states-lists", label: "Lists" },
        ]}
      />

      <TabPanel id="states-default" active={tab === "states-default"}>
        <section className="states">
          <StateBlock title="Buttons — variants and states">
            <StateRow label="default">
              <Button variant="primary">Post</Button>
              <Button variant="secondary">Reply</Button>
              <Button variant="ghost">Share</Button>
              <Button variant="danger">Delete</Button>
            </StateRow>
            <StateRow label="hover (inspect with pointer)">
              <Button variant="primary" data-force="hover">
                Post
              </Button>
              <Button variant="secondary" data-force="hover">
                Reply
              </Button>
            </StateRow>
            <StateRow label="focus-visible (press Tab)">
              <Button variant="primary" data-force="focus">
                Post
              </Button>
              <Button variant="secondary" data-force="focus">
                Reply
              </Button>
            </StateRow>
            <StateRow label="disabled">
              <Button variant="primary" disabled>
                Post
              </Button>
              <Button variant="secondary" disabled>
                Reply
              </Button>
            </StateRow>
            <StateRow label="loading">
              <Button
                variant="primary"
                loading={loadingButton}
                onClick={() => {
                  setLoadingButton(true);
                  window.setTimeout(() => setLoadingButton(false), 1500);
                }}
              >
                {loadingButton ? "Posting" : "Post"}
              </Button>
              <Button variant="secondary" loading>
                Reply
              </Button>
            </StateRow>
            <StateRow label="error">
              <Button variant="primary" error="The gateway rejected this post">
                Retry
              </Button>
            </StateRow>
            <StateRow label="icon buttons">
              <IconButton label="Refresh">⟳</IconButton>
              <IconButton label="Refresh" loading>
                ⟳
              </IconButton>
              <IconButton label="Refresh" disabled>
                ⟳
              </IconButton>
            </StateRow>
          </StateBlock>

          <StateBlock title="Links">
            <StateRow label="inline">
              <Link href="#/states">Inline link</Link>
            </StateRow>
            <StateRow label="nav, current">
              <Link href="#/states" variant="nav" current>
                Current page
              </Link>
              <Link href="#/" variant="nav">
                Another page
              </Link>
            </StateRow>
            <StateRow label="quiet, focus-visible">
              <Link href="#/states" variant="quiet">
                Open details
              </Link>
            </StateRow>
          </StateBlock>
        </section>
      </TabPanel>

      <TabPanel id="states-fields" active={tab === "states-fields"}>
        <section className="states">
          <StateBlock title="Text fields">
            <StateRow label="default">
              <TextField id="s-default" label="Display name" description="Shown on your posts." />
            </StateRow>
            <StateRow label="error">
              <TextField
                id="s-error"
                label="Display name"
                error="A display name is required."
                defaultValue=""
              />
            </StateRow>
            <StateRow label="disabled">
              <TextField id="s-disabled" label="Display name" disabled defaultValue="Locked" />
            </StateRow>
            <StateRow label="loading">
              <TextField id="s-loading" label="Username" loading defaultValue="checking…" />
            </StateRow>
          </StateBlock>

          <StateBlock title="Search">
            <SearchField
              id="s-search"
              label="Search"
              value={query}
              onChange={setQuery}
              loading={false}
            >
              <p className="search__hint">Results appear here. Try “golang”.</p>
            </SearchField>
          </StateBlock>

          <StateBlock title="Composer">
            <Composer onSubmit={() => Promise.resolve()} />
          </StateBlock>
        </section>
      </TabPanel>

      <TabPanel id="states-cards" active={tab === "states-cards"}>
        <section className="states">
          <StateBlock title="Cards">
            <StateRow label="default">
              <PostCard post={demoPosts[0]} />
            </StateRow>
            <StateRow label="long content, clamped">
              <PostCard post={demoPosts[2]} />
            </StateRow>
            <StateRow label="liked">
              <PostCard post={demoPosts[1]} />
            </StateRow>
            <StateRow label="skeleton">
              <PostCardSkeleton />
            </StateRow>
            <StateRow label="tombstone">
              <PostCard post={demoPosts[3]} deleted />
            </StateRow>
            <StateRow label="media failure">
              <PostCard post={demoPosts[2]} mediaError="The image could not be loaded." />
            </StateRow>
          </StateBlock>
        </section>
      </TabPanel>

      <TabPanel id="states-lists" active={tab === "states-lists"}>
        <section className="states">
          <StateBlock title="Lists">
            <StateRow label="loading">
              <FeedList
                ariaLabel="Loading list"
                items={[]}
                loading
                skeletonRows={3}
                renderItem={() => null}
                keyFor={() => "skeleton"}
                renderSkeleton={() => <PostCardSkeleton />}
                emptyTitle=""
              />
            </StateRow>
            <StateRow label="empty">
              <FeedList
                ariaLabel="Empty list"
                items={[]}
                renderItem={() => null}
                keyFor={() => "empty"}
                emptyTitle="Nothing here yet"
                emptyBody="This list is empty by design, to show the state."
                emptyAction={
                  <Button variant="primary" onClick={() => setTab("states-default")}>
                    Back to buttons
                  </Button>
                }
              />
            </StateRow>
            <StateRow label="error">
              <FeedList
                ariaLabel="Failed list"
                items={[]}
                error="The gateway did not answer."
                onRetry={() => {}}
                renderItem={() => null}
                keyFor={() => "error"}
                emptyTitle=""
              />
            </StateRow>
            <StateRow label="ready">
              <FeedList
                ariaLabel="Ready list"
                items={demoPosts}
                renderItem={(post) => <PostCard post={post} />}
                keyFor={(post) => post.id}
                emptyTitle=""
              />
            </StateRow>
          </StateBlock>
        </section>
      </TabPanel>
    </AppShell>
  );
}

function StateBlock({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="states__block">
      <h2 className="states__heading">{title}</h2>
      <div className="states__rows">{children}</div>
    </section>
  );
}

function StateRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="states__row">
      <p className="states__label">{label}</p>
      <div className="states__samples">{children}</div>
    </div>
  );
}

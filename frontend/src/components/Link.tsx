import type { AnchorHTMLAttributes, ReactNode } from "react";
import { press } from "../lib/motion";

/* ==========================================================================
   Link
   --------------------------------------------------------------------------
   States: default | hover | focus-visible | active | visited | disabled.
   A link is a navigation, never an action: it must not carry an aria-disabled
   that a screen reader announces as "unavailable" while it still navigates.
   Where navigation is genuinely unavailable it is rendered as a button or as
   plain text with an explanation, never as a dead anchor.

   Keyboard: Enter follows. Tab order is document order, which is why the skip
   link is the first focusable element in the page.
   Touch:    tap targets are at least control-height-sm tall.
   ========================================================================== */

export type LinkVariant = "inline" | "nav" | "quiet" | "inverse";

export interface LinkProps extends Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "className"> {
  variant?: LinkVariant;
  /** Marks the current page. Rendered as a link with aria-current, not a span. */
  current?: boolean;
  children: ReactNode;
}

export function Link({
  variant = "inline",
  current = false,
  children,
  onClick,
  href,
  ...rest
}: LinkProps) {
  return (
    <a
      {...rest}
      href={href}
      className={["link", `link--${variant}`, current ? "is-current" : ""].filter(Boolean).join(" ")}
      aria-current={current ? "page" : undefined}
      onClick={(event) => {
        press(event.currentTarget);
        onClick?.(event);
      }}
    >
      {children}
    </a>
  );
}

/* ---- Segmented navigation ------------------------------------------------ */

export interface NavItem {
  id: string;
  label: string;
  href: string;
  /** Screen-reader-only clarification, e.g. "3 unread". */
  badge?: string;
}

export interface SideNavProps {
  items: NavItem[];
  activeID: string;
  onNavigate?: (id: string) => void;
}

/**
 * The primary rail. roving tabindex: the list is one tab stop and the arrow
 * keys move within it, so a keyboard user does not have to tab through every
 * destination to reach the content.
 */
export function SideNav({ items, activeID, onNavigate }: SideNavProps) {
  const handleKeys = (event: React.KeyboardEvent<HTMLUListElement>) => {
    const keys = ["ArrowDown", "ArrowUp", "Home", "End"];
    if (!keys.includes(event.key)) return;

    const list = event.currentTarget;
    const links = Array.from(list.querySelectorAll<HTMLAnchorElement>("a"));
    const index = links.indexOf(document.activeElement as HTMLAnchorElement);
    if (index === -1) return;

    event.preventDefault();
    let next = index;
    if (event.key === "ArrowDown") next = (index + 1) % links.length;
    if (event.key === "ArrowUp") next = (index - 1 + links.length) % links.length;
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = links.length - 1;
    links[next].focus();
  };

  return (
    <nav className="sidenav" aria-label="Primary">
      <ul className="sidenav__list" role="list" onKeyDown={handleKeys}>
        {items.map((item) => {
          const active = item.id === activeID;
          return (
            <li key={item.id} className="sidenav__item">
              <Link
                href={item.href}
                variant="nav"
                current={active}
                onClick={(event) => {
                  // A hash router keeps the app single-page without a server
                  // route per view; still a real link so it can be opened in a
                  // new tab and middle-clicked.
                  if (item.href.startsWith("#")) event.preventDefault();
                  onNavigate?.(item.id);
                }}
              >
                <span className="sidenav__label">{item.label}</span>
                {item.badge && <span className="sidenav__badge">{item.badge}</span>}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

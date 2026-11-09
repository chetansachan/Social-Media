import { useEffect, useRef } from "react";
import { attachHeaderElevation } from "../lib/motion";
import { SideNav } from "../components/Link";
import { MobileNav } from "../components/Navigation";
import { useSession } from "../state/session";

export interface NavDestination {
  id: string;
  label: string;
  short: string;
  href: string;
  badge?: string;
}

export const destinations: NavDestination[] = [
  { id: "home", label: "Home", short: "Home", href: "#/" },
  { id: "explore", label: "Explore", short: "Explore", href: "#/explore" },
  { id: "messages", label: "Messages", short: "Messages", href: "#/messages", badge: "3" },
  { id: "notifications", label: "Notifications", short: "Alerts", href: "#/notifications", badge: "2" },
  { id: "profile", label: "Profile", short: "You", href: "#/profile" },
];

/* ==========================================================================
   AppShell
   --------------------------------------------------------------------------
   Density: 5 navigation primitives — the side rail, the in-content tab set (per
   page), the mobile rail, the skip link, and the breadcrumb. Layout is a
   three-column grid at desktop, two at tablet, one at mobile.

   The rail is a real <nav> with a heading, the main landmark is labelled, and
   the skip link is the first focusable element in the document.
   ========================================================================== */

export interface AppShellProps {
  activeID: string;
  children: React.ReactNode;
  /** Rendered in the main column's header, e.g. the page title + tab set. */
  header?: React.ReactNode;
}

export function AppShell({ activeID, children, header }: AppShellProps) {
  const headerRef = useRef<HTMLElement>(null);

  useEffect(() => {
    const detach = attachHeaderElevation(headerRef.current);
    return detach;
  }, []);

  return (
    <div className="shell">
      <a className="skip-link" href="#main">
        Skip to main content
      </a>

      <div className="shell__rail">
        <SideNav
          items={destinations.map((d) => ({ id: d.id, label: d.label, href: d.href, badge: d.badge }))}
          activeID={activeID}
        />
        <SessionChip />
      </div>

      <div className="shell__content">
        {header && (
          <header className="shell__header" ref={headerRef} data-elevated="false">
            {header}
          </header>
        )}
        <main className="shell__main" id="main" tabIndex={-1}>
          {children}
        </main>
      </div>

      <MobileNav
        items={destinations.map(({ id, label: l, short, href }) => ({
          id,
          label: short ?? l,
          href,
        }))}
        activeID={activeID}
        onNavigate={() => {}}
      />
    </div>
  );
}

function SessionChip() {
  const { signedIn } = useSession();
  return (
    <p className="shell__session" aria-live="polite">
      <span className={`shell__dot${signedIn ? " is-on" : ""}`} aria-hidden="true" />
      {signedIn ? "Connected to the gateway" : "Demo data — sign in to connect"}
    </p>
  );
}

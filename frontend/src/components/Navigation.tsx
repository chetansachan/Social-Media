import { useEffect, useRef, useState } from "react";
import { moveIndicator } from "../lib/motion";

/* ==========================================================================
   TabSet
   --------------------------------------------------------------------------
   Density: 5 navigation primitives per page is the design intent; the tab set
   is the in-content one (For you / Following / …).
   States: default | hover | focus-visible | selected | disabled.

   Keyboard: this is an ARIA tabs pattern, so the arrow keys move selection and
   only the selected tab is in the tab order (roving tabindex). Home/End jump.
   Touch: swipe is not required; the tab strip scrolls horizontally rather than
   wrapping, and the indicator shows the scroll position.
   ========================================================================== */

export interface TabItem {
  id: string;
  label: string;
  /** Shown after the label, e.g. an unread count. */
  badge?: number;
  disabled?: boolean;
  disabledReason?: string;
}

export interface TabSetProps {
  items: TabItem[];
  activeID: string;
  onChange: (id: string) => void;
  ariaLabel: string;
}

export function TabSet({ items, activeID, onChange, ariaLabel }: TabSetProps) {
  const stripRef = useRef<HTMLDivElement>(null);
  const indicatorRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    const strip = stripRef.current;
    if (!strip) return;
    const active = strip.querySelector<HTMLElement>('[aria-selected="true"]');
    moveIndicator(indicatorRef.current, active);
  }, [activeID]);

  const onKeys = (event: React.KeyboardEvent<HTMLDivElement>) => {
    const enabled = items.filter((item) => !item.disabled);
    const index = enabled.findIndex((item) => item.id === activeID);
    if (index === -1) return;

    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % enabled.length;
    else if (event.key === "ArrowLeft") next = (index - 1 + enabled.length) % enabled.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = enabled.length - 1;
    else return;

    event.preventDefault();
    onChange(enabled[next].id);
    stripRef.current
      ?.querySelector<HTMLElement>(`[data-tab="${enabled[next].id}"]`)
      ?.focus();
  };

  return (
    <div className="tabs" ref={stripRef} role="tablist" aria-label={ariaLabel} onKeyDown={onKeys}>
      {items.map((item) => {
        const selected = item.id === activeID;
        return (
          <button
            key={item.id}
            type="button"
            role="tab"
            data-tab={item.id}
            id={`tab-${item.id}`}
            aria-selected={selected}
            aria-controls={`panel-${item.id}`}
            tabIndex={selected ? 0 : -1}
            disabled={item.disabled}
            aria-describedby={item.disabled && item.disabledReason ? `${item.id}-why` : undefined}
            className={`tabs__tab${selected ? " is-selected" : ""}`}
            onClick={() => onChange(item.id)}
          >
            <span className="tabs__label">{item.label}</span>
            {item.badge !== undefined && item.badge > 0 && (
              <span className="tabs__badge">{item.badge}</span>
            )}
            {item.disabled && item.disabledReason && (
              <span id={`${item.id}-why`} className="visually-hidden">
                {item.disabledReason}
              </span>
            )}
          </button>
        );
      })}
      <span className="tabs__indicator" ref={indicatorRef} aria-hidden="true" />
    </div>
  );
}

export interface TabPanelProps {
  id: string;
  active: boolean;
  children: React.ReactNode;
}

/**
 * Inactive panels are removed from the DOM rather than hidden, so their
 * content is not reachable by a screen reader or by Tab while off-screen.
 * The panel is labelled by its tab.
 */
export function TabPanel({ id, active, children }: TabPanelProps) {
  if (!active) return null;
  return (
    <div role="tabpanel" id={`panel-${id}`} aria-labelledby={`tab-${id}`} tabIndex={0} className="tabs__panel">
      {children}
    </div>
  );
}

/* ---- Mobile navigation --------------------------------------------------- */

export interface MobileNavProps {
  items: Array<{ id: string; label: string; href: string }>;
  activeID: string;
  onNavigate: (id: string) => void;
}

/**
 * The small-screen rail. On a coarse pointer the desktop side nav is hidden by
 * CSS, and this one is the only navigation, so it must never overflow: labels
 * shorten rather than wrap, and the strip scrolls.
 */
export function MobileNav({ items, activeID, onNavigate }: MobileNavProps) {
  const [overflowing, setOverflowing] = useState(false);
  const ref = useRef<HTMLElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const check = () => setOverflowing(el.scrollWidth > el.clientWidth + 1);
    check();
    window.addEventListener("resize", check);
    return () => window.removeEventListener("resize", check);
  }, [items]);

  return (
    <nav
      className="mobilenav"
      aria-label="Primary"
      ref={ref}
      data-overflowing={overflowing || undefined}
    >
      <ul className="mobilenav__list" role="list">
        {items.map((item) => (
          <li key={item.id} className="mobilenav__item">
            <a
              href={item.href}
              className={`mobilenav__link${item.id === activeID ? " is-current" : ""}`}
              aria-current={item.id === activeID ? "page" : undefined}
              onClick={(event) => {
                if (item.href.startsWith("#")) event.preventDefault();
                onNavigate(item.id);
              }}
            >
              {item.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

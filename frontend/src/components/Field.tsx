import { useCallback, useEffect, useRef, useState } from "react";
import type { InputHTMLAttributes, ReactNode } from "react";

/* ==========================================================================
   Text fields
   --------------------------------------------------------------------------
   States: default | hover | focus-visible | filled | error | disabled |
           loading (search only).
   A field is never bare: label, description, and error all travel with it, so
   the accessible name and description exist without ARIA wiring on the page.

   Keyboard: Tab reaches the field; Escape clears (search only) and returns
             focus to the trigger that opened it.
   Touch:    control-height keeps the target above the 24px minimum, and the
             clear button is a sibling, not an overlay.
   ========================================================================== */

export interface TextFieldProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, "className" | "id"> {
  id: string;
  label: string;
  /** Helper copy shown while the field is valid. Replaced by the error. */
  description?: string;
  error?: string;
  loading?: boolean;
  icon?: ReactNode;
}

export function TextField({
  id,
  label,
  description,
  error,
  loading = false,
  icon,
  children,
  ...rest
}: TextFieldProps) {
  const describedBy = error ? `${id}-error` : description ? `${id}-description` : undefined;

  return (
    <div className={["field", error ? "is-error" : "", loading ? "is-loading" : ""].filter(Boolean).join(" ")}>
      <label className="field__label" htmlFor={id}>
        {label}
      </label>
      <div className="field__control">
        {icon && (
          <span className="field__icon" aria-hidden="true">
            {icon}
          </span>
        )}
        <input
          {...rest}
          id={id}
          className="field__input"
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          aria-busy={loading || undefined}
          data-loading={loading || undefined}
        />
        {loading && <span className="field__spinner" aria-hidden="true" />}
      </div>
      {error ? (
        // role=alert so the message is announced on blur, not only when the
        // element is described by the field.
        <p className="field__message field__message--error" id={`${id}-error`} role="alert">
          {error}
        </p>
      ) : description ? (
        <p className="field__message" id={`${id}-description`}>
          {description}
        </p>
      ) : null}
    </div>
  );
}

/* ---- Search -------------------------------------------------------------- */

export interface SearchFieldProps {
  id: string;
  label?: string;
  value: string;
  onChange: (value: string) => void;
  onSubmit?: (value: string) => void;
  /** Debounce window in ms before onSubmit fires. 0 submits immediately. */
  debounceMs?: number;
  loading?: boolean;
  error?: string;
  /** Render the result list in a popover (combobox pattern) instead of inline. */
  children?: ReactNode;
}

/**
 * A combobox-shaped search field: the input owns the value, the results are a
 * listbox, and the arrow keys move through them.
 *
 * Long-content handling: results are capped by the caller and the listbox
 * scrolls; the input never grows and never moves the submit button.
 */
export function SearchField({
  id,
  label = "Search",
  value,
  onChange,
  onSubmit,
  debounceMs = 250,
  loading = false,
  error,
  children,
}: SearchFieldProps) {
  const listboxID = `${id}-results`;
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!debounceMs || !onSubmit) return;
    const timer = window.setTimeout(() => onSubmit(value), debounceMs);
    return () => window.clearTimeout(timer);
  }, [value, debounceMs, onSubmit]);

  useEffect(() => {
    setActiveIndex(-1);
    setOpen(Boolean(children));
  }, [children]);

  const clear = useCallback(() => {
    onChange("");
    setActiveIndex(-1);
  }, [onChange]);

  return (
    <div className="search" ref={containerRef}>
      <form
        className="search__form"
        role="search"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit?.(value);
        }}
      >
        <TextField
          id={id}
          label={label}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          error={error}
          loading={loading}
          type="search"
          role="combobox"
          aria-expanded={open && Boolean(children)}
          aria-controls={listboxID}
          aria-autocomplete="list"
          aria-activedescendant={
            activeIndex >= 0 ? `${listboxID}-option-${activeIndex}` : undefined
          }
          onKeyDown={(event) => {
            const options = Array.from(
              containerRef.current?.querySelectorAll<HTMLElement>('[role="option"]') ?? [],
            );
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
              event.preventDefault();
              if (options.length === 0) return;
              const delta = event.key === "ArrowDown" ? 1 : -1;
              setActiveIndex((current) => {
                const next = (current + delta + options.length) % options.length;
                options[next].focus();
                return next;
              });
            }
            if (event.key === "Escape") {
              clear();
              event.currentTarget.blur();
            }
          }}
          icon={<SearchIcon />}
        />
        {value !== "" && (
          <button type="button" className="search__clear" onClick={clear}>
            Clear search
          </button>
        )}
      </form>
      {open && children && (
        <div className="search__results" role="listbox" id={listboxID}>
          {children}
        </div>
      )}
    </div>
  );
}

function SearchIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.5-3.5" />
    </svg>
  );
}

import type { ButtonHTMLAttributes, ReactNode } from "react";
import { press } from "../lib/motion";

/* ==========================================================================
   Button
   --------------------------------------------------------------------------
   State matrix (required by the component rules):
     default | hover | focus-visible | active | disabled | loading | error
   Pointer, touch, and keyboard behaviour is defined once here so no screen
   re-invents it.

   Keyboard:  Enter and Space activate. Space must not scroll the page, which is
              why keydown is intercepted rather than relying on click.
   Pointer:   hover and active are CSS states; no JS.
   Touch:     :active is suppressed under a coarse pointer so a tap does not
              leave the button looking pressed.
   ========================================================================== */

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md" | "lg";

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "className"> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Renders the spinner and blocks activation. Keeps the label so width is stable. */
  loading?: boolean;
  /** A submit/action failure. The button stays operable so the user can retry. */
  error?: string;
  /** Trailing content (icon, counter). */
  iconAfter?: ReactNode;
  fullWidth?: boolean;
  children: ReactNode;
}

export function Button({
  variant = "secondary",
  size = "md",
  loading = false,
  error,
  iconAfter,
  fullWidth = false,
  children,
  disabled,
  onClick,
  type = "button",
  ...rest
}: ButtonProps) {
  // A loading button is disabled, but a disabled button is not necessarily
  // loading. Conflating the two loses the spinner and the reason.
  const isDisabled = disabled || loading;

  return (
    <button
      {...rest}
      type={type}
      className={[
        "btn",
        `btn--${variant}`,
        `btn--${size}`,
        fullWidth ? "btn--block" : "",
        loading ? "is-loading" : "",
        error ? "is-error" : "",
      ]
        .filter(Boolean)
        .join(" ")}
      disabled={isDisabled}
      aria-disabled={isDisabled || undefined}
      aria-busy={loading || undefined}
      data-loading={loading || undefined}
      // Space is a page-scroll key; swallowing it here is what makes a button
      // behave like a button on both platforms.
      onKeyDown={(event) => {
        if (event.key === " " || event.key === "Spacebar") event.preventDefault();
        rest.onKeyDown?.(event);
      }}
      onClick={(event) => {
        if (loading) {
          event.preventDefault();
          return;
        }
        press(event.currentTarget);
        onClick?.(event);
      }}
    >
      {loading && <span className="btn__spinner" aria-hidden="true" />}
      <span className="btn__label">{children}</span>
      {iconAfter && <span className="btn__icon-after">{iconAfter}</span>}
      {/* The failure is announced rather than only coloured. */}
      {error && (
        <span role="alert" className="btn__error">
          {error}
        </span>
      )}
    </button>
  );
}

export interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "className" | "children"> {
  /** Accessible name. Required: an icon-only control without one is unusable. */
  label: string;
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  error?: string;
  children: ReactNode;
}

export function IconButton({
  label,
  variant = "ghost",
  size = "md",
  loading = false,
  error,
  children,
  disabled,
  onClick,
  type = "button",
  ...rest
}: IconButtonProps) {
  const isDisabled = disabled || loading;

  return (
    <button
      {...rest}
      type={type}
      className={["btn", "btn--icon", `btn--${variant}`, `btn--${size}`, error ? "is-error" : ""]
        .filter(Boolean)
        .join(" ")}
      disabled={isDisabled}
      aria-disabled={isDisabled || undefined}
      aria-busy={loading || undefined}
      aria-label={label}
      title={label}
      onKeyDown={(event) => {
        if (event.key === " " || event.key === "Spacebar") event.preventDefault();
        rest.onKeyDown?.(event);
      }}
      onClick={(event) => {
        if (loading) {
          event.preventDefault();
          return;
        }
        press(event.currentTarget);
        onClick?.(event);
      }}
    >
      <span aria-hidden="true">{children}</span>
      {error && (
        <span role="alert" className="visually-hidden">
          {error}
        </span>
      )}
    </button>
  );
}

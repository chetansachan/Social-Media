import { useId, useState } from "react";

/* ==========================================================================
   Composer
   --------------------------------------------------------------------------
   States: default | focus-visible | typing | over-limit | disabled | submitting
           | error.
   Long-content handling: the textarea grows with its content up to a measured
   maximum, then scrolls. The counter is always visible past 80% of the limit,
   and the submit control states the remaining count rather than only turning
   red. A over-limit composer blocks submission with an explanation, not a
   disabled button with no reason.
   ========================================================================== */

export interface ComposerProps {
  onSubmit: (text: string) => Promise<void> | void;
  placeholder?: string;
  maxLength?: number;
  /** Disables the control and explains why, e.g. "Sign in to post". */
  disabledReason?: string;
}

const MAX_LENGTH = 1000;
const MAX_HEIGHT = 240;

export function Composer({
  onSubmit,
  placeholder = "What is worth sharing?",
  maxLength = MAX_LENGTH,
  disabledReason,
}: ComposerProps) {
  const id = useId();
  const [text, setText] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>();

  const remaining = maxLength - [...text].length;
  const overLimit = remaining < 0;
  const disabled = Boolean(disabledReason) || overLimit || text.trim().length === 0;

  const submit = async () => {
    if (disabled || submitting) return;
    setSubmitting(true);
    setError(undefined);
    try {
      await onSubmit(text.trim());
      setText("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not post. Try again.");
    } finally {
      setSubmitting(false);
    }
  };

  if (disabledReason) {
    return (
      <div className="composer composer--locked">
        <p className="composer__locked-body">{disabledReason}</p>
      </div>
    );
  }

  return (
    <form
      className={`composer${overLimit ? " is-error" : ""}`}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <label className="composer__label" htmlFor={`${id}-text`}>
        New post
      </label>
      <textarea
        id={`${id}-text`}
        className="composer__input"
        value={text}
        placeholder={placeholder}
        rows={3}
        maxLength={maxLength}
        aria-describedby={`${id}-counter`}
        aria-invalid={overLimit || undefined}
        onChange={(event) => {
          setText(event.target.value);
          // The textarea grows until it hits the measured cap, then scrolls.
          const el = event.currentTarget;
          el.style.height = "auto";
          el.style.height = `${Math.min(el.scrollHeight, MAX_HEIGHT)}px`;
        }}
        onKeyDown={(event) => {
          // Cmd/Ctrl+Enter posts; Escape blurs so a keyboard user can leave
          // without reaching for the mouse.
          if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
            event.preventDefault();
            void submit();
          }
          if (event.key === "Escape") event.currentTarget.blur();
        }}
      />

      <div className="composer__footer">
        <span
          className={`composer__counter${remaining <= maxLength * 0.2 ? " is-tight" : ""}`}
          id={`${id}-counter`}
          aria-live="polite"
        >
          {overLimit
            ? `${Math.abs(remaining)} over the ${maxLength} character limit`
            : `${remaining} characters left`}
        </span>
        <button
          type="submit"
          className="btn btn--primary btn--md"
          disabled={disabled || submitting}
          aria-disabled={disabled || submitting || undefined}
          aria-busy={submitting || undefined}
        >
          {submitting ? "Posting…" : "Post"}
        </button>
      </div>

      {overLimit && (
        <p className="composer__error" role="alert">
          Shorten the post to {maxLength} characters to publish it.
        </p>
      )}
      {error && (
        <p className="composer__error" role="alert">
          {error}
        </p>
      )}
      <p className="composer__hint">Press Cmd or Ctrl plus Enter to post.</p>
    </form>
  );
}

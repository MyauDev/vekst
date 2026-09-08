/**
 * What a screen shows when it cannot show its content: empty, loading, failed.
 *
 * `docs/FRONTEND_PLAN.md` gap 9 counts nine of these across three screens and
 * notes they are "most of what a reviewer sees on a Demo day, because the happy
 * path is fast". They are written here as what a real customer meets on their
 * first morning, not as placeholders describing unbuilt work, so they survive
 * into the finished screens rather than being thrown away.
 *
 * Shape follows `docs/DESIGN.md` §14: no card, no centred illustration, no
 * oversized icon. A rule, a line of type that says what is missing, and an
 * action only where one is actually wired.
 */
import type { ReactNode } from "react";

/**
 * An empty state names what is missing and, where it can, what to do about it.
 *
 * `action` is deliberately optional and deliberately absent rather than
 * disabled where a capability does not exist yet: a control that cannot work is
 * worse than no control, because it invites the click and then explains
 * nothing.
 */
export function EmptyState({
  title,
  detail,
  action,
}: {
  title: string;
  detail: string;
  action?: ReactNode;
}) {
  return (
    <div className="border-t border-border py-8">
      <p className="text-base font-medium text-text">{title}</p>
      <p className="mt-1 max-w-prose text-sm text-text-muted">{detail}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

/**
 * A loading placeholder. **Static, on purpose.**
 *
 * §5 restricts motion in the application to the drill-down panel and sets
 * everything else to zero, so this does not shimmer or pulse. The library
 * default is an animated gradient; a report that is still computing is exactly
 * the impression this product must not give, and the same reasoning that
 * forbids animating a figure forbids animating the space where one will be.
 *
 * `rows` should match what is being waited for, so the page does not jump when
 * the content lands.
 */
export function Skeleton({ rows = 3, className = "" }: { rows?: number; className?: string }) {
  return (
    <div className={`flex flex-col gap-2 ${className}`} aria-hidden="true">
      {Array.from({ length: rows }, (_, i) => (
        <div
          key={i}
          className="h-4 bg-surface-sunken"
          // A ragged right edge reads as text rather than as a table of blocks.
          style={{ width: `${88 - (i % 3) * 14}%` }}
        />
      ))}
    </div>
  );
}

/** Announces a wait to a screen reader, which sees no skeleton. */
export function Loading({ label, rows }: { label: string; rows?: number }) {
  return (
    <div role="status" aria-live="polite" className="border-t border-border py-6">
      <span className="sr-only">{label}</span>
      <Skeleton rows={rows} />
    </div>
  );
}

/**
 * A failure, stated in the reader's language.
 *
 * The backend returns codes and never sentences, so the caller passes a
 * translated message. Nothing here ever renders a raw code: a person cannot act
 * on `invalid_flow`, and showing it is how an error surface stops being read.
 */
export function ErrorState({
  message,
  retry,
  retryLabel,
}: {
  message: string;
  retry?: () => void;
  retryLabel?: string;
}) {
  return (
    <div role="alert" className="border-t border-danger py-6">
      <p className="text-sm font-medium text-danger">{message}</p>
      {retry && retryLabel ? (
        <button
          type="button"
          onClick={retry}
          className="mt-3 border-b border-text pb-0.5 text-2xs font-semibold uppercase tracking-widest text-text"
        >
          {retryLabel}
        </button>
      ) : null}
    </div>
  );
}

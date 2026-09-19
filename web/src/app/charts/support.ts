/**
 * What every chart in `WORKFLOW.md` §5.3 shares, pulled out once rather than
 * written six times — `DESIGN.md` §14.2's chart-shell reasoning applies to the
 * logic underneath the shell as much as the markup inside it.
 */
import { localeTag } from "../../money";
import type { Locale } from "../../i18n";

/**
 * bklit's charts measure their container with `@visx/responsive`'s
 * `ParentSize`, which needs a real `ResizeObserver` -- jsdom has none. Where
 * a chart cannot be measured it never reports a size, `ParentSize` never
 * renders its children, and the reader would see an empty card. Every chart
 * in this module falls back to its table view instead — not a placeholder,
 * the same figures in the form `WORKFLOW.md` §5.3 already requires reachable
 * from every chart.
 */
export function canRenderChart(): boolean {
  return typeof ResizeObserver !== "undefined";
}

/** Fails toward "do not animate" in any environment that cannot answer, the
 *  same posture `LandingPage.tsx`'s `prefersReducedMotionOrNoJS` uses. */
export function reducedMotion(): boolean {
  try {
    return (
      typeof window === "undefined" ||
      typeof window.matchMedia !== "function" ||
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    );
  } catch {
    return true;
  }
}

/** `ru` groups with a space; a chart library's own default is always a comma.
 *  §4 reaches an axis exactly as it reaches a table cell. */
export function axisNumberFormatter(locale: Locale): (v: number) => string {
  const fmt = new Intl.NumberFormat(localeTag(locale));
  return (v: number) => fmt.format(v);
}

/**
 * §8.15: a chart may draw itself in; a figure inside it may not count up.
 * bklit's charts animate via `motion` springs rather than a CSS transition,
 * so the global `prefers-reduced-motion` floor in `index.css` §4 does not
 * reach them on its own -- every chart in this module passes this to its
 * `animate` prop instead of hard-coding `true`.
 */
export function chartsAnimate(): boolean {
  return !reducedMotion();
}

export interface Sliced<T> {
  readonly label: string;
  readonly value: number;
  readonly item: T;
}

export interface Folded<T> {
  /** At most `limit` entries, each a real category. */
  kept: readonly Sliced<T>[];
  /** Present only when there was a tail to fold. */
  other?: { label: string; value: number };
}

/**
 * §13.2 and `WORKFLOW.md` §5.3: a ninth category folds into one "Other"
 * rather than a generated hue. Sorted descending by magnitude first, so what
 * gets folded is always the smallest — the categorical bar (8.9) and the
 * Sankey (8.8) both call this rather than each inventing the cutoff.
 */
export function foldTopN<T>(
  items: readonly Sliced<T>[],
  limit: number,
  otherLabel: string,
): Folded<T> {
  const sorted = [...items].sort((a, b) => b.value - a.value);
  if (sorted.length <= limit) return { kept: sorted };
  const kept = sorted.slice(0, limit);
  const tail = sorted.slice(limit);
  return {
    kept,
    other: { label: otherLabel, value: tail.reduce((sum, s) => sum + s.value, 0) },
  };
}

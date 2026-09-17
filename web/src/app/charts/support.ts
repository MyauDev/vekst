/**
 * What every chart in `WORKFLOW.md` §5.3 shares, pulled out once rather than
 * written six times — `DESIGN.md` §14.2's chart-shell reasoning applies to the
 * logic underneath the shell as much as the markup inside it.
 */
import { localeTag } from "../../money";
import type { Locale } from "../../i18n";

/**
 * A canvas needs a 2D context, and jsdom has none. Where a chart cannot be
 * drawn, every chart in this module falls back to its table view — not as a
 * placeholder, but as the same figures in the form `WORKFLOW.md` §5.3 already
 * requires reachable from every chart.
 */
export function canDrawCanvas(): boolean {
  try {
    if (typeof document === "undefined") return false;
    return document.createElement("canvas").getContext("2d") !== null;
  } catch {
    return false;
  }
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

/**
 * Reads a raw `--vk-*` custom property from the document.
 *
 * A canvas inherits no token the way an element does, so this is how a chart
 * stays inside the same token layer as everything else: read at render time,
 * re-read when the theme changes. jsdom applies no stylesheet (the existing
 * precedent is `report.test.tsx`'s "both palettes" describe block), so this
 * returns `""` under test — every caller treats an empty string as "let
 * ECharts use its structural default," never as a crash.
 */
export function chartToken(name: string): string {
  if (typeof document === "undefined") return "";
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/** `ru` groups with a space; ECharts' own default is always a comma. §4
 *  reaches an axis exactly as it reaches a table cell. */
export function axisNumberFormatter(locale: Locale): (v: number) => string {
  const fmt = new Intl.NumberFormat(localeTag(locale));
  return (v: number) => fmt.format(v);
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

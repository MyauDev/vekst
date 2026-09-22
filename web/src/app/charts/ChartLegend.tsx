/**
 * `DESIGN.md` §13.5's legend: "a legend wherever two or more series appear".
 *
 * bklit ships no legend primitive, so this is plain JSX -- and it was plain
 * JSX twice, once in `RevenueExpenseChart` and once in `CashFlowChart`, which
 * is exactly the shape §13.5 makes recurring: any chart that gains a second
 * series needs one and there is only one right way to draw it.
 *
 * `data-chart-legend` marks it for `charts.test.tsx`, which asserts a legend's
 * presence on the two-series charts and its absence on the single-series ones
 * -- a legend on one series is the "an icon beside every label" mistake in
 * chart form.
 *
 * §13.5 again: text wears text colours, never a series colour. The swatch
 * carries the identity and the label is `text-text-muted` like any other
 * label, which is also what keeps the pairing legible to a reader who cannot
 * separate the two hues.
 */
import type { ReactNode } from "react";

export interface LegendItem {
  /** A token, never a literal -- `scripts/check-web-tokens.sh`. */
  color: string;
  label: ReactNode;
}

export function ChartLegend({ items }: Readonly<{ items: readonly LegendItem[] }>) {
  return (
    <div data-chart-legend className="mb-3 flex items-center gap-5 text-2xs text-text-muted">
      {items.map((item) => (
        <span key={String(item.label)} className="flex items-center gap-1.5">
          <span
            aria-hidden="true"
            className="h-2 w-2 rounded-full"
            style={{ backgroundColor: item.color }}
          />
          {item.label}
        </span>
      ))}
    </div>
  );
}

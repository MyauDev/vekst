/**
 * Top expense categories — horizontal bar, sorted. `WORKFLOW.md` §5.3, task
 * 8.9. Built on the vendored `@bklit/bar-chart`.
 *
 * One hue, slot 1, every bar the same step (§13.4): bar length already
 * encodes magnitude, so colouring bars by their own value spends the
 * identity channel re-encoding what length already shows.
 */
import { useMemo } from "react";
import { BarChart } from "@/components/charts/bar-chart";
import { Bar } from "@/components/charts/bar";
import { Grid } from "@/components/charts/grid";
import { BarYAxis } from "@/components/charts/bar-y-axis";
import { ChartTooltip } from "@/components/charts/tooltip";

import { NO_DATA, exponentOf, formatMinorUnits } from "../../money";
import type { Locale } from "../../i18n";
import type { Report } from "../../data/report";
import { ChartShell } from "./ChartShell";
import { CategoryTable } from "./ChartTable";
import { chartsAnimate, foldTopN } from "./support";
import type { Sliced } from "./support";

const MAX_SLOTS = 8;

interface Row {
  categoryId: string;
}

/**
 * One bar per non-revenue, non-computed line -- the real `GetManagementPNL`
 * has no category-level breakdown at all (`core/internal/report/pnl.go`'s
 * `Order` is twelve rows: seven sections and five computed results, never
 * one row per leaf category), so this chart's bars are section totals
 * (CS, OCS, OPEX, OIE, FR, CIT) rather than the individual expense
 * categories the fixture invented. Coarser than before, and real.
 *
 * `pnl.go`'s own `present()` prints a cost section positive -- "a cost is
 * negative in the store and positive on the page" -- but that inversion runs
 * both ways: a section that nets to a *credit* for the range (a currency
 * gain booked under Financial result, a refund under Cost of Sales) comes
 * back negative, on purpose, because it was not a cost that period. This is
 * "top expense categories", not "top category magnitudes" -- a line that
 * prints negative here is not an expense at all, however large, and
 * `Math.abs()`-ing it back to positive would draw a real gain as though it
 * were this range's second-biggest cost. Excluded, not flipped.
 */
export function rows(report: Report, locale: Locale) {
  const exp = exponentOf(report.currencyCode);
  const sliced: Sliced<Row>[] = report.lines
    .filter((line) => !line.computed && line.categoryId !== "01")
    .filter((line) => BigInt(line.total.minorUnits) > 0n)
    .map((line) => {
      const minor = BigInt(line.total.minorUnits);
      return {
        label: line.label,
        value: exp === undefined ? 0 : Number(minor) / 10 ** exp,
        item: { categoryId: line.categoryId },
      };
    });

  const { kept, other } = foldTopN(sliced, MAX_SLOTS, "—");
  const named = other
    ? [...kept, { label: other.label, value: other.value, item: { categoryId: "other" } }]
    : kept;

  return named.map((r) => ({
    name: r.label,
    value: r.value,
    text: exp === undefined ? NO_DATA : formatMinorUnits(String(Math.round(r.value * 10 ** exp)), exp, locale),
  }));
}

export function ExpenseCategoriesChart({
  report,
  locale,
}: Readonly<{ report: Report; locale: Locale }>) {
  const data = useMemo(() => rows(report, locale), [report, locale]);
  // Largest at the top: BarChart plots array order bottom-to-top for a
  // horizontal orientation, so the sort is reversed from the table's own
  // (largest-first) order.
  const sorted = useMemo(() => [...data].sort((a, b) => a.value - b.value), [data]);

  return (
    <ChartShell
      titleKey="chart.expenses.title"
      locale={locale}
      hasData={data.length > 0}
      table={<CategoryTable rows={data.map(({ name, text }) => ({ label: name, text }))} locale={locale} />}
      chart={
        <BarChart
          data={sorted}
          xDataKey="name"
          orientation="horizontal"
          aspectRatio="2.4 / 1"
          // Wide enough for the longest category label ("Software and
          // subscriptions") at BarYAxis's patched 190px cap -- see the
          // comment there.
          margin={{ left: 200, right: 16, top: 8, bottom: 8 }}
          animationDuration={chartsAnimate() ? 600 : 0}
        >
          <Grid horizontal={false} vertical />
          <BarYAxis showAllLabels />
          <Bar dataKey="value" fill="var(--chart-1)" />
          <ChartTooltip
            rows={(point) => [
              {
                color: "var(--chart-1)",
                label: String(point["name"]),
                value: sorted.find((r) => r.name === point["name"])?.text ?? "",
              },
            ]}
          />
        </BarChart>
      }
    />
  );
}

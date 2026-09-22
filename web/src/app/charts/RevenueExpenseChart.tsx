/**
 * Revenue against expenses — two lines, one axis. `WORKFLOW.md` §5.3, task
 * 8.11. Built on the vendored `@bklit/line-chart`.
 *
 * Two series, so `DESIGN.md` §13.5 requires a legend — built here rather than
 * with a bklit primitive, since none ships one. Slots 1 and 2 in the fixed
 * categorical order (§13.2), never chosen for this chart specifically.
 *
 * No `<XAxis>`: bklit's ships one, but its tick labels go through
 * `shortDateFmt`, a hard-coded `Intl.DateTimeFormat("en-US", ...)` with no
 * locale parameter — DESIGN.md §4 requires `ru` to read correctly, and this
 * is a monthly P&L, not a day-precision series, so "Jan 15" would be wrong
 * twice over. A locale-correct period row is built here instead; the exact
 * period is always available on hover (the tooltip is ours) and in the table.
 */
import { useMemo } from "react";
import { LineChart } from "@/components/charts/line-chart";
import { Line } from "@/components/charts/line";
import { Grid } from "@/components/charts/grid";
import { ChartTooltip } from "@/components/charts/tooltip";

import { exponentOf, formatMinorUnits } from "../../money";
import { t } from "../../i18n";
import type { Locale } from "../../i18n";
import { formatPeriodShort } from "../../ui/period";
import type { Report } from "../../data/report";
import { ChartLegend } from "./ChartLegend";
import { ChartShell } from "./ChartShell";
import { PeriodTable } from "./ChartTable";
import { chartsAnimate } from "./support";

const MARGIN = { top: 24, right: 16, bottom: 8, left: 16 };

export function RevenueExpenseChart({
  report,
  locale,
}: Readonly<{ report: Report; locale: Locale }>) {
  const exp = exponentOf(report.currencyCode);
  const revenueLine = report.lines.find((l) => l.categoryId === "01");

  const rows = useMemo(
    () =>
      report.periods.map((period, i) => {
        const revenueMinor = revenueLine?.values[i]?.minorUnits ?? "0";
        // As a positive magnitude -- this chart compares two magnitudes, and
        // expensesByPeriod is signed the way every cost in this product is.
        const expenseMinor = report.expensesByPeriod[i]!.minorUnits.replace(/^-/, "");
        return {
          period,
          revenue: exp === undefined ? 0 : Number(BigInt(revenueMinor)) / 10 ** exp,
          expenses: exp === undefined ? 0 : Number(BigInt(expenseMinor)) / 10 ** exp,
          revenueText: exp === undefined ? "" : formatMinorUnits(revenueMinor, exp, locale),
          expensesText: exp === undefined ? "" : formatMinorUnits(expenseMinor, exp, locale),
        };
      }),
    [report, revenueLine, exp, locale],
  );

  const hasData = rows.some((r) => r.revenue !== 0 || r.expenses !== 0);
  const revenueLabel = t("chart.revenueExpense.revenue", locale);
  const expensesLabel = t("chart.revenueExpense.expenses", locale);

  return (
    <ChartShell
      titleKey="chart.revenueExpense.title"
      noteKey="chart.revenueExpense.note"
      locale={locale}
      hasData={hasData}
      table={
        <PeriodTable
          periods={report.periods}
          series={[
            { label: revenueLabel, values: rows.map((r) => r.revenueText) },
            { label: expensesLabel, values: rows.map((r) => r.expensesText) },
          ]}
          locale={locale}
        />
      }
      chart={
        <div>
          {/* The legend §13.5 requires for two or more series. */}
          <ChartLegend
            items={[
              { color: "var(--chart-1)", label: revenueLabel },
              { color: "var(--chart-2)", label: expensesLabel },
            ]}
          />

          <LineChart data={rows} xDataKey="period" margin={MARGIN} aspectRatio="2.6 / 1">
            <Grid horizontal vertical={false} />
            <Line
              dataKey="revenue"
              stroke="var(--chart-1)"
              animate={chartsAnimate()}
              showMarkers
            />
            <Line
              dataKey="expenses"
              stroke="var(--chart-2)"
              animate={chartsAnimate()}
              showMarkers
            />
            <ChartTooltip
              rows={(point) => {
                const row = rows.find((r) => r.period === point["period"]);
                return [
                  { color: "var(--chart-1)", label: revenueLabel, value: row?.revenueText ?? "" },
                  { color: "var(--chart-2)", label: expensesLabel, value: row?.expensesText ?? "" },
                ];
              }}
            />
          </LineChart>

          {/* Locale-correct period labels -- see file header. Padding matches
              the chart's own left/right margin so labels sit roughly under
              their point; exact identification is the tooltip's job. */}
          <div
            className="mt-1 flex justify-between text-2xs text-text-subtle"
            style={{ paddingLeft: MARGIN.left, paddingRight: MARGIN.right }}
          >
            {rows.map((r) => (
              <span key={r.period}>{formatPeriodShort(r.period, locale)}</span>
            ))}
          </div>
        </div>
      }
    />
  );
}

/**
 * Money in and money out, month by month — one diverging column per period,
 * inflow above the zero line and outflow below it. Built on the vendored
 * `@bklit/bar-chart`, the same primitive `NetResultChart` uses.
 *
 * Why this exists next to `RevenueExpenseChart`, which looks like the same
 * chart: it is not the same figures, and the gap between them is a thing an
 * owner has to be able to see. That chart draws the P&L — `NET SALES` against
 * the cost lines between it and CM. This one draws the bank: `data/report.ts`'s
 * `cash`, from the reconciliation the backend already computes per period.
 * CAPEX, everything classified out of the P&L, and (on an accrual basis) the
 * difference between when a cost was incurred and when it was paid all fall in
 * the gap. A business can be profitable on the first chart and out of money on
 * the second in the same month, and that month is the one worth seeing.
 *
 * Transfers are not here. The wire counts an organisation moving its own money
 * between its own accounts apart from in and out, because counting it would
 * draw a business trading with itself; the reconciliation strip below the tabs
 * is where they are accounted for.
 *
 * Two series, so §13.5 requires a legend. Slots 1 and 2, the same two
 * `RevenueExpenseChart` uses and in the same roles — money in wears the hue
 * revenue wears, money out the hue expenses wear — so a reader moving between
 * the two cards is not re-learning the palette. The diverging palette would
 * have been the other candidate (§13.3, the shape is above/below a baseline)
 * and is wrong here: this chart has two named series, not one measure whose
 * sign changes, and `NetResultChart` immediately below is that chart. Reusing
 * its two hues would make "in" and "a profitable month" the same colour.
 *
 * `stacked` is what puts one column per month rather than two: each row carries
 * a positive `moneyIn` and a negative `moneyOut`, and a stack through zero
 * draws exactly one column spanning both.
 */
import { useMemo } from "react";
import { BarChart } from "@/components/charts/bar-chart";
import { Bar } from "@/components/charts/bar";
import { Grid } from "@/components/charts/grid";
import { BarXAxis } from "@/components/charts/bar-x-axis";
import { ChartTooltip } from "@/components/charts/tooltip";

import { NO_DATA, exponentOf, formatMinorUnits } from "../../money";
import { t } from "../../i18n";
import type { Locale } from "../../i18n";
import { formatPeriodShort } from "../../ui/period";
import type { Report } from "../../data/report";
import { ChartLegend } from "./ChartLegend";
import { ChartShell } from "./ChartShell";
import { PeriodTable } from "./ChartTable";
import { chartsAnimate } from "./support";

export interface CashRow {
  /** Already formatted for the axis — this is a categorical bar chart, so the
   *  x value is the label, the way `NetResultChart` does it. */
  period: string;
  moneyIn: number;
  moneyOut: number;
  inText: string;
  outText: string;
  netText: string;
  /** `BarChart` takes `Record<string, unknown>[]`; an interface without an
   *  index signature is not assignable to one, which is a TypeScript rule
   *  about interfaces rather than anything about this data. */
  [key: string]: string | number;
}

/**
 * The rows the chart draws and the table prints, as a pure function of the
 * report — so the sign rule below is checkable without a DOM.
 *
 * `cash` is already signed for display (in positive, out negative) in the data
 * layer. Nothing here re-signs it: an `Math.abs()` on the way to a stacked bar
 * is how an outflow ends up drawn above the line.
 */
export function rows(report: Report, locale: Locale): CashRow[] {
  const exp = exponentOf(report.currencyCode);
  const major = (minorUnits: string) =>
    exp === undefined ? 0 : Number(BigInt(minorUnits)) / 10 ** exp;
  const text = (minorUnits: string) =>
    exp === undefined ? NO_DATA : formatMinorUnits(minorUnits, exp, locale);

  return report.cash.map((c) => ({
    period: formatPeriodShort(c.period, locale),
    moneyIn: major(c.moneyIn.minorUnits),
    moneyOut: major(c.moneyOut.minorUnits),
    inText: text(c.moneyIn.minorUnits),
    outText: text(c.moneyOut.minorUnits),
    netText: text(c.net.minorUnits),
  }));
}

export function CashFlowChart({
  report,
  locale,
}: Readonly<{ report: Report; locale: Locale }>) {
  const data = useMemo(() => rows(report, locale), [report, locale]);

  const inLabel = t("recon.in", locale);
  const outLabel = t("recon.out", locale);
  const netLabel = t("chart.cashflow.net", locale);

  const hasData = data.some((r) => r.moneyIn !== 0 || r.moneyOut !== 0);

  return (
    <ChartShell
      titleKey="chart.cashflow.title"
      noteKey="chart.cashflow.note"
      locale={locale}
      hasData={hasData}
      table={
        <PeriodTable
          periods={report.periods}
          series={[
            { label: inLabel, values: data.map((r) => r.inText) },
            { label: outLabel, values: data.map((r) => r.outText) },
            // The third row is derived from the two above it and is why the
            // table is worth reading next to the chart: a column whose two
            // bars look similar is a month that broke even, and only the
            // figure says which way.
            { label: netLabel, values: data.map((r) => r.netText) },
          ]}
          locale={locale}
        />
      }
      chart={
        <div>
          {/* §13.5's legend, the same component `RevenueExpenseChart` draws
              -- and the same two slots, so "in" wears revenue's hue. */}
          <ChartLegend
            items={[
              { color: "var(--chart-1)", label: inLabel },
              { color: "var(--chart-2)", label: outLabel },
            ]}
          />

          <BarChart
            data={data}
            xDataKey="period"
            stacked
            aspectRatio="2.6 / 1"
            animationDuration={chartsAnimate() ? 500 : 0}
          >
            <Grid horizontal vertical={false} />
            <BarXAxis showAllLabels />
            <Bar dataKey="moneyIn" fill="var(--chart-1)" />
            <Bar dataKey="moneyOut" fill="var(--chart-2)" />
            <ChartTooltip
              rows={(point) => {
                const row = data.find((r) => r.period === point["period"]);
                return [
                  { color: "var(--chart-1)", label: inLabel, value: row?.inText ?? "" },
                  { color: "var(--chart-2)", label: outLabel, value: row?.outText ?? "" },
                  // Transparent rather than a hue: the net is not a third
                  // series, it is the two above it subtracted. A swatch here
                  // would claim a third bar exists somewhere on the chart.
                  { color: "transparent", label: netLabel, value: row?.netText ?? "" },
                ];
              }}
            />
          </BarChart>
        </div>
      }
    />
  );
}

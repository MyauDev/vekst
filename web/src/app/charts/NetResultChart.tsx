/**
 * Net result by month — one column per period. `WORKFLOW.md` §5.3, task 8.10.
 * Built on the vendored `@bklit/bar-chart`.
 *
 * Blue profit, orange loss, per `DESIGN.md` §13.3 — never red: a loss month is
 * not an error, and red is what marks a rejected import (§6).
 *
 * **A loss is drawn as its size, not below a baseline.** This was specified as
 * a diverging column centred on zero and never was one: bklit's `BarChart`
 * builds its value scale as `domain: [0, maxValue * 1.1]` (`bar-chart.tsx`)
 * and its bars as `barHeight = innerHeight - scale(value)`, so a negative
 * value lands off the axis and renders a rectangle of negative height —
 * nothing. Every loss month was invisible, and the card said so nowhere. It
 * went unnoticed because the sample data has no loss months; the first real
 * statement would have had one eventually and it would have been read as a
 * month that did not happen.
 *
 * So `profit` and `loss` are both magnitudes and exactly one is non-zero in
 * any month, which is also why `stacked` is still correct: a stack of a number
 * and a zero is that number, at the full width of the band. What carries the
 * sign is the hue, the legend and the figure in the tooltip and table — not
 * the direction, which the library cannot draw. §13.5 forbids identity by
 * colour alone, and this chart now has two named series, so it has the legend
 * that rule asks for.
 *
 * Patching the vendored scale to admit negatives is the other fix and is not
 * this one: `web/src/components/**` is copied in by the shadcn registry CLI,
 * nothing in the suite renders it (jsdom has no `ResizeObserver`), and a
 * charting change nothing can verify is how the bug above shipped.
 */
import { useMemo } from "react";
import { BarChart } from "@/components/charts/bar-chart";
import { Bar } from "@/components/charts/bar";
import { Grid } from "@/components/charts/grid";
import { BarXAxis } from "@/components/charts/bar-x-axis";
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

export interface NetRow {
  period: string;
  /** Magnitudes, both of them. Exactly one is non-zero in any month. */
  profit: number;
  loss: number;
  /** The signed figure, which is what the tooltip and the table print. */
  text: string;
  [key: string]: string | number;
}

export function netResultRows(report: Report, locale: Locale): NetRow[] {
  const exp = exponentOf(report.currencyCode);
  return report.periods.map((period, i) => {
    const m = report.netByPeriod[i]!;
    const major = exp === undefined ? 0 : Number(BigInt(m.minorUnits)) / 10 ** exp;
    return {
      period: formatPeriodShort(period, locale),
      profit: major > 0 ? major : 0,
      loss: major < 0 ? -major : 0,
      text: exp === undefined ? "" : formatMinorUnits(m.minorUnits, exp, locale),
    };
  });
}

export function NetResultChart({ report, locale }: Readonly<{ report: Report; locale: Locale }>) {
  const rows = useMemo(() => netResultRows(report, locale), [report, locale]);

  const profitLabel = t("chart.netresult.profit", locale);
  const lossLabel = t("chart.netresult.loss", locale);

  const hasData = rows.some((r) => r.profit !== 0 || r.loss !== 0);

  return (
    <ChartShell
      titleKey="chart.netresult.title"
      noteKey="chart.netresult.note"
      locale={locale}
      hasData={hasData}
      table={
        <PeriodTable
          periods={report.periods}
          series={[{ label: t("chart.netresult.series", locale), values: rows.map((r) => r.text) }]}
          locale={locale}
        />
      }
      chart={
        <div>
          <ChartLegend
            items={[
              { color: "var(--chart-diverge-positive)", label: profitLabel },
              { color: "var(--chart-diverge-negative)", label: lossLabel },
            ]}
          />
          {/* `stacked` keeps one full-width column per month: exactly one of
              the two series is non-zero, so the stack is that series. */}
          <BarChart
            data={rows}
            xDataKey="period"
            stacked
            aspectRatio="2.6 / 1"
            animationDuration={chartsAnimate() ? 500 : 0}
          >
            <Grid horizontal vertical={false} />
            <BarXAxis showAllLabels />
            <Bar dataKey="profit" fill="var(--chart-diverge-positive)" />
            <Bar dataKey="loss" fill="var(--chart-diverge-negative)" />
            <ChartTooltip
              rows={(point) => {
                const row = rows.find((r) => r.period === point["period"]);
                const lost = (row?.loss ?? 0) !== 0;
                return [
                  {
                    color: lost
                      ? "var(--chart-diverge-negative)"
                      : "var(--chart-diverge-positive)",
                    label: lost ? lossLabel : profitLabel,
                    // The signed figure: the bar is a size, so the sign has
                    // to be read somewhere.
                    value: row?.text ?? "",
                  },
                ];
              }}
            />
          </BarChart>
        </div>
      }
    />
  );
}

/**
 * Net result by month — diverging column, centred on zero. `WORKFLOW.md`
 * §5.3, task 8.10. Built on the vendored `@bklit/bar-chart`.
 *
 * Blue positive, orange negative, per `DESIGN.md` §13.3 — never red: a loss
 * month is not an error, and red is what marks a rejected import (§6).
 *
 * `<Bar>` takes one `fill` for its whole series, not a per-datum colour, so
 * the divergence is two stacked series rather than one: each row carries
 * `positive` and `negative` fields where exactly one is non-zero, and
 * `stacked` collapses them back into a single column per month.
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
import { ChartShell } from "./ChartShell";
import { PeriodTable } from "./ChartTable";
import { chartsAnimate } from "./support";

export function NetResultChart({ report, locale }: Readonly<{ report: Report; locale: Locale }>) {
  const exp = exponentOf(report.currencyCode);

  const rows = useMemo(
    () =>
      report.periods.map((period, i) => {
        const m = report.netByPeriod[i]!;
        const major = exp === undefined ? 0 : Number(BigInt(m.minorUnits)) / 10 ** exp;
        return {
          period: formatPeriodShort(period, locale),
          positive: major > 0 ? major : 0,
          negative: major < 0 ? major : 0,
          text: exp === undefined ? "" : formatMinorUnits(m.minorUnits, exp, locale),
        };
      }),
    [report, exp, locale],
  );

  const hasData = rows.some((r) => r.positive !== 0 || r.negative !== 0);

  return (
    <ChartShell
      titleKey="chart.netresult.title"
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
        <BarChart
          data={rows}
          xDataKey="period"
          stacked
          aspectRatio="2.6 / 1"
          animationDuration={chartsAnimate() ? 500 : 0}
        >
          <Grid horizontal vertical={false} />
          <BarXAxis showAllLabels />
          <Bar dataKey="positive" fill="var(--chart-diverge-positive)" />
          <Bar dataKey="negative" fill="var(--chart-diverge-negative)" />
          <ChartTooltip
            rows={(point) => {
              const row = rows.find((r) => r.period === point["period"]);
              const negative = (point["negative"] as number) !== 0;
              return [
                {
                  color: negative ? "var(--chart-diverge-negative)" : "var(--chart-diverge-positive)",
                  label: t("chart.netresult.series", locale),
                  value: row?.text ?? "",
                },
              ];
            }}
          />
        </BarChart>
      }
    />
  );
}

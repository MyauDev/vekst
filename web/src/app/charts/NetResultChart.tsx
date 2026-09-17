/**
 * Net result by month — diverging column, centred on zero. `WORKFLOW.md`
 * §5.3, task 8.10.
 *
 * Blue positive, orange negative, per `DESIGN.md` §13.3 — never red: a loss
 * month is not an error, and red is what marks a rejected import (§6).
 */
import { useMemo } from "react";
import { BarChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import * as echarts from "echarts/core";

import { exponentOf, formatMinorUnits } from "../../money";
import { t } from "../../i18n";
import type { Locale } from "../../i18n";
import { useTheme } from "../../ui/preferences";
import { formatPeriodShort } from "../../ui/period";
import type { Report } from "../../data/report";
import { ChartShell } from "./ChartShell";
import { PeriodTable } from "./ChartTable";
import { axisNumberFormatter, chartToken, reducedMotion } from "./support";

echarts.use([BarChart, GridComponent, TooltipComponent, CanvasRenderer]);

export function NetResultChart({ report, locale }: Readonly<{ report: Report; locale: Locale }>) {
  const [theme] = useTheme();
  const exp = exponentOf(report.currencyCode);

  const majorValues = useMemo(
    () =>
      report.netByPeriod.map((m) =>
        exp === undefined ? 0 : Number(BigInt(m.minorUnits)) / 10 ** exp,
      ),
    [report, exp],
  );

  const texts = useMemo(
    () =>
      report.netByPeriod.map((m) =>
        exp === undefined ? "" : formatMinorUnits(m.minorUnits, exp, locale),
      ),
    [report, exp, locale],
  );

  const option = useMemo(() => {
    if (majorValues.every((v) => v === 0)) return null;
    const pos = chartToken("--vk-diverge-pos-3");
    const neg = chartToken("--vk-diverge-neg-3");
    return {
      animation: !reducedMotion(),
      animationDuration: 300,
      grid: { left: 8, right: 16, top: 16, bottom: 24, containLabel: true },
      tooltip: {
        trigger: "axis",
        axisPointer: { type: "shadow" },
        formatter: (params: readonly { dataIndex: number }[]) => {
          const i = params[0]?.dataIndex ?? 0;
          return `${formatPeriodShort(report.periods[i]!, locale)}<br/>${texts[i]}`;
        },
      },
      xAxis: {
        type: "category",
        data: report.periods.map((p) => formatPeriodShort(p, locale)),
        axisLabel: { color: chartToken("--vk-text-subtle"), fontSize: 11 },
        axisLine: { lineStyle: { color: chartToken("--vk-chart-axis") } },
        axisTick: { show: false },
      },
      yAxis: {
        type: "value",
        axisLabel: {
          color: chartToken("--vk-text-subtle"),
          fontSize: 11,
          formatter: axisNumberFormatter(locale),
        },
        splitLine: { lineStyle: { color: chartToken("--vk-chart-grid") } },
      },
      series: [
        {
          type: "bar",
          data: majorValues.map((v) => ({
            value: v,
            itemStyle: { color: v < 0 ? neg : pos },
          })),
          barMaxWidth: 28,
        },
      ],
    };
  }, [majorValues, texts, report.periods, locale, theme]);

  return (
    <ChartShell
      titleKey="chart.netresult.title"
      locale={locale}
      option={option}
      table={
        <PeriodTable
          periods={report.periods}
          series={[{ label: t("chart.netresult.series", locale), values: texts }]}
          locale={locale}
        />
      }
    />
  );
}

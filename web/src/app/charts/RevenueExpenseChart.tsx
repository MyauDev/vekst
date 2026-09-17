/**
 * Revenue against expenses — two lines, one axis. `WORKFLOW.md` §5.3, task
 * 8.11.
 *
 * Two series, so `DESIGN.md` §13.5 requires both a legend and direct labels
 * -- neither stands in for the other; a legend is scanned once, a direct
 * label is read at the point that matters. Slots 1 and 2 in the fixed
 * categorical order (§13.2), never chosen for this chart specifically.
 */
import { useMemo } from "react";
import { LineChart } from "echarts/charts";
import { GridComponent, LegendComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import * as echarts from "echarts/core";

import { exponentOf, formatMinorUnits, sumMinorUnits } from "../../money";
import type { Locale } from "../../i18n";
import { t } from "../../i18n";
import { useTheme } from "../../ui/preferences";
import { formatPeriodShort } from "../../ui/period";
import type { Report } from "../../data/report";
import { ChartShell } from "./ChartShell";
import { PeriodTable } from "./ChartTable";
import { axisNumberFormatter, chartToken, reducedMotion } from "./support";

echarts.use([LineChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

export function RevenueExpenseChart({
  report,
  locale,
}: Readonly<{ report: Report; locale: Locale }>) {
  const [theme] = useTheme();
  const exp = exponentOf(report.currencyCode);

  const revenue = report.sections.find((s) => s.id === "revenue")!;
  const expenseSections = report.sections.filter((s) => s.id !== "revenue");

  const revenueMinor = revenue.subtotals.map((m) => m.minorUnits);
  const expenseMinor = report.periods.map((_, i) =>
    // Cost of sales and operating expenses combined, as a positive magnitude
    // -- this chart compares two magnitudes, and expenses are stored negative.
    sumMinorUnits(expenseSections.map((s) => s.subtotals[i]!.minorUnits)).replace(/^-/, ""),
  );

  const revenueMajor = useMemo(
    () => revenueMinor.map((u) => (exp === undefined ? 0 : Number(BigInt(u)) / 10 ** exp)),
    [revenueMinor, exp],
  );
  const expenseMajor = useMemo(
    () => expenseMinor.map((u) => (exp === undefined ? 0 : Number(BigInt(u)) / 10 ** exp)),
    [expenseMinor, exp],
  );

  const revenueText = useMemo(
    () => revenueMinor.map((u) => (exp === undefined ? "" : formatMinorUnits(u, exp, locale))),
    [revenueMinor, exp, locale],
  );
  const expenseText = useMemo(
    () => expenseMinor.map((u) => (exp === undefined ? "" : formatMinorUnits(u, exp, locale))),
    [expenseMinor, exp, locale],
  );

  const option = useMemo(() => {
    if (revenueMajor.every((v) => v === 0) && expenseMajor.every((v) => v === 0)) return null;
    const revenueLabel = t("chart.revenueExpense.revenue", locale);
    const expensesLabel = t("chart.revenueExpense.expenses", locale);
    const text = chartToken("--vk-text-muted");
    return {
      animation: !reducedMotion(),
      animationDuration: 300,
      grid: { left: 8, right: 48, top: 40, bottom: 24, containLabel: true },
      legend: {
        top: 0,
        left: 0,
        icon: "circle",
        itemWidth: 8,
        itemHeight: 8,
        textStyle: { color: text, fontSize: 12 },
      },
      tooltip: {
        trigger: "axis",
        formatter: (params: readonly { dataIndex: number }[]) => {
          const i = params[0]?.dataIndex ?? 0;
          return `${formatPeriodShort(report.periods[i]!, locale)}<br/>${revenueLabel}: ${revenueText[i]}<br/>${expensesLabel}: ${expenseText[i]}`;
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
          name: revenueLabel,
          type: "line",
          data: revenueMajor,
          symbolSize: 8,
          lineStyle: { width: 2, color: chartToken("--vk-series-1") },
          itemStyle: { color: chartToken("--vk-series-1") },
          endLabel: { show: true, formatter: revenueLabel, color: chartToken("--vk-series-1"), fontSize: 11 },
        },
        {
          name: expensesLabel,
          type: "line",
          data: expenseMajor,
          symbolSize: 8,
          lineStyle: { width: 2, color: chartToken("--vk-series-2") },
          itemStyle: { color: chartToken("--vk-series-2") },
          endLabel: { show: true, formatter: expensesLabel, color: chartToken("--vk-series-2"), fontSize: 11 },
        },
      ],
    };
  }, [revenueMajor, expenseMajor, revenueText, expenseText, report.periods, locale, theme]);

  return (
    <ChartShell
      titleKey="chart.revenueExpense.title"
      locale={locale}
      option={option}
      table={
        <PeriodTable
          periods={report.periods}
          series={[
            { label: t("chart.revenueExpense.revenue", locale), values: revenueText },
            { label: t("chart.revenueExpense.expenses", locale), values: expenseText },
          ]}
          locale={locale}
        />
      }
    />
  );
}

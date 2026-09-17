/**
 * Top expense categories — horizontal bar, sorted. `WORKFLOW.md` §5.3, task
 * 8.9.
 *
 * One hue, slot 1, every bar the same step (§13.4): bar length already
 * encodes magnitude, so colouring bars by their own value spends the
 * identity channel re-encoding what length already shows.
 */
import { useMemo } from "react";
import { BarChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import * as echarts from "echarts/core";

import { NO_DATA, exponentOf, formatMinorUnits } from "../../money";
import type { Locale } from "../../i18n";
import { useTheme } from "../../ui/preferences";
import type { Report } from "../../data/report";
import { ChartShell } from "./ChartShell";
import { CategoryTable } from "./ChartTable";
import { axisNumberFormatter, chartToken, foldTopN, reducedMotion } from "./support";
import type { Sliced } from "./support";

echarts.use([BarChart, GridComponent, TooltipComponent, CanvasRenderer]);

const MAX_SLOTS = 8;

interface Row {
  categoryId: string;
}

function rows(report: Report, locale: Locale) {
  const exp = exponentOf(report.currencyCode);
  const sliced: Sliced<Row>[] = report.sections
    .filter((s) => s.id !== "revenue")
    .flatMap((s) => s.lines)
    .filter((line) => line.total !== null)
    .map((line) => {
      const minor = BigInt(line.total!.minorUnits);
      const magnitude = minor < 0n ? -minor : minor;
      return {
        label: line.label,
        value: exp === undefined ? 0 : Number(magnitude) / 10 ** exp,
        item: { categoryId: line.categoryId },
      };
    });

  const { kept, other } = foldTopN(sliced, MAX_SLOTS, "—");
  const named = other
    ? [...kept, { label: other.label, value: other.value, item: { categoryId: "other" } }]
    : kept;

  return named.map((r) => ({
    label: r.label,
    value: r.value,
    text: exp === undefined ? NO_DATA : formatMinorUnits(String(Math.round(r.value * 10 ** exp)), exp, locale),
  }));
}

export function ExpenseCategoriesChart({
  report,
  locale,
}: Readonly<{ report: Report; locale: Locale }>) {
  const [theme] = useTheme();
  const data = useMemo(() => rows(report, locale), [report, locale]);

  const option = useMemo(() => {
    if (data.length === 0) return null;
    const sorted = [...data].sort((a, b) => a.value - b.value); // ascending: largest bar on top
    return {
      animation: !reducedMotion(),
      animationDuration: 300,
      grid: { left: 8, right: 16, top: 8, bottom: 8, containLabel: true },
      tooltip: {
        trigger: "axis",
        axisPointer: { type: "shadow" },
        // Repeats the figure, never reveals it -- §9 forbids hover-only information.
        formatter: (params: readonly { dataIndex: number }[]) => {
          const row = sorted[params[0]?.dataIndex ?? 0];
          return row ? `${row.label}<br/>${row.text}` : "";
        },
      },
      xAxis: {
        type: "value",
        axisLabel: { color: chartToken("--vk-text-subtle"), fontSize: 11, formatter: axisNumberFormatter(locale) },
        splitLine: { lineStyle: { color: chartToken("--vk-chart-grid") } },
      },
      yAxis: {
        type: "category",
        data: sorted.map((r) => r.label),
        axisLabel: { color: chartToken("--vk-text-muted"), fontSize: 12 },
        axisLine: { lineStyle: { color: chartToken("--vk-chart-axis") } },
        axisTick: { show: false },
      },
      series: [
        {
          type: "bar",
          data: sorted.map((r) => r.value),
          itemStyle: { color: chartToken("--vk-series-1") },
          barMaxWidth: 18,
        },
      ],
    };
    // theme is a dependency because every colour above is re-read from the
    // token layer, which the theme attribute changes.
  }, [data, locale, theme]);

  return (
    <ChartShell
      titleKey="chart.expenses.title"
      locale={locale}
      option={option}
      table={<CategoryTable rows={data} locale={locale} />}
    />
  );
}

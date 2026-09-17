/**
 * Category trend — small multiples, one line each. `WORKFLOW.md` §5.3, task
 * 8.12.
 *
 * One hue plus `--color-chart-deemph` for context (§13.4): small multiples
 * are an all-pairs comparison, which caps distinguishable series at three by
 * colour alone, so every facet uses the same hue and identity comes from the
 * facet's own title instead. The de-emphasis line is each category's own
 * average across the range — the context a reader actually wants ("is this
 * month high or low for this line"), not a second series.
 *
 * One y-axis per facet (§8.7 forbids two inside *one* chart; a grid of
 * single-axis facets is not that).
 */
import { useMemo } from "react";
import { LineChart } from "echarts/charts";
import { GraphicComponent, GridComponent, TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import * as echarts from "echarts/core";

import { exponentOf, formatMinorUnits } from "../../money";
import type { Locale } from "../../i18n";
import { useTheme } from "../../ui/preferences";
import type { Report } from "../../data/report";
import { ChartShell } from "./ChartShell";
import { PeriodTable } from "./ChartTable";
import { chartToken, reducedMotion } from "./support";

echarts.use([LineChart, GraphicComponent, GridComponent, TooltipComponent, CanvasRenderer]);

const MAX_FACETS = 8;
const COLS = 4;

interface Facet {
  categoryId: string;
  label: string;
  values: readonly number[];
  texts: readonly string[];
}

function facets(report: Report, locale: Locale): Facet[] {
  const exp = exponentOf(report.currencyCode);
  const all = report.sections
    .flatMap((s) => s.lines)
    .filter((l) => l.total !== null)
    .map((l) => {
      const magnitude = (m: { minorUnits: string } | null) =>
        m && exp !== undefined ? Math.abs(Number(BigInt(m.minorUnits)) / 10 ** exp) : 0;
      return {
        categoryId: l.categoryId,
        label: l.label,
        totalAbs: Math.abs(exp === undefined || !l.total ? 0 : Number(BigInt(l.total.minorUnits))),
        values: l.values.map(magnitude),
        texts: l.values.map((v) => (v && exp !== undefined ? formatMinorUnits(v.minorUnits, exp, locale) : "—")),
      };
    })
    .sort((a, b) => b.totalAbs - a.totalAbs)
    .slice(0, MAX_FACETS);
  return all;
}

export function CategoryTrendChart({
  report,
  locale,
}: Readonly<{ report: Report; locale: Locale }>) {
  const [theme] = useTheme();
  const data = useMemo(() => facets(report, locale), [report, locale]);

  const option = useMemo(() => {
    if (data.length === 0) return null;
    const rows = Math.ceil(data.length / COLS);
    const gapPct = 4;
    const cellW = (100 - gapPct * (COLS - 1)) / COLS;
    const cellH = (100 - gapPct * (rows - 1)) / rows;
    const line = chartToken("--vk-series-1");
    const deemph = chartToken("--vk-chart-deemph");
    const label = chartToken("--vk-text-muted");

    const grids = data.map((_, i) => {
      const col = i % COLS;
      const row = Math.floor(i / COLS);
      return {
        left: `${col * (cellW + gapPct)}%`,
        top: `${row * (cellH + gapPct) + 6}%`,
        width: `${cellW}%`,
        height: `${cellH - 8}%`,
      };
    });

    return {
      animation: !reducedMotion(),
      animationDuration: 300,
      grid: grids,
      tooltip: { trigger: "axis" },
      // The facet's title, as canvas-percentage text rather than a per-facet
      // chart title component (that component cannot be gridIndex-scoped) and
      // rather than a markPoint at a data coordinate (a category's own values
      // rarely include zero, so a point pinned to y=0 lands outside the
      // auto-scaled range and never actually appears -- this was tried first).
      graphic: grids.map((g, i) => ({
        type: "text",
        left: g.left,
        top: `${parseFloat(String(g.top)) - 4}%`,
        style: {
          text: data[i]!.label,
          fill: label,
          fontSize: 11,
          fontWeight: 600,
        },
      })),
      xAxis: data.map((_, i) => ({
        gridIndex: i,
        type: "category",
        data: report.periods,
        show: false,
      })),
      yAxis: data.map((_, i) => ({ gridIndex: i, type: "value", show: false })),
      series: data.flatMap((f, i) => {
        const avg = f.values.reduce((s, v) => s + v, 0) / (f.values.length || 1);
        return [
          {
            name: f.label,
            type: "line",
            xAxisIndex: i,
            yAxisIndex: i,
            data: f.values,
            symbol: "none",
            lineStyle: { width: 2, color: line },
          },
          // The facet's own average, in the de-emphasis grey -- the context a
          // reader wants ("high or low against its own normal"), not a second
          // identity. A flat data series rather than a `markLine`: it shares
          // the main line's coordinate system exactly, with no separate
          // API to get subtly wrong.
          {
            type: "line",
            xAxisIndex: i,
            yAxisIndex: i,
            data: f.values.map(() => avg),
            symbol: "none",
            silent: true,
            lineStyle: { width: 1, color: deemph, type: "dashed" },
          },
        ];
      }),
    };
  }, [data, report.periods, theme]);

  return (
    <ChartShell
      titleKey="chart.trend.title"
      locale={locale}
      option={option}
      height="h-96"
      table={
        <PeriodTable
          periods={report.periods}
          series={data.map((f) => ({ label: f.label, values: f.texts }))}
          locale={locale}
        />
      }
    />
  );
}

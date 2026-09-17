/**
 * Money flow — Sankey: revenue sources → total in → expense categories.
 * `WORKFLOW.md` §5.3, task 8.8. "The 'where does my money go' answer, and
 * the picture customers screenshot."
 *
 * Top 8 plus Other on the expense side (§13.2, task 8.17) — a Sankey is a
 * categorical-hue chart like the bar in `ExpenseCategoriesChart`, just with
 * two hops instead of one.
 *
 * A Sankey's node height is its total flow, so the diagram only reads as
 * "nothing was dropped" if inflow equals outflow at every node. The revenue
 * side always balances by construction (every source sums to "Total in").
 * The expense side balances only when expenses do not exceed revenue for the
 * range, in which case the remainder becomes an explicit "Net result" link
 * — the same figure the headline row already shows, not a new one. Where
 * the remainder is zero or negative there is nothing honest to draw for it,
 * so the link is simply omitted rather than forced to a floor of zero.
 */
import { useMemo } from "react";
import { SankeyChart } from "echarts/charts";
import { TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import * as echarts from "echarts/core";

import { NO_DATA, exponentOf, formatMinorUnits } from "../../money";
import { t } from "../../i18n";
import type { Locale } from "../../i18n";
import { useTheme } from "../../ui/preferences";
import type { Report } from "../../data/report";
import { ChartShell } from "./ChartShell";
import { CategoryTable } from "./ChartTable";
import { chartToken, foldTopN, reducedMotion } from "./support";
import type { Sliced } from "./support";

echarts.use([SankeyChart, TooltipComponent, CanvasRenderer]);

const MAX_EXPENSE_SLOTS = 8;

export function MoneyFlowChart({ report, locale }: Readonly<{ report: Report; locale: Locale }>) {
  const [theme] = useTheme();
  const exp = exponentOf(report.currencyCode);

  const graph = useMemo(() => {
    if (exp === undefined) return null;
    const totalInLabel = t("chart.moneyflow.totalIn", locale);
    const netLabel = t("chart.netresult.series", locale);

    const revenue = report.sections.find((s) => s.id === "revenue")!;
    const revenueLines = revenue.lines.filter((l) => l.total !== null);
    if (revenueLines.length === 0) return null;

    const expenseLines: Sliced<null>[] = report.sections
      .filter((s) => s.id !== "revenue")
      .flatMap((s) => s.lines)
      .filter((l) => l.total !== null)
      .map((l) => ({
        label: l.label,
        value: Math.abs(Number(BigInt(l.total!.minorUnits)) / 10 ** exp),
        item: null,
      }));

    const { kept, other } = foldTopN(expenseLines, MAX_EXPENSE_SLOTS, t("chart.moneyflow.other", locale));
    const expenseNodes = other ? [...kept, { label: other.label, value: other.value }] : kept;

    const totalInMinor = revenue.total.minorUnits;
    const totalInMajor = Number(BigInt(totalInMinor)) / 10 ** exp;
    const totalOutMajor = expenseNodes.reduce((s, e) => s + e.value, 0);
    const remainder = totalInMajor - totalOutMajor;

    const nodes = [
      ...revenueLines.map((l) => ({ name: l.label })),
      { name: totalInLabel },
      ...expenseNodes.map((e) => ({ name: e.label })),
      ...(remainder > 0 ? [{ name: netLabel }] : []),
    ];

    const links = [
      ...revenueLines.map((l) => ({
        source: l.label,
        target: totalInLabel,
        value: Number(BigInt(l.total!.minorUnits)) / 10 ** exp,
      })),
      ...expenseNodes.map((e) => ({ source: totalInLabel, target: e.label, value: e.value })),
      ...(remainder > 0 ? [{ source: totalInLabel, target: netLabel, value: remainder }] : []),
    ];

    return { nodes, links };
  }, [report, exp, locale]);

  const rows = useMemo(() => {
    if (exp === undefined) return [];
    return report.sections
      .flatMap((s) => s.lines)
      .filter((l) => l.total !== null)
      .map((l) => ({ label: l.label, text: formatMinorUnits(l.total!.minorUnits, exp, locale) }));
  }, [report, exp, locale]);

  const option = useMemo(() => {
    if (!graph) return null;
    const nodeColor = chartToken("--vk-series-1");
    const linkColor = chartToken("--vk-chart-grid");
    const label = chartToken("--vk-text-muted");
    return {
      animation: !reducedMotion(),
      animationDuration: 400,
      tooltip: {
        trigger: "item",
        // Repeats the value the diagram already shows -- §9 forbids
        // hover-only information; this is not where the number is revealed.
      },
      series: [
        {
          type: "sankey",
          data: graph.nodes,
          links: graph.links,
          nodeWidth: 12,
          nodeGap: 10,
          label: { color: label, fontSize: 11 },
          lineStyle: { color: linkColor, opacity: 0.5, curveness: 0.5 },
          itemStyle: { color: nodeColor, borderWidth: 0 },
          emphasis: { focus: "adjacency" },
        },
      ],
    };
  }, [graph, theme]);

  return (
    <ChartShell
      titleKey="chart.moneyflow.title"
      locale={locale}
      option={option}
      height="h-96"
      table={rows.length ? <CategoryTable rows={rows} locale={locale} /> : <p className="text-sm text-text-muted">{NO_DATA}</p>}
    />
  );
}

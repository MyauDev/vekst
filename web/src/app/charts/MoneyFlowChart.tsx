/**
 * Money flow — Sankey: revenue sources → total in → expense categories.
 * `WORKFLOW.md` §5.3, task 8.8. "The 'where does my money go' answer, and
 * the picture customers screenshot."
 *
 * The other four charts in this module are bklit (`@bklit/*`, vendored under
 * `src/components/charts`); this one stays on ECharts by explicit direction.
 * bklit's Sankey has no collision avoidance in its label layout, and this
 * chart's own data shape breaks it: the expense side spans roughly a 40x
 * magnitude range (Materials down to Professional fees), so the small nodes
 * become slivers a few pixels tall while their text labels still need
 * 16-20px of vertical room — the labels overlap and drift from their bars
 * regardless of chart height. Not a one-line fix, so this chart is a
 * documented exception rather than a forced adoption.
 *
 * Top 8 plus Other on the expense side (§13.2, task 8.17) — a Sankey is a
 * categorical-hue chart like the bar in `ExpenseCategoriesChart`, just with
 * two hops instead of one. Each expense node gets its own hue in the fixed
 * slot order (§13.2); revenue sources, "Total in" and "Net result" are
 * structure rather than identity and stay ink.
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
import { useEffect, useMemo, useRef } from "react";
import { SankeyChart } from "echarts/charts";
import { TooltipComponent } from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import * as echarts from "echarts/core";

import { NO_DATA, exponentOf, formatMinorUnits } from "../../money";
import { t } from "../../i18n";
import type { Locale } from "../../i18n";
import { useTheme } from "../../ui/preferences";
import type { Report } from "../../data/report";
import { lineName } from "../lineName";
import { ChartShell } from "./ChartShell";
import { CategoryTable } from "./ChartTable";
import { foldTopN, reducedMotion } from "./support";
import type { Sliced } from "./support";

echarts.use([SankeyChart, TooltipComponent, CanvasRenderer]);

const MAX_EXPENSE_SLOTS = 8;

/** §13.2's fixed slot order, read as the raw tokens ECharts itself expects
 *  (a canvas has no CSS cascade, so `chartToken` resolves these once per
 *  render rather than at draw time the way a bklit `var()` would). */
const SERIES_SLOTS = [
  "--vk-series-1", "--vk-series-2", "--vk-series-3", "--vk-series-4",
  "--vk-series-5", "--vk-series-6", "--vk-series-7", "--vk-series-8",
] as const;

function chartToken(name: string): string {
  if (typeof document === "undefined") return "";
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/**
 * The Sankey's expense side, before folding into slots and before colour: a
 * pure function of the report and its exponent, so the sign rule it applies
 * is checkable without a DOM.
 *
 * `pnl.go`'s `present()` prints a cost section positive and, on purpose, a
 * section that nets to a *credit* for the range negative -- a currency gain
 * booked under Financial result is not an expense that period, however
 * large. A Sankey link cannot be negative, and flipping it back with
 * `Math.abs()` would draw that gain as an outflow the same as a real cost;
 * excluded here for the same reason `ExpenseCategoriesChart.rows` excludes
 * it, which is also what keeps the diagram's own remainder calculation from
 * being overstated and silently swallowing the "Net result" link.
 */
export function expenseLinesFor(report: Report, exp: number, locale: Locale): Sliced<null>[] {
  return report.lines
    .filter((l) => !l.computed && l.categoryId !== "01")
    .filter((l) => BigInt(l.total.minorUnits) > 0n)
    .map((l) => ({
      label: lineName(l.categoryId, l.label, locale),
      value: Number(BigInt(l.total.minorUnits)) / 10 ** exp,
      item: null,
    }));
}

export function MoneyFlowChart({ report, locale }: Readonly<{ report: Report; locale: Locale }>) {
  const [theme] = useTheme();
  const exp = exponentOf(report.currencyCode);

  const graph = useMemo(() => {
    if (exp === undefined) return null;
    const totalInLabel = t("chart.moneyflow.totalIn", locale);
    const netLabel = t("chart.netresult.series", locale);

    // The real backend has one revenue line (NET SALES, "01"), not several --
    // `core/internal/report/pnl.go`'s `Order` computes no per-source revenue
    // breakdown, so this Sankey's revenue side is a single node now rather
    // than the fixture's several.
    const revenue = report.lines.find((l) => l.categoryId === "01");
    if (!revenue) return null;

    // No revenue, no diagram. A Sankey's node height is its total flow, so
    // the picture only means "nothing was dropped" if inflow equals outflow
    // at every node -- and with a zero revenue node there is no inflow for
    // the expense side to come out of. Drawn anyway (as it was against a real
    // statement whose revenue was all misclassified) it is a hairline "Total
    // in" with a full-height expense bar hanging off it: a diagram asserting
    // that 171,262.11 flowed out of nothing. The table view below says the
    // same figures without claiming they balance.
    if (BigInt(revenue.total.minorUnits) <= 0n) return null;

    const revenueLines = [revenue];

    const expenseLines = expenseLinesFor(report, exp, locale);

    const { kept, other } = foldTopN(expenseLines, MAX_EXPENSE_SLOTS, t("chart.moneyflow.other", locale));
    const expenseNodes = other ? [...kept, { label: other.label, value: other.value }] : kept;

    const totalInMajor = Number(BigInt(revenue.total.minorUnits)) / 10 ** exp;
    const totalOutMajor = expenseNodes.reduce((s, e) => s + e.value, 0);
    const remainder = totalInMajor - totalOutMajor;

    const structural = chartToken("--vk-text");
    const nodes = [
      ...revenueLines.map((l) => ({
        name: lineName(l.categoryId, l.label, locale),
        itemStyle: { color: structural },
      })),
      { name: totalInLabel, itemStyle: { color: structural } },
      ...expenseNodes.map((e, i) => ({
        name: e.label,
        itemStyle: { color: chartToken(SERIES_SLOTS[i % SERIES_SLOTS.length]!) },
      })),
      ...(remainder > 0 ? [{ name: netLabel, itemStyle: { color: structural } }] : []),
    ];

    const links = [
      ...revenueLines.map((l) => ({
        source: lineName(l.categoryId, l.label, locale),
        target: totalInLabel,
        value: Number(BigInt(l.total.minorUnits)) / 10 ** exp,
      })),
      ...expenseNodes.map((e) => ({ source: totalInLabel, target: e.label, value: e.value })),
      ...(remainder > 0 ? [{ source: totalInLabel, target: netLabel, value: remainder }] : []),
    ];

    return { nodes, links };
    // theme is a dependency because chartToken re-reads it, and the DOM read
    // only happens inside this memo, not on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [report, exp, locale, theme]);

  const rows = useMemo(() => {
    if (exp === undefined) return [];
    return report.lines.map((l) => ({
      label: lineName(l.categoryId, l.label, locale),
      text: formatMinorUnits(l.total.minorUnits, exp, locale),
    }));
  }, [report, exp, locale]);

  const option = useMemo(() => {
    if (!graph) return null;
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
          emphasis: { focus: "adjacency" },
        },
      ],
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [graph, theme]);

  // `ChartShell` mounts and unmounts this `<div>` as the reader toggles
  // between chart and table -- a plain `useEffect` keyed on `option` would
  // never re-fire for that transition, since `option` itself does not
  // change when the div remounts, and the chart would stay blank after the
  // first toggle back. A callback ref fires on every genuine mount and
  // unmount regardless of dependency diffing, which is what this needs.
  const instance = useRef<echarts.ECharts | null>(null);
  const resizeObserver = useRef<ResizeObserver | null>(null);
  const initFrame = useRef<number | null>(null);
  const attach = (node: HTMLDivElement | null) => {
    resizeObserver.current?.disconnect();
    instance.current?.dispose();
    instance.current = null;
    if (initFrame.current !== null) cancelAnimationFrame(initFrame.current);

    if (!node || !option) return;

    // Deferred one frame: reading the node's size in the same tick it mounts
    // -- e.g. right after the reader clicks back from the table view -- can
    // catch layout mid-reflow. `echarts.init()` measures once, synchronously,
    // on call; a Sankey's own `resize()` path doesn't fully recompute that
    // measurement afterward (the same internal coordinate-system code
    // `SankeyView2._updateViewCoordSys` above), so a bad first read stays
    // bad. Waiting a frame for layout to settle is more reliable than
    // resizing our way out of it after the fact.
    initFrame.current = requestAnimationFrame(() => {
      initFrame.current = null;
      const chart = echarts.init(node);
      chart.setOption(option);
      instance.current = chart;
      attachResizeObserver(node, chart);
    });
  };

  function attachResizeObserver(node: HTMLDivElement, chart: echarts.ECharts) {
    if (typeof ResizeObserver !== "undefined") {
      resizeObserver.current = new ResizeObserver(() => {
        // Scrolling this card into view can fire a resize while the
        // container's box is mid-reflow (width/height still 0 or
        // transitional). ECharts' own Sankey coordinate-system code throws
        // on that transient state (`SankeyView2._updateViewCoordSys`, an
        // upstream issue, not this wrapper) -- skip a resize with nothing to
        // measure yet, and swallow the rare remaining race rather than let
        // an internal charting-library exception surface as an app error.
        if (node.clientWidth === 0 || node.clientHeight === 0) return;
        try {
          chart.resize();
        } catch {
          /* transient upstream layout race; the next resize corrects it */
        }
      });
      resizeObserver.current.observe(node);
    }
  }

  // Redraw in place when the option itself changes (theme, locale, data)
  // without waiting for an unmount/remount.
  useEffect(() => {
    instance.current?.setOption(option ?? {});
  }, [option]);

  return (
    <ChartShell
      titleKey="chart.moneyflow.title"
      noteKey="chart.moneyflow.note"
      locale={locale}
      hasData={option !== null}
      table={rows.length ? <CategoryTable rows={rows} locale={locale} /> : <p className="text-sm text-text-muted">{NO_DATA}</p>}
      chart={<div ref={attach} className="h-96 w-full" />}
    />
  );
}

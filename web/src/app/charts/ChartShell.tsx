/**
 * The chart shell every chart in `WORKFLOW.md` §5.3 is built from: title,
 * table-view toggle, canvas lifecycle. §8.2 exists so the rules in §8.3–8.6
 * are satisfied once here rather than five times in five chart files.
 *
 * A chart-specific legend, where §8.4 requires one, lives inside that chart's
 * own ECharts `option` (`legend: {...}`) rather than in this shell — it is
 * chart config, not shell chrome, and only `RevenueExpenseChart` needs it.
 */
import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import * as echarts from "echarts/core";

import { t } from "../../i18n";
import type { Locale, MessageKey } from "../../i18n";
import { canDrawCanvas } from "./support";

interface ChartShellProps {
  titleKey: MessageKey;
  locale: Locale;
  /** `null` when there is nothing to draw (e.g. every value is zero). */
  option: Record<string, unknown> | null;
  table: ReactNode;
  /** Tailwind height class for the canvas host. */
  height?: string;
}

export function ChartShell({
  titleKey,
  locale,
  option,
  table,
  height = "h-80",
}: Readonly<ChartShellProps>) {
  const drawable = canDrawCanvas() && option !== null;
  const [asTable, setAsTable] = useState(!drawable);
  const host = useRef<HTMLDivElement>(null);

  // Init when there is something to draw and the reader wants the chart,
  // tear down and redraw whenever `option`'s identity changes -- the
  // caller's own `useMemo` (over the report, locale and theme) is what
  // decides when that identity actually changes.
  useEffect(() => {
    const node = host.current;
    if (!node || !option || asTable) return;

    const chart = echarts.init(node);
    // §8.15: a chart may draw itself in; a figure inside it may not count up.
    chart.setOption(option);

    const ro =
      typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => chart.resize());
    ro?.observe(node);

    return () => {
      ro?.disconnect();
      chart.dispose();
    };
  }, [option, asTable]);

  return (
    <section className="rounded-panel border border-border bg-surface-raised p-6">
      <div className="flex flex-wrap items-baseline gap-4">
        <h2 className="text-2xs font-semibold uppercase tracking-widest text-text-subtle">
          {t(titleKey, locale)}
        </h2>
        {drawable ? (
          <button
            type="button"
            onClick={() => setAsTable((v) => !v)}
            className="ml-auto border-b border-text pb-px text-2xs font-semibold uppercase tracking-widest text-text hover:border-text-muted hover:text-text-muted active:text-text-subtle"
          >
            {t(asTable ? "chart.view.chart" : "chart.view.table", locale)}
          </button>
        ) : null}
      </div>

      <div className="mt-4">
        {asTable || !drawable ? table : <div ref={host} className={`w-full ${height}`} />}
      </div>
    </section>
  );
}

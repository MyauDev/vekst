/**
 * The chart shell every chart in `WORKFLOW.md` §5.3 is built from: title and
 * table-view toggle. §8.2 exists so the rules in §8.3–8.6 are satisfied once
 * here rather than five times in five chart files.
 *
 * Rebuilt for bklit: the ECharts version owned a canvas's imperative
 * init/resize/dispose lifecycle here. bklit's charts are ordinary React
 * components with no instance to manage, so this shell is back to pure
 * presentation -- title, toggle, a slot for whichever child renders.
 *
 * A chart-specific legend, where §8.4 requires one, is chart JSX (bklit has
 * no shell-level legend concept) rather than shell chrome -- only
 * `RevenueExpenseChart` needs one.
 */
import { useState } from "react";
import type { ReactNode } from "react";

import { t } from "../../i18n";
import type { Locale, MessageKey } from "../../i18n";
import { canRenderChart } from "./support";

interface ChartShellProps {
  titleKey: MessageKey;
  locale: Locale;
  /** `false` when there is nothing to draw (e.g. every value is zero). */
  hasData: boolean;
  chart: ReactNode;
  table: ReactNode;
}

export function ChartShell({
  titleKey,
  locale,
  hasData,
  chart,
  table,
}: Readonly<ChartShellProps>) {
  const drawable = canRenderChart() && hasData;
  const [asTable, setAsTable] = useState(!drawable);

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

      <div className="mt-4">{asTable || !drawable ? table : chart}</div>
    </section>
  );
}

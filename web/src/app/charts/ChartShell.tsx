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
  /**
   * One line saying what this card is showing, under the title. Added
   * 2026-09-22: five cards whose headings were two words each, stacked on one
   * tab, left the reader to work out from the axes which figures each one was
   * drawn from -- and two of them ("Revenue against expenses", "Money in and
   * out") are genuinely different sets of numbers that look alike. The
   * distinction has to be on the card, not in whoever's head built it.
   *
   * Optional only so a card with genuinely nothing to add is not made to
   * invent a sentence.
   */
  noteKey?: MessageKey;
  /** `false` when there is nothing to draw (e.g. every value is zero). */
  hasData: boolean;
  chart: ReactNode;
  table: ReactNode;
}

export function ChartShell({
  titleKey,
  noteKey,
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
        {/* A card's own name, at the size §14.2 raised the ceiling to rather
            than the 2xs uppercase micro-heading it used to be. An uppercase
            tracked-out label is a column header; this is the title of the
            thing in the card, and it is the first thing read on the tab. */}
        <h2 className="text-sm font-semibold text-text">{t(titleKey, locale)}</h2>
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

      {noteKey ? (
        <p className="mt-1 max-w-prose text-xs text-text-muted">{t(noteKey, locale)}</p>
      ) : null}

      <div className="mt-4">{asTable || !drawable ? table : chart}</div>
    </section>
  );
}

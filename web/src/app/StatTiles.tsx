/**
 * The headline row: revenue, expenses, net result, amount awaiting review.
 *
 * `WORKFLOW.md` §5.3 lists these among the charts and gives their colour job as
 * **none** -- "a single number is not a one-bar chart". So they take no palette
 * and no library, which makes them the one part of that section that costs
 * nothing to bring forward.
 *
 * Every figure is computed from the rows the table renders, so a tile cannot
 * disagree with the column beneath it.
 *
 * Cards, since the §14 amendment of 2026-09-16. These were hairline-separated
 * columns in a single row; the label-above-figure pairing is unchanged, and it
 * is still the type that carries the hierarchy rather than the surface.
 */
import { NO_DATA, exponentOf, formatMinorUnits } from "../money";
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import type { Money } from "../data/types";
import type { Report } from "../data/report";

function Tile({
  labelKey,
  value,
  locale,
  muted,
}: Readonly<{
  labelKey: MessageKey;
  value: Money;
  locale: Locale;
  muted?: boolean;
}>) {
  const exp = exponentOf(value.currencyCode);
  const text = exp === undefined ? NO_DATA : formatMinorUnits(value.minorUnits, exp, locale);
  return (
    <div className="flex flex-col gap-2 rounded-panel border border-border bg-surface-raised p-6">
      <span className="text-2xs uppercase tracking-widest text-text-subtle">
        {t(labelKey, locale)}
      </span>
      {/*
        Proportional figures, not tabular. Tabular is for columns that must
        align vertically; a single large number has no column to align with, and
        tabular spacing makes it look mechanical.
      */}
      <span className={`text-2xl font-semibold ${muted ? "text-text-muted" : "text-figure"}`}>
        {text}
      </span>
    </div>
  );
}

export function StatTiles({ report, locale }: Readonly<{ report: Report; locale: Locale }>) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Tile labelKey="stat.revenue" value={report.revenueTotal} locale={locale} />
      <Tile labelKey="stat.expenses" value={report.expensesTotal} locale={locale} />
      <Tile labelKey="stat.net" value={report.netTotal} locale={locale} />
      {/* An unreviewed row is a wrong number in this very report, which is why
          it sits beside the result rather than only on the rail. */}
      <Tile labelKey="stat.unreviewed" value={report.unreviewedAmount} locale={locale} muted />
    </div>
  );
}

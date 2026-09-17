/**
 * §8.3: every chart has a table view, and the table is the same figures the
 * chart draws — not a fallback, a second reading of one set of numbers. Two
 * shapes cover all five charts in `WORKFLOW.md` §5.3: one row per category
 * (money flow, top expenses), or one row per series with one column per
 * period (net result, revenue vs. expenses, category trend).
 */
import { t } from "../../i18n";
import type { Locale } from "../../i18n";
import { formatPeriodShort } from "../../ui/period";
import type { Period } from "../../data/types";

export function CategoryTable({
  rows,
  locale,
}: Readonly<{ rows: readonly { label: string; text: string }[]; locale: Locale }>) {
  return (
    <table className="w-full border-collapse text-sm">
      <thead>
        <tr className="h-10">
          <th
            scope="col"
            className="border-b border-border-strong pr-3 text-left text-2xs font-medium uppercase tracking-widest text-text-subtle"
          >
            {t("report.category", locale)}
          </th>
          <th
            scope="col"
            className="border-b border-border-strong pl-3 text-right text-2xs font-medium uppercase tracking-widest text-text-subtle"
          >
            {t("report.total", locale)}
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.label} className="h-10 border-b border-border">
            <th scope="row" className="pr-3 text-left font-normal">
              {r.label}
            </th>
            <td className="tabular pl-3 text-right text-figure">{r.text}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export interface PeriodSeries {
  /** Already resolved text -- a translated string or a category label from
   *  data, never a key. Callers translate at the call site, the same way
   *  `PnlTable` renders `line.label` directly rather than looking it up. */
  label: string;
  values: readonly string[];
}

export function PeriodTable({
  periods,
  series,
  locale,
}: Readonly<{ periods: readonly Period[]; series: readonly PeriodSeries[]; locale: Locale }>) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="h-10">
            <th
              scope="col"
              className="border-b border-border-strong pr-3 text-left text-2xs font-medium uppercase tracking-widest text-text-subtle"
            >
              {t("report.category", locale)}
            </th>
            {periods.map((p) => (
              <th
                key={p}
                scope="col"
                className="border-b border-border-strong px-3 text-right text-2xs font-medium uppercase tracking-widest text-text-subtle"
              >
                {formatPeriodShort(p, locale)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {series.map((s) => (
            <tr key={s.label} className="h-10 border-b border-border">
              <th scope="row" className="pr-3 text-left font-normal">
                {s.label}
              </th>
              {s.values.map((v, i) => (
                <td key={periods[i]} className="tabular px-3 text-right text-figure">
                  {v}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

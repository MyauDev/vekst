/**
 * The Management P&L.
 *
 * Separate from any general table component by design (D11): freezing the
 * category column, freezing the header and scrolling sideways is this table's
 * whole job, and nothing else in the product does it. Folding that into a
 * shared table makes the most complex component in the codebase the one used
 * correctly in exactly one place.
 *
 * `DESIGN.md` §5: hairlines between groups, **no zebra striping** -- zebra
 * fights the state colours -- and horizontal scroll is expected rather than a
 * defect.
 *
 * Rows are 40px since the §5 amendment of 2026-09-16, up from 32px. §1.1's
 * rows-per-screen argument has not gone away, which is why the step was one
 * and not three: a twelve-period P&L of this length still arrives in a single
 * screen, and that is the measure to re-check if anyone raises it again.
 *
 * The frozen column and header paint `surface-raised` rather than `surface`
 * because the table now sits inside a raised card. A frozen cell has to match
 * the surface it slides over or the freeze becomes visible as a colour seam.
 *
 * `report.lines` is flat and printed in order -- no section grouping, no
 * subtotal row of its own: `core/internal/report/pnl.go`'s `Order` is
 * exactly the twelve rows in this table, sections and the five computed
 * results (GM, NM, CM, IBT, NI) already interleaved (design D3), so a
 * computed line's own emphasis (bold, a heavier top border) is what used to
 * be `SubtotalRow`'s job. Below the table, `report.buckets` -- what the
 * table could not include, in the fixed order CLAUDE.md names: unclassified,
 * non-P&L, unallocated, other basis.
 */
import { Link } from "@tanstack/react-router";

import { NO_DATA, formatMinorUnits, exponentOf, localeTag } from "../money";
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import { formatPeriodShort } from "../ui/period";
import type { Money } from "../data/types";
import type { Report, ReportLine, ReportBucket, BucketKind } from "../data/report";

const BUCKET_KEY: Record<BucketKind, MessageKey> = {
  unclassified: "report.bucket.unclassified",
  non_pnl: "report.bucket.non_pnl",
  unallocated: "report.bucket.unallocated",
  other_basis: "report.bucket.other_basis",
};

function fmt(m: Money, locale: Locale): string {
  const exp = exponentOf(m.currencyCode);
  if (exp === undefined) return NO_DATA;
  return formatMinorUnits(m.minorUnits, exp, locale);
}

/** A ratio, not money (design §2.4's own money callout draws this line): a
 *  JS `number` is correct here, the same call the wire's own field makes. */
function fmtPercent(ratio: number | undefined, locale: Locale): string {
  if (ratio === undefined) return "";
  return new Intl.NumberFormat(localeTag(locale), {
    style: "percent",
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(ratio);
}

/** Every figure cell: right-aligned, tabular, and a link to its own transactions. */
function Figure({
  value,
  categoryId,
  period,
  locale,
  from,
  to,
  linked,
}: Readonly<{
  value: Money;
  categoryId: string;
  period: string;
  locale: Locale;
  from: string;
  to: string;
  linked: boolean;
}>) {
  const text = fmt(value, locale);
  // The landing renders this same table to show the real product rather than a
  // picture of it, but its figures lead nowhere: a marketing page that drops
  // the reader into a sign-in wall mid-scroll has spent their attention badly.
  if (!linked) {
    return <td className="tabular px-3 text-right text-figure">{text}</td>;
  }
  return (
    <td className="tabular p-0 text-right">
      {/* A link rather than a click handler: the drill-down is a route, so this
          survives a copy-paste, a middle click and the back button. */}
      <Link
        to="/app/reports/pnl/cell/$categoryId/$period"
        params={{ categoryId, period }}
        // A figure cell only exists in the table view -- this table is never
        // rendered under the charts tab -- so the panel opens back onto it.
        search={{ from, to, view: "table" }}
        // A colour step on press, never a transform: this cell is a figure, and
        // a figure that moves under the finger is a figure that looks like it
        // is still being computed (§5).
        className="block px-3 py-1.5 text-figure hover:bg-surface-sunken active:bg-border"
      >
        {text}
      </Link>
    </td>
  );
}

function Row({
  line,
  report,
  locale,
  from,
  to,
  linked,
}: Readonly<{
  line: ReportLine;
  report: Report;
  locale: Locale;
  from: string;
  to: string;
  linked: boolean;
}>) {
  return (
    <tr
      className={
        line.computed
          ? "h-10 border-b-2 border-border-strong font-medium"
          : "h-10 border-b border-border"
      }
    >
      <th
        scope="row"
        className="sticky left-0 z-10 whitespace-nowrap bg-surface-raised pr-3 pl-4 text-left font-normal"
      >
        {line.label}
      </th>
      {line.values.map((v, i) => (
        <Figure
          key={report.periods[i]}
          value={v}
          categoryId={line.categoryId}
          period={report.periods[i]!}
          locale={locale}
          from={from}
          to={to}
          linked={linked}
        />
      ))}
      <td className="tabular px-3 text-right font-medium">{fmt(line.total, locale)}</td>
      <td className="tabular px-3 text-right text-text-muted">
        {fmtPercent(line.percentOfRevenue, locale)}
      </td>
    </tr>
  );
}

function BucketRow({
  bucket,
  report,
  locale,
  from,
  to,
  linked,
}: Readonly<{
  bucket: ReportBucket;
  report: Report;
  locale: Locale;
  from: string;
  to: string;
  linked: boolean;
}>) {
  return (
    <tr className="h-10 border-b border-border">
      <th
        scope="row"
        className="sticky left-0 z-10 whitespace-nowrap bg-surface-raised pr-3 pl-4 text-left text-2xs font-medium uppercase tracking-wider text-text-muted"
      >
        {t(BUCKET_KEY[bucket.kind], locale)}
      </th>
      {bucket.values.map((v, i) => (
        <Figure
          key={report.periods[i]}
          value={v}
          categoryId={bucket.kind}
          period={report.periods[i]!}
          locale={locale}
          from={from}
          to={to}
          linked={linked}
        />
      ))}
      <td className="tabular px-3 text-right font-medium text-text-muted">{fmt(bucket.total, locale)}</td>
      {/* Buckets carry no percentage of revenue -- nothing is computed from
          them (pnl.go's own comment on BucketOrder). */}
      <td />
    </tr>
  );
}

export function PnlTable({
  report,
  locale,
  from,
  to,
  linked = true,
}: Readonly<{
  report: Report;
  locale: Locale;
  from: string;
  to: string;
  /** False on the landing, where a figure must not lead into the application. */
  linked?: boolean;
}>) {
  return (
    // The table scrolls inside its own box; the page never scrolls sideways.
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="h-10">
            {/* The corner carries the currency, once. §4: repeating a code in
                every cell costs a column of width and tells the reader nothing
                they do not already know. */}
            <th
              scope="col"
              // Wide enough for the longest category label to sit on one line.
              // At 40px rows a wrapped label makes its row taller than its
              // neighbours, and a column of figures whose rows are different
              // heights is harder to read across than one that scrolls.
              className="sticky left-0 top-0 z-20 whitespace-nowrap border-b border-border-strong bg-surface-raised pr-3 pl-4 text-left"
            >
              <span className="text-2xs uppercase tracking-widest text-text-subtle">
                {t("report.category", locale)} · {report.currencyCode}
              </span>
            </th>
            {report.periods.map((p) => (
              <th
                key={p}
                scope="col"
                className="sticky top-0 z-10 border-b border-border-strong bg-surface-raised px-3 text-right text-2xs font-medium uppercase tracking-widest text-text-subtle"
              >
                {formatPeriodShort(p, locale)}
              </th>
            ))}
            <th
              scope="col"
              className="sticky top-0 z-10 border-b border-border-strong bg-surface-raised px-3 text-right text-2xs font-semibold uppercase tracking-widest text-text"
            >
              {t("report.total", locale)}
            </th>
            <th
              scope="col"
              className="sticky top-0 z-10 border-b border-border-strong bg-surface-raised px-3 text-right text-2xs font-medium uppercase tracking-widest text-text-subtle"
            >
              {t("report.percentOfRevenue", locale)}
            </th>
          </tr>
        </thead>

        <tbody>
          {report.lines.map((line) => (
            <Row
              key={line.categoryId}
              line={line}
              report={report}
              locale={locale}
              from={from}
              to={to}
              linked={linked}
            />
          ))}
        </tbody>

        <tfoot>
          <tr className="h-11 font-semibold">
            <th scope="row" className="sticky left-0 z-10 whitespace-nowrap bg-surface-raised pr-3 pl-4 text-left">
              {t("report.net", locale)}
            </th>
            {report.netByPeriod.map((m, i) => (
              <td key={report.periods[i]} className="tabular px-3 text-right">
                {fmt(m, locale)}
              </td>
            ))}
            <td className="tabular px-3 text-right">{fmt(report.netTotal, locale)}</td>
            <td />
          </tr>
        </tfoot>

        {report.buckets.length > 0 ? (
          <tbody>
            {report.buckets.map((bucket) => (
              <BucketRow
                key={bucket.kind}
                bucket={bucket}
                report={report}
                locale={locale}
                from={from}
                to={to}
                linked={linked}
              />
            ))}
          </tbody>
        ) : null}
      </table>
    </div>
  );
}

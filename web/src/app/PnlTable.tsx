/**
 * The Management P&L.
 *
 * Separate from any general table component by design (D11): freezing the
 * category column, freezing the header and scrolling sideways is this table's
 * whole job, and nothing else in the product does it. Folding that into a
 * shared table makes the most complex component in the codebase the one used
 * correctly in exactly one place.
 *
 * `DESIGN.md` §5: 32px rows, hairlines between groups, **no zebra striping** --
 * zebra fights the state colours -- and horizontal scroll is expected rather
 * than a defect. §1.1 is why the rows are tight: an accountant comparing twelve
 * periods scrolls to compare if padding lowers the rows per screen.
 */
import { Link } from "@tanstack/react-router";

import { NO_DATA, formatMinorUnits, exponentOf } from "../money";
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import { formatPeriodShort } from "../ui/period";
import type { Money } from "../data/types";
import type { Report, ReportLine, ReportSection } from "../data/report";

function fmt(m: Money | null, locale: Locale): string {
  if (!m) return NO_DATA;
  const exp = exponentOf(m.currencyCode);
  if (exp === undefined) return NO_DATA;
  return formatMinorUnits(m.minorUnits, exp, locale);
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
}: {
  value: Money | null;
  categoryId: string;
  period: string;
  locale: Locale;
  from: string;
  to: string;
  linked: boolean;
}) {
  const text = fmt(value, locale);
  if (!value) {
    return <td className="tabular px-3 text-right text-figure-blocked">{text}</td>;
  }
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
        search={{ from, to }}
        className="block px-3 py-1.5 text-figure hover:bg-surface-sunken"
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
}: {
  line: ReportLine;
  report: Report;
  locale: Locale;
  from: string;
  to: string;
  linked: boolean;
}) {
  // A blocked line names its reason where the line is, and spans the columns it
  // cannot fill. DESIGN.md §2: "blocked" without the missing input named is a
  // dead end.
  if (line.blockedReason) {
    return (
      <tr className="h-8 border-b border-border">
        <th
          scope="row"
          className="sticky left-0 z-10 bg-surface pr-3 pl-4 text-left font-normal text-text-muted"
        >
          {line.label}
        </th>
        <td colSpan={report.periods.length + 2} className="px-3">
          <span className="text-2xs font-medium uppercase tracking-wider text-danger">
            {t("report.blocked", locale)}
          </span>
          <span className="ml-2 text-xs text-text-muted">
            {t(`report.blocked.${line.blockedReason}` as MessageKey, locale)}
          </span>
        </td>
      </tr>
    );
  }

  return (
    <tr className="h-8 border-b border-border">
      <th
        scope="row"
        className="sticky left-0 z-10 bg-surface pr-3 pl-4 text-left font-normal"
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
      <td className="tabular px-3 text-right text-text-muted">{line.percentOfRevenue ?? ""}</td>
    </tr>
  );
}

function SubtotalRow({
  section,
  locale,
  periods,
}: {
  section: ReportSection;
  locale: Locale;
  periods: readonly string[];
}) {
  return (
    <tr className="h-8 border-b-2 border-border-strong font-medium">
      <th scope="row" className="sticky left-0 z-10 bg-surface pr-3 pl-4 text-left">
        {section.label}
      </th>
      {section.subtotals.map((m, i) => (
        <td key={periods[i]} className="tabular px-3 text-right">
          {fmt(m, locale)}
        </td>
      ))}
      <td className="tabular px-3 text-right">{fmt(section.total, locale)}</td>
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
}: {
  report: Report;
  locale: Locale;
  from: string;
  to: string;
  /** False on the landing, where a figure must not lead into the application. */
  linked?: boolean;
}) {
  return (
    // The table scrolls inside its own box; the page never scrolls sideways.
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-sm">
        <thead>
          <tr className="h-8">
            {/* The corner carries the currency, once. §4: repeating a code in
                every cell costs a column of width and tells the reader nothing
                they do not already know. */}
            <th
              scope="col"
              className="sticky left-0 top-0 z-20 border-b border-border-strong bg-surface pr-3 pl-4 text-left"
            >
              <span className="text-2xs uppercase tracking-widest text-text-subtle">
                {t("report.category", locale)} · {report.currencyCode}
              </span>
            </th>
            {report.periods.map((p) => (
              <th
                key={p}
                scope="col"
                className="sticky top-0 z-10 border-b border-border-strong bg-surface px-3 text-right text-2xs font-medium uppercase tracking-widest text-text-subtle"
              >
                {formatPeriodShort(p, locale)}
              </th>
            ))}
            <th
              scope="col"
              className="sticky top-0 z-10 border-b border-border-strong bg-surface px-3 text-right text-2xs font-semibold uppercase tracking-widest text-text"
            >
              {t("report.total", locale)}
            </th>
            <th
              scope="col"
              className="sticky top-0 z-10 border-b border-border-strong bg-surface px-3 text-right text-2xs font-medium uppercase tracking-widest text-text-subtle"
            >
              {t("report.percentOfRevenue", locale)}
            </th>
          </tr>
        </thead>

        {report.sections.map((section) => (
          <tbody key={section.id}>
            {section.lines.map((line) => (
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
            <SubtotalRow section={section} locale={locale} periods={report.periods} />
          </tbody>
        ))}

        <tfoot>
          <tr className="h-9 font-semibold">
            <th scope="row" className="sticky left-0 z-10 bg-surface pr-3 pl-4 text-left">
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
      </table>
    </div>
  );
}

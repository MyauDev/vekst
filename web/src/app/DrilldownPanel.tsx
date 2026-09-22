/**
 * The transactions -- or, for a computed line, the operands -- behind one
 * figure, as a panel over the report.
 *
 * A route rather than a state flag (design D8), so the panel has a URL that
 * survives a copy-paste and the back button closes it rather than leaving the
 * report. The report stays mounted underneath.
 *
 * §5 allows a shadow here: this is a true overlay, which is the only place one
 * is permitted. 120ms, and it is the only motion in the application -- carried
 * by `.panel-enter` in the token layer, which spends `--duration-panel` and
 * `--ease-panel`. It slides in from the edge it is anchored to, so the panel
 * emerges from where it lives rather than fading in from nowhere.
 *
 * The entrance is animated and the dismissal is not: the panel is a route, and
 * the router unmounts it the moment the back button fires. Holding it mounted
 * to play an exit would put a render concern inside navigation, which is a
 * worse trade than an asymmetry nobody has complained about.
 *
 * Two answer kinds, not one: GM has no transactions of its own, it is NET
 * SALES minus CS, and both of those have transactions -- so a computed
 * line's cell opens onto its operands instead, each itself a link into
 * another drill-down (report.proto's own words for
 * `REPORT_ANSWER_KIND_OPERANDS`). `rowsUnavailable` is gone: the fixture's
 * "this figure's rows are not in the sample" has no backend equivalent --
 * every request now returns whatever is really there, including zero.
 */
import { Link, useParams, useSearch } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { getDrilldown } from "../data/report";
import type { DrilldownOperand, DrilldownRow } from "../data/report";
import { NO_DATA, exponentOf, formatMinorUnits } from "../money";
import { t } from "../i18n";
import type { Locale } from "../i18n";
import { formatPeriodLong } from "../ui/period";
import { useLocale } from "../ui/preferences";
import { cellName, lineName } from "./lineName";
import { ErrorState, Loading } from "../ui/feedback";

function fmt(m: { minorUnits: string; currencyCode: string }, locale: Locale): string {
  const exp = exponentOf(m.currencyCode);
  return exp === undefined ? NO_DATA : formatMinorUnits(m.minorUnits, exp, locale);
}

/**
 * The transactions behind one figure. A scroll region, not a virtual list.
 *
 * One category in one month is tens of rows, occasionally hundreds — below the
 * point where windowing pays for itself, and windowing has a real cost: it needs
 * a measured viewport, so it renders nothing until layout arrives. The review
 * queue in section 7 is the list that genuinely needs it, and that is where
 * `@tanstack/react-virtual` earns its place.
 */
function Rows({ rows, locale }: Readonly<{ rows: readonly DrilldownRow[]; locale: Locale }>) {
  return (
    <div className="max-h-96 overflow-y-auto">
      {rows.map((row) => (
        <div
          key={row.id}
          className="flex h-10 items-center gap-3 border-b border-border text-xs"
        >
          <span className="tabular w-20 shrink-0 text-text-muted">{row.bookedOn}</span>
          <span className="min-w-0 flex-1 truncate">{row.description}</span>
          <span className="w-20 shrink-0 text-text-muted">{row.categoryLabel}</span>
          {/* The engine layer and the confidence are what make a classification
              auditable -- DESIGN.md §2 forbids removing them. */}
          <span className="w-8 shrink-0 text-2xs uppercase tracking-wider text-text-subtle">
            {row.layer}
          </span>
          <span className="tabular w-10 shrink-0 text-right text-2xs text-text-subtle">
            {/* Absent on an unclassified row -- one of the bucket drill-downs
                this same panel now opens, not only a line's. */}
            {row.confidence !== undefined ? `${Math.round(row.confidence * 100)}%` : NO_DATA}
          </span>
          <span className="tabular w-24 shrink-0 text-right text-figure">
            {fmt(row.amount, locale)}
          </span>
        </div>
      ))}
    </div>
  );
}

/** What a computed line is made of. Each operand opens its own drill-down in
 *  turn -- GM has no transactions of its own; NET SALES and CS do. */
function Operands({
  operands,
  period,
  search,
  locale,
}: Readonly<{
  operands: readonly DrilldownOperand[];
  period: string;
  search: { from: string; to: string; view: "table" | "charts" };
  locale: Locale;
}>) {
  return (
    <ul className="flex flex-col gap-1 py-4">
      {operands.map((o) => (
        <li key={o.categoryId}>
          <Link
            to="/app/reports/pnl/cell/$categoryId/$period"
            params={{ categoryId: o.categoryId, period }}
            search={search}
            className="flex items-center justify-between gap-3 rounded px-2 py-2 text-sm text-text hover:bg-surface-sunken active:bg-border"
          >
            {/* The readable name, the same one the table row above carries
                -- an operand list reading "NET SALES" and "CS" tells the
                reader what GM is keyed by, not what it is made of. */}
            <span>{lineName(o.categoryId, o.label, locale)}</span>
            <span className="text-2xs uppercase tracking-widest text-text-subtle">
              {o.subtracted ? "−" : "+"}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function DrilldownPanel() {
  const { categoryId, period } = useParams({
    from: "/app/reports/pnl/cell/$categoryId/$period",
  });
  const search = useSearch({ from: "/app/reports/pnl" });
  const [locale] = useLocale();

  const { data, error, isPending } = useQuery({
    queryKey: ["drilldown", categoryId, period, search.from, search.to],
    queryFn: () => getDrilldown({ categoryId, period, from: search.from, to: search.to }),
  });

  const empty = data && data.kind === "transactions" && data.rows.length === 0;

  return (
    <div className="panel-enter fixed inset-y-0 right-0 z-30 flex w-full max-w-2xl flex-col border-l border-border bg-overlay shadow-panel">
      <header className="flex items-start gap-3 border-b border-border px-8 py-6">
        <div className="min-w-0">
          {/* A shared link arrives with no memory of the click, so the panel has
              to say which figure this is rather than assume the reader knows. */}
          <h2 className="truncate text-lg font-semibold tracking-tight">
            {/* A bucket has no human name on the wire, only a kind, so a
                panel opened on one used to be headed "unclassified"
                verbatim; a line arrives as the taxonomy's abbreviation.
                `cellName` answers both vocabularies -- see
                `app/lineName.ts`. */}
            {cellName(categoryId, data?.categoryLabel ?? categoryId, locale)}
          </h2>
          <p className="mt-0.5 text-2xs uppercase tracking-widest text-text-subtle">
            {formatPeriodLong(period, locale)}
          </p>
        </div>
        {data ? (
          <span className="tabular ml-auto shrink-0 text-lg font-semibold text-figure">
            {fmt(data.amount, locale)}
          </span>
        ) : null}
        <Link
          to="/app/reports/pnl"
          search={search}
          aria-label={t("drilldown.close", locale)}
          className="ml-3 shrink-0 border-b border-transparent pb-px text-2xs uppercase tracking-widest text-text-subtle hover:border-text hover:text-text active:text-text-muted"
        >
          {t("drilldown.close", locale)}
        </Link>
      </header>

      <div className="min-h-0 grow overflow-hidden px-8">
        {isPending ? <Loading label="…" rows={4} /> : null}
        {error ? <ErrorState message={String(error)} /> : null}
        {/* No rule of its own: the header above already carries one, and a
            `border-t` under `max-w-prose` stops short of the panel edge, which
            reads as a broken divider rather than a deliberate one. */}
        {empty ? (
          <p className="max-w-prose py-6 text-sm text-text-muted">
            {t("drilldown.empty", locale)}
          </p>
        ) : null}
        {data?.kind === "operands" ? (
          <>
            <h3 className="text-2xs font-semibold uppercase tracking-widest text-text-subtle">
              {t("drilldown.operands", locale)}
            </h3>
            <Operands operands={data.operands} period={period} search={search} locale={locale} />
          </>
        ) : null}
        {data?.kind === "transactions" && !empty ? <Rows rows={data.rows} locale={locale} /> : null}
      </div>

      {data ? (
        <footer className="border-t border-border px-8 py-4">
          {/* The three strings that make a March report reproduce in June. */}
          <p className="tabular text-2xs text-text-subtle">
            {t("drilldown.provenance", locale)
              .replace("{taxonomy}", data.provenance.taxonomyVersion)
              .replace("{ruleset}", data.provenance.rulesetVersion)
              .replace("{engine}", data.provenance.engineVersion)}
          </p>
        </footer>
      ) : null}
    </div>
  );
}

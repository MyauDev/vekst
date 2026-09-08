/**
 * The transactions behind one figure, as a panel over the report.
 *
 * A route rather than a state flag (design D8), so the panel has a URL that
 * survives a copy-paste and the back button closes it rather than leaving the
 * report. The report stays mounted underneath.
 *
 * §5 allows a shadow here: this is a true overlay, which is the only place one
 * is permitted. 120ms, and it is the only motion in the application.
 */
import { useRef } from "react";
import { Link, useParams, useSearch } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";

import { getDrilldown } from "../data/report";
import type { DrilldownRow } from "../data/report";
import { NO_DATA, exponentOf, formatMinorUnits } from "../money";
import { t } from "../i18n";
import type { Locale } from "../i18n";
import { formatPeriodLong } from "../ui/period";
import { useLocale } from "../ui/preferences";
import { ErrorState, Loading } from "../ui/feedback";

function fmt(m: { minorUnits: string; currencyCode: string }, locale: Locale): string {
  const exp = exponentOf(m.currencyCode);
  return exp === undefined ? NO_DATA : formatMinorUnits(m.minorUnits, exp, locale);
}

function Rows({ rows, locale }: { rows: readonly DrilldownRow[]; locale: Locale }) {
  const parentRef = useRef<HTMLDivElement>(null);
  // Windowed. Five rows here, but a real cell can carry hundreds and the review
  // queue thousands -- FRONTEND_PLAN gap 7. Virtualising once, in the component
  // that owns the list, is cheaper than discovering the need on a customer file.
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 32,
    overscan: 8,
  });

  return (
    <div ref={parentRef} className="max-h-96 overflow-y-auto">
      <div style={{ height: virtual.getTotalSize(), position: "relative" }}>
        {virtual.getVirtualItems().map((item) => {
          const row = rows[item.index]!;
          return (
            <div
              key={row.id}
              className="absolute inset-x-0 flex h-8 items-center gap-3 border-b border-border text-xs"
              style={{ transform: `translateY(${item.start}px)` }}
            >
              <span className="tabular w-20 shrink-0 text-text-muted">{row.bookedOn}</span>
              <span className="min-w-0 flex-1 truncate">{row.description}</span>
              <span className="w-20 shrink-0 text-text-muted">{row.categoryLabel}</span>
              {/* The engine layer and the confidence are what make a
                  classification auditable -- DESIGN.md §2 forbids removing them. */}
              <span className="w-8 shrink-0 text-2xs uppercase tracking-wider text-text-subtle">
                {row.layer}
              </span>
              <span className="tabular w-10 shrink-0 text-right text-2xs text-text-subtle">
                {Math.round(row.confidence * 100)}%
              </span>
              <span className="tabular w-24 shrink-0 text-right text-figure">
                {fmt(row.amount, locale)}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function DrilldownPanel() {
  const { categoryId, period } = useParams({
    from: "/app/reports/pnl/cell/$categoryId/$period",
  });
  const search = useSearch({ from: "/app/reports/pnl" });
  const [locale] = useLocale();

  const { data, error, isPending } = useQuery({
    queryKey: ["drilldown", categoryId, period],
    queryFn: () => getDrilldown({ categoryId, period }),
  });

  return (
    <div className="fixed inset-y-0 right-0 z-30 flex w-full max-w-2xl flex-col border-l border-border bg-overlay shadow-panel">
      <header className="flex items-start gap-3 border-b border-border px-5 py-4">
        <div className="min-w-0">
          {/* A shared link arrives with no memory of the click, so the panel has
              to say which figure this is rather than assume the reader knows. */}
          <h2 className="truncate text-md font-semibold">
            {data?.categoryLabel ?? categoryId}
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
          className="ml-3 shrink-0 border-b border-transparent pb-px text-2xs uppercase tracking-widest text-text-subtle hover:border-text hover:text-text"
        >
          {t("drilldown.close", locale)}
        </Link>
      </header>

      <div className="min-h-0 grow overflow-hidden px-5">
        {isPending ? <Loading label="…" rows={4} /> : null}
        {error ? <ErrorState message={String(error)} /> : null}
        {data && data.rowsUnavailable ? (
          <p className="max-w-prose border-t border-border py-6 text-sm text-text-muted">
            {t("drilldown.unavailable", locale)}
          </p>
        ) : null}
        {data && !data.rowsUnavailable ? <Rows rows={data.rows} locale={locale} /> : null}
      </div>

      {data ? (
        <footer className="border-t border-border px-5 py-3">
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

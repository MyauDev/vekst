/**
 * The Management P&L. The whole MVP, and the Demo's definition of done.
 */
import { Link, Outlet, useSearch } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { getReport, MixedBasisError } from "../data/report";
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import type { ReportView } from "../router";
import { useLocale } from "../ui/preferences";
import { EmptyState, ErrorState, Loading } from "../ui/feedback";
import { CashFlowChart } from "./charts/CashFlowChart";
import { CategoryTrendChart } from "./charts/CategoryTrendChart";
import { ExpenseCategoriesChart } from "./charts/ExpenseCategoriesChart";
import { MoneyFlowChart } from "./charts/MoneyFlowChart";
import { NetResultChart } from "./charts/NetResultChart";
import { RevenueExpenseChart } from "./charts/RevenueExpenseChart";
import { PnlTable } from "./PnlTable";
import { StatTiles } from "./StatTiles";
import { Reconciliation } from "./Reconciliation";

/**
 * §14: no segmented controls and no pills, the same rule `TopBar`'s
 * language/theme `Choice` already follows -- a choice is an underline. State
 * lives in the URL rather than in `useState` so the tab a report was shared
 * on is the tab the link reopens (task: chart placement, 2026-09-19).
 */
function ViewTabs({
  view,
  from,
  to,
  locale,
}: Readonly<{ view: ReportView; from: string; to: string; locale: Locale }>) {
  const tabs: readonly { view: ReportView; labelKey: MessageKey }[] = [
    { view: "table", labelKey: "report.tab.table" },
    { view: "charts", labelKey: "report.tab.charts" },
  ];
  return (
    <nav aria-label={t("report.title", locale)} className="flex items-baseline gap-5 border-b border-border">
      {tabs.map((tab) => (
        <Link
          key={tab.view}
          to="/app/reports/pnl"
          search={{ from, to, view: tab.view }}
          className={
            tab.view === view
              ? "border-b-2 border-text pb-2 text-sm font-semibold text-text"
              : "border-b-2 border-transparent pb-2 text-sm text-text-muted hover:text-text"
          }
          aria-current={tab.view === view ? "page" : undefined}
        >
          {t(tab.labelKey, locale)}
        </Link>
      ))}
    </nav>
  );
}

export function ReportScreen() {
  const { from, to, view } = useSearch({ from: "/app/reports/pnl" });
  const [locale] = useLocale();

  const { data, error, isPending } = useQuery({
    queryKey: ["report", from, to],
    queryFn: () => getReport({ from, to }),
  });

  // The header renders in every branch below: a loading, blocked or empty
  // report is exactly the state a person needs the screen's own identity
  // for -- the period control that used to live here has moved to `TopBar`,
  // visible (and changeable) on every screen now rather than only this one
  // (`ui/periodPreference.ts`). The basis line only ever has something to
  // say once `data` exists, so it is gated on `data` itself rather than
  // threaded through `isPending`/`error`.
  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("report.title", locale)}</h1>
        {data ? (
          <>
            {/*
              The basis sits with the table, never in a footnote -- DESIGN.md §8 and
              WORKFLOW.md §5.1. It is derived from source_kind rather than chosen by
              a human, and saying so is what stops a reader treating it as a setting.
            */}
            <span className="text-2xs uppercase tracking-widest text-text">
              {t(data.basis === "cash" ? "report.basis.cash" : "report.basis.accrual", locale)}
            </span>
            <span className="text-2xs text-text-subtle">{t("report.basis.note", locale)}</span>
          </>
        ) : null}
      </header>

      {isPending ? (
        <Loading label={t("report.title", locale)} rows={6} />
      ) : // The one whole-report blocked state, replacing the fixture's per-line
      // one: a report is computed from one source_kind for its whole
      // duration, so an entity mixing bank and ledger imports with no
      // confirmed match is refused entirely rather than guessed at line by
      // line (design §2.5).
      error instanceof MixedBasisError ? (
        <div className="max-w-prose rounded-panel border border-border border-l-2 border-l-danger bg-surface-raised p-6 text-sm text-text">
          <span className="text-2xs font-medium uppercase tracking-wider text-danger">
            {t("report.blocked", locale)}
          </span>
          <p className="mt-2">{t("report.blocked.mixed_basis", locale)}</p>
        </div>
      ) : error ? (
        <ErrorState message={String(error)} />
      ) : // Nothing happened: every line and every bucket totals zero. Lines
      // themselves are always present -- the real backend computes all
      // twelve regardless of data -- so "no report yet" is a fact about the
      // totals, not about how many rows came back.
      data.lines.every((l) => l.total.minorUnits === "0") &&
        data.buckets.every((b) => b.total.minorUnits === "0") ? (
        <EmptyState
          title={t("empty.reports.title", locale)}
          detail={t("empty.reports.detail", locale)}
        />
      ) : (
        <>
          <StatTiles report={data} locale={locale} />

          {/*
            Charts used to sit stacked below the table, after the reconciliation
            strip -- easy to miss on a screen whose defining content is a
            twelve-column table. A tab makes both one click away instead of one
            scroll away, and the tab itself lives in the URL, so a link into
            either view still reopens the same view.
          */}
          <ViewTabs view={view} from={from} to={to} locale={locale} />

          {view === "table" ? (
            <div className="rounded-panel border border-border bg-surface-raised p-6">
              <PnlTable report={data} locale={locale} from={from} to={to} />
            </div>
          ) : (
            // WORKFLOW.md §5.3's table order: money flow, money in and out,
            // top expenses, net result, revenue vs. expenses, category trend.
            // The headline row above (the stat tiles) is that table's first
            // row -- a single number is not a one-bar chart.
            //
            // Money in and out sits second, directly under the Sankey it
            // answers the other half of: the Sankey is where the money went
            // over the whole range, and this is when it moved. Both are the
            // bank rather than the P&L, which is why they are adjacent and
            // why "Revenue against expenses" -- which looks like the same
            // chart and is not the same figures -- stays further down with
            // the other P&L cards.
            <div className="flex flex-col gap-6">
              <MoneyFlowChart report={data} locale={locale} />
              <CashFlowChart report={data} locale={locale} />
              <ExpenseCategoriesChart report={data} locale={locale} />
              <NetResultChart report={data} locale={locale} />
              <RevenueExpenseChart report={data} locale={locale} />
              <CategoryTrendChart report={data} locale={locale} />
            </div>
          )}

          {/* DESIGN.md §2: the reconciliation strip may never be removed by a
              minimal pass. It stays outside the tab switch on purpose -- it is
              what proves the numbers on either tab, not a property of one. */}
          <Reconciliation recon={data.reconciliation} locale={locale} />

          {/* The drill-down renders here, over a report that stays mounted. */}
          <Outlet />
        </>
      )}
    </div>
  );
}

/**
 * The Management P&L. The whole MVP, and the Demo's definition of done.
 */
import { Outlet, useSearch } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { getReport } from "../data/report";
import { t } from "../i18n";
import { useLocale } from "../ui/preferences";
import { EmptyState, ErrorState, Loading } from "../ui/feedback";
import { CategoryTrendChart } from "./charts/CategoryTrendChart";
import { ExpenseCategoriesChart } from "./charts/ExpenseCategoriesChart";
import { MoneyFlowChart } from "./charts/MoneyFlowChart";
import { NetResultChart } from "./charts/NetResultChart";
import { RevenueExpenseChart } from "./charts/RevenueExpenseChart";
import { PnlTable } from "./PnlTable";
import { StatTiles } from "./StatTiles";
import { Reconciliation } from "./Reconciliation";

export function ReportScreen() {
  const { from, to } = useSearch({ from: "/app/reports/pnl" });
  const [locale] = useLocale();

  const { data, error, isPending } = useQuery({
    queryKey: ["report", from, to],
    queryFn: () => getReport({ from, to }),
  });

  if (isPending) return <Loading label={t("report.title", locale)} rows={6} />;
  if (error) return <ErrorState message={String(error)} />;
  if (data.sections.every((s) => s.lines.length === 0)) {
    return (
      <EmptyState
        title={t("empty.reports.title", locale)}
        detail={t("empty.reports.detail", locale)}
      />
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <header className="flex flex-wrap items-baseline gap-x-4 gap-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">{t("report.title", locale)}</h1>
        {/*
          The basis sits with the table, never in a footnote -- DESIGN.md §8 and
          WORKFLOW.md §5.1. It is derived from source_kind rather than chosen by
          a human, and saying so is what stops a reader treating it as a setting.
        */}
        <span className="text-2xs uppercase tracking-widest text-text">
          {t(data.basis === "cash" ? "report.basis.cash" : "report.basis.accrual", locale)}
        </span>
        <span className="text-2xs text-text-subtle">{t("report.basis.note", locale)}</span>
      </header>

      <StatTiles report={data} locale={locale} />

      <div className="rounded-panel border border-border bg-surface-raised p-6">
        <PnlTable report={data} locale={locale} from={from} to={to} />
      </div>

      {/* WORKFLOW.md §5.3's table order: money flow, top expenses, net result,
          revenue vs. expenses, category trend. The headline row above (the
          stat tiles) is that table's first row -- a single number is not a
          one-bar chart. */}
      <MoneyFlowChart report={data} locale={locale} />
      <ExpenseCategoriesChart report={data} locale={locale} />
      <NetResultChart report={data} locale={locale} />
      <RevenueExpenseChart report={data} locale={locale} />
      <CategoryTrendChart report={data} locale={locale} />

      <Reconciliation recon={data.reconciliation} locale={locale} />

      {/* The drill-down renders here, over a report that stays mounted. */}
      <Outlet />
    </div>
  );
}

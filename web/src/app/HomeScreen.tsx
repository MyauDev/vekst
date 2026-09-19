/**
 * Home. The rail's own way back to a start, added 2026-09-19 alongside the
 * `/app` redirect that now lands here instead of on the Management P&L.
 *
 * Three cards in pipeline order — Imports, Review, Reports — each a shortcut
 * that also answers the question a shortcut alone does not: not just "go to
 * Imports" but "how many batches, and what did the last one do." Every
 * number on this screen is real, from the same queries the destinations
 * themselves use (`["reviewSummary"]`, `["report", from, to]`), never a
 * placeholder invented for the sake of filling a card.
 *
 * The account card is the identity summary decided for this pass -- name,
 * email, sign out, the same data `AppLayout` already renders into the rail's
 * account slot. A real settings surface (organisation, billing,
 * notifications) is a different, larger feature with no settings data
 * behind it yet; this is not a placeholder for that screen, it is the whole
 * of what "account" means in the product today.
 */
import { Link, useRouteContext } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";

import { AccountCard } from "./AccountCard";
import { listBatches } from "../data/imports";
import { getReport } from "../data/report";
import { reviewSummary } from "../data/review";
import { t } from "../i18n";
import type { Locale, MessageKey } from "../i18n";
import { NO_DATA, exponentOf, formatMinorUnits } from "../money";
import { defaultRange, formatPeriodRange } from "../ui/period";
import { useLocale } from "../ui/preferences";
import { BatchStateChip } from "../ui/StateChip";

/** The card shell every widget on this screen shares: title, a way in, one
 *  headline figure. Register B's card idiom (DESIGN.md §5 as amended
 *  2026-09-17) -- `rounded-panel`, `surface-raised` on the page's own
 *  `surface-sunken` canvas -- same as everywhere else behind sign-in. */
function Card({
  to,
  titleKey,
  locale,
  children,
}: Readonly<{ to: string; titleKey: MessageKey; locale: Locale; children: React.ReactNode }>) {
  return (
    <Link
      to={to}
      className="flex flex-col gap-3 rounded-panel border border-border bg-surface-raised p-6 hover:border-border-strong active:bg-surface-sunken"
    >
      <h2 className="text-2xs font-semibold uppercase tracking-widest text-text-subtle">
        {t(titleKey, locale)}
      </h2>
      {children}
    </Link>
  );
}

function ImportsCard({ locale }: Readonly<{ locale: Locale }>) {
  const { data } = useQuery({ queryKey: ["batches"], queryFn: listBatches });
  const latest = data?.[0];

  return (
    <Card to="/app/imports" titleKey="nav.imports" locale={locale}>
      <p className="tabular text-2xl font-semibold tracking-tight">
        {data ? data.length : NO_DATA}
      </p>
      <p className="text-2xs uppercase tracking-widest text-text-subtle">
        {t("home.imports.count", locale)}
      </p>
      {latest ? (
        <div className="mt-2 flex items-center gap-2 border-t border-border pt-3 text-xs">
          <span className="text-text-subtle">{t("home.imports.latest", locale)}</span>
          <span className="min-w-0 flex-1 truncate text-text-muted">{latest.fileName}</span>
          <BatchStateChip state={latest.state} locale={locale} />
        </div>
      ) : null}
    </Card>
  );
}

function ReviewCard({ locale }: Readonly<{ locale: Locale }>) {
  const { data } = useQuery({ queryKey: ["reviewSummary"], queryFn: reviewSummary });
  const exp = data ? exponentOf(data.amount.currencyCode) : undefined;
  const amount =
    data && exp !== undefined ? formatMinorUnits(data.amount.minorUnits, exp, locale) : NO_DATA;

  return (
    <Card to="/app/review" titleKey="nav.review" locale={locale}>
      <p className="tabular text-2xl font-semibold tracking-tight">
        {data ? data.groups : NO_DATA}
      </p>
      <p className="text-2xs uppercase tracking-widest text-text-subtle">
        {t("home.review.awaiting", locale)}
      </p>
      {data && data.groups > 0 ? (
        <p className="tabular mt-2 border-t border-border pt-3 text-xs text-text-muted">{amount}</p>
      ) : null}
    </Card>
  );
}

function ReportsCard({ locale }: Readonly<{ locale: Locale }>) {
  const range = defaultRange();
  const { data } = useQuery({
    queryKey: ["report", range.from, range.to],
    queryFn: () => getReport(range),
  });
  const exp = data ? exponentOf(data.currencyCode) : undefined;
  const net =
    data && exp !== undefined ? formatMinorUnits(data.netTotal.minorUnits, exp, locale) : NO_DATA;

  return (
    <Card to="/app/reports/pnl" titleKey="nav.reports" locale={locale}>
      <p className="tabular text-2xl font-semibold tracking-tight text-figure">{net}</p>
      <p className="text-2xs uppercase tracking-widest text-text-subtle">
        {t("home.reports.net", locale)}
      </p>
      <p className="mt-2 border-t border-border pt-3 text-xs text-text-muted">
        {formatPeriodRange(range.from, range.to, locale)}
      </p>
    </Card>
  );
}

export function HomeScreen() {
  const [locale] = useLocale();
  // A router hook, not a connect import -- this file stays a screen the
  // "no screen imports a transport" rule is happy with. `AccountCard` is
  // the deliberate, named exception, the same shape `AppLayout` already is.
  const { transport } = useRouteContext({ from: "/app/home" });

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">{t("home.title", locale)}</h1>

      {/* WORKFLOW.md's pipeline order, the same order the rail itself uses:
          data in, data corrected, data read. */}
      <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
        <ImportsCard locale={locale} />
        <ReviewCard locale={locale} />
        <ReportsCard locale={locale} />
      </div>

      <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
        <AccountCard transport={transport} locale={locale} />
      </div>
    </div>
  );
}

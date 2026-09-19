/**
 * The left rail. 240px, 40px rows, four items and no fifth.
 *
 * **Home, then pipeline order** — Imports, Review, Reports below it is still
 * the order of `WORKFLOW.md`'s pipeline: data in, data corrected, data read.
 * Home is not a pipeline stage and sits above that order rather than inside
 * it, added 2026-09-19 as the rail's own way back to a start now that `/app`
 * redirects there instead of straight to the P&L. Reports stays last: it is
 * the pipeline's destination, Home is the rail's.
 *
 * **No icons.** `docs/DESIGN.md` §2 removes icon-plus-label wherever the label
 * alone is clear, and one word per row could not be clearer. §14 keeps an icon
 * only where it carries what a word cannot. A rail of plain words reads as a
 * table of contents, which is the reference this product is working from.
 *
 * **The active row is a rule and a weight, not a filled pill.** §14: a tinted
 * rounded row is every component library's nav item.
 */
import { Link } from "@tanstack/react-router";

import { t } from "../i18n";
import type { Locale } from "../i18n";

/** The only badge in the interface. The review queue is the one screen whose
 *  work expires: an unreviewed row is a wrong number in a report, which is why
 *  `WORKFLOW.md` §5.3 also puts the unreviewed amount among the headline
 *  figures. Nothing else earns a count. */
function Count({ value, locale }: Readonly<{ value: number; locale: Locale }>) {
  if (value <= 0) return null;
  return (
    <span
      className="tabular ml-auto text-2xs font-medium text-warn"
      aria-label={`${value} ${t("nav.awaitingReview", locale)}`}
    >
      {value}
    </span>
  );
}

function Item({
  to,
  label,
  count,
  locale,
}: Readonly<{
  to: string;
  label: string;
  count?: number;
  locale: Locale;
}>) {
  return (
    <Link
      to={to}
      // Press feedback is a colour step and nothing else: instant, so the row
      // answers on pointer-down rather than on release, and flat, because a
      // row that scales under the finger moves the label it is carrying.
      className="group mx-2 flex h-10 items-center gap-2 rounded-panel px-3 text-sm text-text-muted hover:bg-surface-sunken hover:text-text active:bg-border"
      activeProps={{
        className: "bg-surface-sunken font-medium text-text",
        "aria-current": "page",
      }}
    >
      {/* The rule sits in the row rather than beside it, so the label does not
          shift by two pixels when it becomes active. */}
      <span className="h-4 w-0.5 shrink-0 bg-transparent group-aria-[current=page]:bg-text" />
      <span>{label}</span>
      {count === undefined ? null : <Count value={count} locale={locale} />}
    </Link>
  );
}

export function Rail({
  locale,
  awaitingReview = 0,
  account,
}: Readonly<{
  locale: Locale;
  awaitingReview?: number;
  account?: React.ReactNode;
}>) {
  return (
    <nav
      aria-label={t("nav.primary", locale)}
      className="flex w-60 shrink-0 flex-col border-r border-border bg-surface"
    >
      <div className="flex h-16 items-center px-5">
        <span className="text-md font-semibold tracking-tight">{t("app.title", locale)}</span>
      </div>

      <div className="flex flex-col gap-1 py-2">
        <Item to="/app/home" label={t("nav.home", locale)} locale={locale} />
        <Item to="/app/imports" label={t("nav.imports", locale)} locale={locale} />
        <Item
          to="/app/review"
          label={t("nav.review", locale)}
          count={awaitingReview}
          locale={locale}
        />
        <Item to="/app/reports/pnl" label={t("nav.reports", locale)} locale={locale} />
      </div>

      {account ? (
        <div className="mt-auto border-t border-border px-5 py-4">{account}</div>
      ) : null}
    </nav>
  );
}

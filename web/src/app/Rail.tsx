/**
 * The left rail. 208px, 32px rows, three items and no fourth.
 *
 * **Pipeline order, not importance order** — Imports, Review, Reports. It is
 * the order of `WORKFLOW.md`'s pipeline: data in, data corrected, data read. A
 * rail is a map rather than a ranking, and read top to bottom this one teaches
 * the shape of the product. Reports is the destination and the default route,
 * which is exactly why it is last.
 *
 * **No icons.** `docs/DESIGN.md` §2 removes icon-plus-label wherever the label
 * alone is clear, and three words could not be clearer. §14 keeps an icon only
 * where it carries what a word cannot. A rail of three words reads as a table
 * of contents, which is the reference this product is working from.
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
      className="group flex h-8 items-center gap-2 pl-3 pr-3 text-sm text-text-muted"
      activeProps={{
        className: "font-medium text-text",
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
      aria-label={t("nav.reports", locale)}
      className="flex w-52 shrink-0 flex-col border-r border-border"
    >
      <div className="flex h-12 items-center border-b border-border px-3">
        <span className="text-md font-semibold tracking-tight">{t("app.title", locale)}</span>
      </div>

      <div className="flex flex-col py-2">
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
        <div className="mt-auto border-t border-border px-3 py-3">{account}</div>
      ) : null}
    </nav>
  );
}

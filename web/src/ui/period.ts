/**
 * Period ranges. `YYYY-MM` in and out -- months are the grain of every report
 * this product makes, so month precision is the honest unit and the URL stays
 * readable (`INFORMATION_ARCHITECTURE.md` §7).
 */
import type { Locale } from "../i18n";

const BCP47: Record<Locale, string> = { en: "en-GB", ru: "ru-RU" };
const EN_DASH = "–";

/** Defaults to the current year to date. An accountant's year is the unit that
 *  matters; a trailing twelve months crosses the boundary where their mental
 *  model breaks. */
export function defaultRange(now = new Date()): { from: string; to: string } {
  const year = now.getUTCFullYear();
  const month = String(now.getUTCMonth() + 1).padStart(2, "0");
  return { from: `${year}-01`, to: `${year}-${month}` };
}

function parse(period: string): Date | undefined {
  const m = /^(\d{4})-(0[1-9]|1[0-2])$/.exec(period);
  if (!m) return undefined;
  return new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, 1));
}

/**
 * "Jan – Aug 2026", or "Nov 2025 – Aug 2026" when the range crosses a year.
 * The year is printed once where it can be, because a period control that
 * repeats it spends width on a word the reader already has.
 */
export function formatPeriodRange(from: string, to: string, locale: Locale): string {
  const a = parse(from);
  const b = parse(to);
  if (!a || !b) return `${from} ${EN_DASH} ${to}`;

  const tag = BCP47[locale];
  const month = new Intl.DateTimeFormat(tag, { month: "short", timeZone: "UTC" });
  const monthYear = new Intl.DateTimeFormat(tag, {
    month: "short",
    year: "numeric",
    timeZone: "UTC",
  });

  if (a.getUTCFullYear() === b.getUTCFullYear()) {
    return `${month.format(a)} ${EN_DASH} ${monthYear.format(b)}`;
  }
  return `${monthYear.format(a)} ${EN_DASH} ${monthYear.format(b)}`;
}

/** "Mar" — a column header. The year lives in the range label, not in every
 *  column: a table that repeats it spends width on a word the reader has. */
export function formatPeriodShort(period: string, locale: Locale): string {
  const d = parse(period);
  if (!d) return period;
  return new Intl.DateTimeFormat(BCP47[locale], { month: "short", timeZone: "UTC" }).format(d);
}

/** "March 2026" — for a drill-down panel, which arrives with no memory of the
 *  column its figure came from. */
export function formatPeriodLong(period: string, locale: Locale): string {
  const d = parse(period);
  if (!d) return period;
  return new Intl.DateTimeFormat(BCP47[locale], {
    month: "long", year: "numeric", timeZone: "UTC",
  }).format(d);
}

/*
 * Mock money formatting.
 *
 * The wire carries `minor_units` as a STRING (see src/money.test.ts and the
 * invariant in CLAUDE.md): a JS number cannot hold an int64 exactly. So every
 * function here takes and returns strings and sums in BigInt. No float touches
 * an amount at any point.
 *
 * This is mock code. The real formatter is change 5.1a and belongs in
 * src/money.ts with its own tests.
 */
// One locale concept in the repo, not two: src/i18n.ts owns it.
export type { Locale } from "../i18n";
import type { Locale } from "../i18n";

const MINUS = "−"; // U+2212, not a hyphen: it aligns with the digits.
const NBSP = " ";

/** Format int64 minor units for display. `exponent` is the currency's, 2 for EUR. */
export function formatMinorUnits(minorUnits: string, lang: Locale, exponent = 2): string {
  const negative = minorUnits.startsWith("-");
  const digits = (negative ? minorUnits.slice(1) : minorUnits).padStart(exponent + 1, "0");
  const whole = digits.slice(0, digits.length - exponent);
  const fraction = digits.slice(digits.length - exponent);

  const groupMark = lang === "ru" ? NBSP : ",";
  const decimalMark = lang === "ru" ? "," : ".";
  const grouped = whole.replace(/\B(?=(\d{3})+(?!\d))/g, groupMark);

  return `${negative ? MINUS : ""}${grouped}${decimalMark}${fraction}`;
}

/** Sum minor-unit strings. Returns a minor-unit string. */
export function sumMinorUnits(values: readonly string[]): string {
  return values.reduce((acc, v) => acc + BigInt(v), 0n).toString();
}

/**
 * Percent of revenue, to one decimal.
 *
 * A percentage is a ratio, not an amount, so it may become a number here —
 * but only after both sides have been reduced to a safe magnitude.
 */
export function percentOfRevenue(amount: string, revenue: string, lang: Locale): string {
  const revenueBig = BigInt(revenue);
  if (revenueBig === 0n) return "";
  const tenths = (BigInt(amount) * 1000n) / revenueBig;
  const negative = tenths < 0n;
  const abs = (negative ? -tenths : tenths).toString().padStart(2, "0");
  const decimalMark = lang === "ru" ? "," : ".";
  return `${negative ? MINUS : ""}${abs.slice(0, -1)}${decimalMark}${abs.slice(-1)}%`;
}

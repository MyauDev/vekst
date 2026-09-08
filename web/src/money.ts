/**
 * Money, on the client.
 *
 * The one place a wrong number can be introduced after the backend got it
 * right, which is why it is shared code with its own tests rather than
 * formatting done inside a component (`docs/FRONTEND_PLAN.md` §5).
 *
 * **No float touches an amount, ever.** The wire carries `minor_units` as a
 * string because a JavaScript `number` cannot hold an `int64` exactly, so every
 * function here takes and returns strings and sums in `BigInt`. `money.test.ts`
 * guards the generated field's type; these functions are what keeps the
 * guarantee once the value is in hand.
 *
 * Display rules are `docs/DESIGN.md` §4.
 */
import type { Locale } from "./i18n";

/**
 * ISO-4217 minor-unit digits. **Transcribed from `core/internal/money/exponents.go`,
 * which is the original.** The two must agree: a client that scales an amount
 * differently from the server prints a different number from the one that was
 * computed, and nothing would catch it.
 *
 * Source: the ISO 4217 active currency & funds code list, transcribed
 * 2026-09-02. ISO revises the list, so a missing currency may simply postdate
 * that date; update the Go table, this one and both dates together.
 */
const EXPONENT: Readonly<Record<string, number>> = {
  AED: 2, AFN: 2, ALL: 2, AMD: 2, ANG: 2, AOA: 2, ARS: 2, AUD: 2,
  AWG: 2, AZN: 2, BAM: 2, BBD: 2, BDT: 2, BGN: 2, BHD: 3, BIF: 0,
  BMD: 2, BND: 2, BOB: 2, BOV: 2, BRL: 2, BSD: 2, BTN: 2, BWP: 2,
  BYN: 2, BZD: 2, CAD: 2, CDF: 2, CHE: 2, CHF: 2, CHW: 2, CLF: 4,
  CLP: 0, CNY: 2, COP: 2, COU: 2, CRC: 2, CUC: 2, CUP: 2, CVE: 2,
  CZK: 2, DJF: 0, DKK: 2, DOP: 2, DZD: 2, EGP: 2, ERN: 2, ETB: 2,
  EUR: 2, FJD: 2, FKP: 2, GBP: 2, GEL: 2, GHS: 2, GIP: 2, GMD: 2,
  GNF: 0, GTQ: 2, GYD: 2, HKD: 2, HNL: 2, HTG: 2, HUF: 2, IDR: 2,
  ILS: 2, INR: 2, IQD: 3, IRR: 2, ISK: 0, JMD: 2, JOD: 3, JPY: 0,
  KES: 2, KGS: 2, KHR: 2, KMF: 0, KPW: 2, KRW: 0, KWD: 3, KYD: 2,
  KZT: 2, LAK: 2, LBP: 2, LKR: 2, LRD: 2, LSL: 2, LYD: 3, MAD: 2,
  MDL: 2, MGA: 2, MKD: 2, MMK: 2, MNT: 2, MOP: 2, MRU: 2, MUR: 2,
  MVR: 2, MWK: 2, MXN: 2, MXV: 2, MYR: 2, MZN: 2, NAD: 2, NGN: 2,
  NIO: 2, NOK: 2, NPR: 2, NZD: 2, OMR: 3, PAB: 2, PEN: 2, PGK: 2,
  PHP: 2, PKR: 2, PLN: 2, PYG: 0, QAR: 2, RON: 2, RSD: 2, RUB: 2,
  RWF: 0, SAR: 2, SBD: 2, SCR: 2, SDG: 2, SEK: 2, SGD: 2, SHP: 2,
  SLE: 2, SOS: 2, SRD: 2, SSP: 2, STN: 2, SVC: 2, SYP: 2, SZL: 2,
  THB: 2, TJS: 2, TMT: 2, TND: 3, TOP: 2, TRY: 2, TTD: 2, TWD: 2,
  TZS: 2, UAH: 2, UGX: 0, USD: 2, USN: 2, UYI: 0, UYU: 2, UYW: 4,
  UZS: 2, VED: 2, VES: 2, VND: 0, VUV: 0, WST: 2, XAF: 0, XAG: 0,
  XAU: 0, XBA: 0, XBB: 0, XBC: 0, XBD: 0, XCD: 2, XDR: 0, XOF: 0,
  XPD: 0, XPF: 0, XPT: 0, XSU: 0, XUA: 0, YER: 2, ZAR: 2, ZMW: 2,
  ZWL: 2,};

/** U+2212. Not a hyphen: the minus aligns with the digits in a tabular column. */
export const MINUS = "−";

/** What a line with no data renders as. §4: an accountant must be able to tell
 *  "nothing happened" from "nothing was loaded", so this is never a zero. */
export const NO_DATA = "—";

/** Thrown rather than guessed. An unknown code is never defaulted to 2 -- that
 *  is how a KWD amount, whose exponent is 3, becomes wrong by a factor of ten
 *  in a report nobody re-checks. Mirrors `money.ErrUnknownCurrency` in core. */
export class UnknownCurrencyError extends Error {
  constructor(readonly code: string) {
    super(`money: unknown currency code ${code}`);
    this.name = "UnknownCurrencyError";
  }
}

/** The minor-unit digits for a code, or `undefined`. Mirrors `money.Exponent`. */
export function exponentOf(code: string): number | undefined {
  return EXPONENT[code.toUpperCase()];
}

const BCP47: Record<Locale, string> = { en: "en-GB", ru: "ru-RU" };

/** The locale's decimal separator, read from Intl rather than assumed. */
function decimalSeparator(locale: Locale): string {
  const part = new Intl.NumberFormat(BCP47[locale])
    .formatToParts(1.1)
    .find((p) => p.type === "decimal");
  return part ? part.value : ".";
}

/**
 * Formats integer minor units for display, without the currency code.
 *
 * §4 puts the code once in a column header rather than in every cell, so this
 * returns the figure alone. Grouping and the decimal mark come from `Intl`:
 * `ru` groups with a space and marks the decimal with a comma, and hardcoding
 * either is how the second language quietly renders wrong.
 *
 * The integer part is formatted as a `BigInt`, so an amount past
 * `Number.MAX_SAFE_INTEGER` survives intact.
 */
export function formatMinorUnits(minorUnits: string, exponent: number, locale: Locale): string {
  const negative = minorUnits.startsWith("-");
  const digits = (negative ? minorUnits.slice(1) : minorUnits).padStart(exponent + 1, "0");

  const whole = digits.slice(0, digits.length - exponent);
  const fraction = exponent === 0 ? "" : digits.slice(digits.length - exponent);

  const grouped = new Intl.NumberFormat(BCP47[locale]).format(BigInt(whole));
  const body = fraction ? `${grouped}${decimalSeparator(locale)}${fraction}` : grouped;

  // §6: a negative figure is not an error. A minus sign, never parentheses --
  // parentheses are an English accounting convention and the Demo ships `ru` as
  // a first-class language -- and never red, which is reserved for failure.
  return negative ? `${MINUS}${body}` : body;
}

/** Formats an amount and its currency. Throws on a code the table does not carry. */
export function formatMoney(
  money: { minorUnits: string; currencyCode: string },
  locale: Locale,
): string {
  const exponent = exponentOf(money.currencyCode);
  if (exponent === undefined) throw new UnknownCurrencyError(money.currencyCode);
  return formatMinorUnits(money.minorUnits, exponent, locale);
}

/** Sums minor-unit strings in `BigInt`. Returns a minor-unit string. */
export function sumMinorUnits(values: readonly string[]): string {
  return values.reduce((acc, v) => acc + BigInt(v), 0n).toString();
}

/**
 * Percent of a base, to one decimal place.
 *
 * A percentage is a ratio rather than an amount, so it may become a number --
 * but only after both sides have been divided down to a safe magnitude, which
 * the `BigInt` arithmetic below does before anything is converted.
 *
 * Returns `undefined` when the base is zero: there is no percentage of nothing,
 * and rendering `0.0%` would assert one.
 */
export function percentOf(amount: string, base: string, locale: Locale): string | undefined {
  const baseBig = BigInt(base);
  if (baseBig === 0n) return undefined;

  const tenths = (BigInt(amount) * 1000n) / baseBig;
  const negative = tenths < 0n;
  const abs = (negative ? -tenths : tenths).toString().padStart(2, "0");
  const body = `${abs.slice(0, -1)}${decimalSeparator(locale)}${abs.slice(-1)}%`;
  return negative ? `${MINUS}${body}` : body;
}

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/** Repo-relative, anchored to this file rather than to the working directory:
 *  a test that passes or fails depending on where it was invoked from is not
 *  testing what it claims to. */
const repo = (...p: string[]) => join(dirname(fileURLToPath(import.meta.url)), "..", "..", ...p);

import { describe, expect, it } from "vitest";

import {
  MINUS,
  NO_DATA,
  UnknownCurrencyError,
  exponentOf,
  formatMinorUnits,
  formatMoney,
  percentOf,
  sumMinorUnits,
} from "./money";

describe("no float touches an amount", () => {
  it("renders an amount past Number.MAX_SAFE_INTEGER exactly", () => {
    // 2^53 + 1. As a JS number this is 9007199254740992 -- the last digit is
    // lost -- so a formatter that parses to a number fails here and nowhere
    // else, which is why the assertion is on the exact digits.
    const exact = formatMoney({ minorUnits: "9007199254740993", currencyCode: "EUR" }, "en");
    expect(exact).toBe("90,071,992,547,409.93");
  });

  it("sums in BigInt without losing a unit", () => {
    const total = sumMinorUnits(["9007199254740993", "1", "-2"]);
    expect(total).toBe("9007199254740992");
  });
});

describe("currency exponent", () => {
  it("renders a currency whose exponent is not 2 in its own denomination", () => {
    // JPY has no minor unit: 1234 minor units is 1,234 yen, not 12.34.
    expect(formatMoney({ minorUnits: "1234", currencyCode: "JPY" }, "en")).toBe("1,234");
    // KWD has three. The same digits are a thousandth of the EUR reading.
    expect(formatMoney({ minorUnits: "1234", currencyCode: "KWD" }, "en")).toBe("1.234");
    expect(formatMoney({ minorUnits: "1234", currencyCode: "EUR" }, "en")).toBe("12.34");
    // CLF has four.
    expect(formatMoney({ minorUnits: "1234", currencyCode: "CLF" }, "en")).toBe("0.1234");
  });

  it("throws on an unknown code rather than assuming 2", () => {
    // Defaulting is how a KWD amount becomes wrong by a factor of ten in a
    // report nobody re-checks. Mirrors money.ErrUnknownCurrency in core.
    expect(() => formatMoney({ minorUnits: "1234", currencyCode: "ZZZ" }, "en")).toThrow(
      UnknownCurrencyError,
    );
    expect(exponentOf("ZZZ")).toBeUndefined();
  });

  it("agrees with core/internal/money/exponents.go, which is the original", () => {
    // Two tables that disagree print two different numbers for one amount and
    // nothing else would catch it.
    const go = readFileSync(repo("core/internal/money/exponents.go"), "utf8");
    const body = go.split("var exponent = map[string]int{")[1]!.split("}")[0]!;
    const pairs = [...body.matchAll(/"([A-Z]{3})":\s*(\d)/g)];

    expect(pairs.length).toBeGreaterThan(150);
    for (const [, code, digits] of pairs) {
      expect(exponentOf(code!), `exponent for ${code}`).toBe(Number(digits));
    }
  });
});

describe("display rules — DESIGN.md §4 and §6", () => {
  it("uses a minus sign, not parentheses and not a hyphen", () => {
    const negative = formatMoney({ minorUnits: "-681240", currencyCode: "EUR" }, "en");
    expect(negative).toBe(`${MINUS}6,812.40`);
    expect(negative).not.toContain("(");
    expect(negative).not.toContain("-"); // U+002D
  });

  it("renders a true zero as a figure, distinct from no data", () => {
    expect(formatMoney({ minorUnits: "0", currencyCode: "EUR" }, "en")).toBe("0.00");
    expect(NO_DATA).toBe("—");
  });

  it("changes grouping with the locale without changing the value", () => {
    const units = "128460000";
    const en = formatMinorUnits(units, 2, "en");
    const ru = formatMinorUnits(units, 2, "ru");

    expect(en).toBe("1,284,600.00");
    expect(ru).toContain(","); // ru marks the decimal with a comma
    expect(ru).not.toContain(".");
    expect(en).not.toBe(ru);
    // Same underlying units either way -- only the presentation moved.
    expect(ru.replace(/\D/g, "")).toBe(en.replace(/\D/g, ""));
  });

  it("omits the currency code, which belongs in the column header", () => {
    expect(formatMoney({ minorUnits: "1234", currencyCode: "EUR" }, "en")).not.toMatch(/EUR|€/);
  });
});

describe("percent of a base", () => {
  it("computes to one decimal without a float in the amount path", () => {
    expect(percentOf("2500", "10000", "en")).toBe("25.0%");
    expect(percentOf("-681240", "1284600", "en")).toBe(`${MINUS}53.0%`);
  });

  it("returns nothing for a zero base rather than asserting 0.0%", () => {
    expect(percentOf("1234", "0", "en")).toBeUndefined();
  });
});

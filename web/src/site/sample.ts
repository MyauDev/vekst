/**
 * The figures the landing page shows.
 *
 * Content, not application state. It lives in `site/` so this directory stays
 * self-contained: `ARCHITECTURE.md` §8 schedules a static marketing bundle at
 * `/site` for Commercial, and a landing page that loads a screen's data cannot
 * be lifted into one without a rewrite.
 *
 * The types come from `data/`, imported **as types only** — erased at build, so
 * nothing of the data layer reaches the marketing bundle. That is the line: the
 * page may render the product's real components, and may not read the product's
 * state.
 *
 * Six periods rather than the report's twelve, because the point here is to be
 * recognisable at a glance rather than complete.
 *
 * Twelve lines, not a category breakdown per section: the real
 * `GetManagementPNL` returns exactly the top-level sections and the five
 * computed results (`core/internal/report/pnl.go`'s own `Order`), never a
 * per-leaf-category row -- detail lives behind the drill-down, not in the
 * table. A sample with more rows than the real table ever has would be the
 * one thing this file must not be: a picture of a product that does not
 * exist.
 */
import { sumMinorUnits } from "../money";
import type { Money } from "../data/types";
import type { Report, ReportLine } from "../data/report";

const CURRENCY = "EUR";
const PERIODS = ["2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08"];
const money = (minorUnits: string): Money => ({ minorUnits, currencyCode: CURRENCY });

/** One section's raw values by period -- everything below is derived from
 *  these, the same chain `core/internal/report/pnl.go`'s `Chain` computes. */
const RAW: Record<string, readonly string[]> = {
  "01": ["7060000", "7100000", "6710000", "7290000", "7220000", "7580000"], // NET SALES
  "02": ["-2440000", "-2340000", "-2240000", "-2550000", "-2390000", "-2560000"], // CS
  "03": ["-336000", "-325000", "-299000", "-347000", "-329000", "-355000"], // OCS
  "04": ["-2601240", "-2691860", "-2684025", "-2770580", "-2725535", "-2846090"], // OPEX
  "05": ["-42000", "-38000", "-40000", "-41000", "-39000", "-43000"], // OIE
};

function line(code: string, label: string, computed: boolean, values: readonly string[]): ReportLine {
  return {
    categoryId: code,
    label,
    computed,
    values: values.map(money),
    total: money(sumMinorUnits(values)),
  };
}

function perPeriod(fn: (i: number) => string): readonly string[] {
  return PERIODS.map((_, i) => fn(i));
}

/** Derived exactly as the report is, so the sample cannot fail to add up. */
export const SAMPLE_REPORT: Report = (() => {
  const netSales = line("01", "NET SALES", false, RAW["01"]!);
  const cs = line("02", "Cost of sales", false, RAW["02"]!);
  const gmValues = perPeriod((i) => sumMinorUnits([RAW["01"]![i]!, RAW["02"]![i]!]));
  const gm = line("91", "GM", true, gmValues);

  const ocs = line("03", "Other cost of sales", false, RAW["03"]!);
  const nmValues = perPeriod((i) => sumMinorUnits([gmValues[i]!, RAW["03"]![i]!]));
  const nm = line("92", "NM", true, nmValues);

  const opex = line("04", "Operating expenses", false, RAW["04"]!);
  const oie = line("05", "Other expenses", false, RAW["05"]!);
  const cmValues = perPeriod((i) => sumMinorUnits([nmValues[i]!, RAW["04"]![i]!, RAW["05"]![i]!]));
  const cm = line("93", "CM", true, cmValues);

  // No financial result or tax line in the sample -- IBT and NI both equal CM.
  const ibt = line("94", "IBT", true, cmValues);
  const ni = line("95", "NI", true, cmValues);

  const lines = [netSales, cs, gm, ocs, nm, opex, oie, cm, ibt, ni];

  return {
    currencyCode: CURRENCY,
    periods: PERIODS,
    basis: "cash",
    lines,
    buckets: [],
    netByPeriod: ni.values,
    netTotal: ni.total,
    revenueTotal: netSales.total,
    // Derived the same way report.ts derives it: revenueTotal - CM, in BigInt,
    // never a string trimmed of its own sign.
    expensesTotal: money((BigInt(netSales.total.minorUnits) - BigInt(cm.total.minorUnits)).toString()),
    expensesByPeriod: perPeriod((i) =>
      (BigInt(netSales.values[i]!.minorUnits) - BigInt(cm.values[i]!.minorUnits)).toString(),
    ).map(money),
    reconciliation: {
      opening: money("15230000"), moneyIn: money("0"),
      moneyOut: money("0"), transfers: money("0"), closing: money("15230000"),
    },
    provenance: { taxonomyVersion: "v3", rulesetVersion: "v11", engineVersion: "0.4.2" },
    unreviewedAmount: money("0"),
  };
})();

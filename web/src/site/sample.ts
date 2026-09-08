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
 */
import { sumMinorUnits } from "../money";
import type { Money } from "../data/types";
import type { Report, ReportSection, SectionId } from "../data/report";

const CURRENCY = "EUR";
const PERIODS = ["2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08"];
const money = (minorUnits: string): Money => ({ minorUnits, currencyCode: CURRENCY });

const RAW: Record<SectionId, { label: string; rows: [string, string, string[]][] }> = {
  revenue: {
    label: "Revenue",
    rows: [
      ["product-sales", "Product sales", ["5590000","5460000","5150000","5780000","5540000","5840000"]],
      ["services", "Services", ["1470000","1640000","1560000","1510000","1680000","1740000"]],
    ],
  },
  cost_of_sales: {
    label: "Cost of sales",
    rows: [
      ["materials", "Materials", ["-2440000","-2340000","-2240000","-2550000","-2390000","-2560000"]],
      ["inbound-freight", "Inbound freight", ["-336000","-325000","-299000","-347000","-329000","-355000"]],
    ],
  },
  operating_expenses: {
    label: "Operating expenses",
    rows: [
      ["payroll", "Payroll", ["-1920000","-2010000","-2010000","-2010000","-2010000","-2070000"]],
      ["logistics", "Logistics", ["-681240","-631860","-624025","-710580","-665535","-726090"]],
      ["rent-and-utilities", "Rent and utilities", ["-350000","-350000","-350000","-350000","-350000","-350000"]],
    ],
  },
};

function section(id: SectionId): ReportSection {
  const { label, rows } = RAW[id];
  const lines = rows.map(([categoryId, lineLabel, units]) => ({
    categoryId,
    label: lineLabel,
    section: id,
    values: units.map(money),
    total: money(sumMinorUnits(units)),
  }));
  const subtotals = PERIODS.map((_, i) =>
    money(sumMinorUnits(rows.map(([, , units]) => units[i]!))),
  );
  return {
    id,
    label,
    lines,
    subtotals,
    total: money(sumMinorUnits(subtotals.map((m) => m.minorUnits))),
  };
}

/** Derived exactly as the report is, so the sample cannot fail to add up. */
export const SAMPLE_REPORT: Report = (() => {
  const sections = (["revenue", "cost_of_sales", "operating_expenses"] as const).map(section);
  const netByPeriod = PERIODS.map((_, i) =>
    money(sumMinorUnits(sections.map((s) => s.subtotals[i]!.minorUnits))),
  );
  const revenue = sections[0]!;
  return {
    currencyCode: CURRENCY,
    periods: PERIODS,
    basis: "cash",
    sourceKinds: ["bank"],
    sections,
    netByPeriod,
    netTotal: money(sumMinorUnits(netByPeriod.map((m) => m.minorUnits))),
    revenueTotal: revenue.total,
    expensesTotal: money(
      sumMinorUnits(sections.slice(1).map((s) => s.total.minorUnits)),
    ),
    reconciliation: {
      opening: money("15230000"), moneyIn: money("0"),
      moneyOut: money("0"), transfers: money("0"), closing: money("15230000"),
    },
    provenance: { taxonomyVersion: "v3", rulesetVersion: "v11", engineVersion: "0.4.2" },
    unreviewedAmount: money("0"),
  };
})();

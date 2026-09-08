/**
 * The Management P&L and the transactions behind any figure in it.
 *
 * Every total, subtotal, column total and percentage below is **computed from
 * the rows the table renders**. Nothing is typed in. A fixture with a
 * hand-written total is a fixture that can disagree with itself, and the one
 * defect this product cannot ship is a report whose parts do not add up.
 *
 * The real backend computes these server-side, so the shapes returned here are
 * the shapes the RPC will return -- the component never learns which it got.
 */
import { sumMinorUnits, percentOf } from "../money";
import type { Basis, EngineLayer, Money, Period, Provenance, SourceKind } from "./types";

export type SectionId = "revenue" | "cost_of_sales" | "operating_expenses";

export interface ReportLine {
  /** Stable across taxonomy versions: it addresses the drill-down URL, and
   *  those URLs are read by people and pasted into messages. */
  categoryId: string;
  label: string;
  section: SectionId;
  /** One entry per period. `null` is no data for that period, which is not a
   *  zero -- a zero means nothing happened. */
  values: readonly (Money | null)[];
  total: Money | null;
  /** Percent of total revenue, already formatted. Absent when revenue is zero. */
  percentOfRevenue?: string;
  /** Set when the line is refused rather than computed. A blocked line carries
   *  no values: `DESIGN.md` §2 requires the reason to be shown wherever the
   *  line is, and the reason is the content. */
  blockedReason?: string;
}

export interface ReportSection {
  id: SectionId;
  label: string;
  lines: readonly ReportLine[];
  /** Per-period subtotals, then the section total. */
  subtotals: readonly Money[];
  total: Money;
}

/** Opening + in + out + transfers = closing. What proves nothing was dropped. */
export interface Reconciliation {
  opening: Money;
  moneyIn: Money;
  moneyOut: Money;
  transfers: Money;
  closing: Money;
}

export interface Report {
  currencyCode: string;
  periods: readonly Period[];
  basis: Basis;
  /** What the basis was derived from. A mixed set is why a line can be blocked. */
  sourceKinds: readonly SourceKind[];
  sections: readonly ReportSection[];
  /** Per-period net result, then the net total. */
  netByPeriod: readonly Money[];
  netTotal: Money;
  revenueTotal: Money;
  expensesTotal: Money;
  reconciliation: Reconciliation;
  provenance: Provenance;
  /** Amount awaiting review. A headline figure because an unreviewed row is a
   *  wrong number in this very report -- `WORKFLOW.md` §5.3. */
  unreviewedAmount: Money;
}

/* --------------------------------------------------------------------------
 * Fixtures. Raw lines only -- every aggregate below is derived.
 * ----------------------------------------------------------------------- */

const CURRENCY = "EUR";
const PERIODS: readonly Period[] = [
  "2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08",
];

const SECTION_LABEL: Record<SectionId, string> = {
  revenue: "Revenue",
  cost_of_sales: "Cost of sales",
  operating_expenses: "Operating expenses",
};

interface RawLine {
  categoryId: string;
  label: string;
  section: SectionId;
  units?: readonly string[];
  blockedReason?: string;
}

const RAW: readonly RawLine[] = [
  { categoryId: "product-sales", label: "Product sales", section: "revenue",
    units: ["5240000","5120000","5590000","5460000","5150000","5780000","5540000","5840000"] },
  { categoryId: "services", label: "Services", section: "revenue",
    units: ["1410000","1520000","1470000","1640000","1560000","1510000","1680000","1740000"] },
  // Blocked on purpose. A line drawing on ledger and bank data with no confirmed
  // D4 match counts an invoice and its payment twice, so it is refused rather
  // than computed -- and the screen has to say why.
  { categoryId: "consulting-income", label: "Consulting income", section: "revenue",
    blockedReason: "mixed_sources_no_match" },
  { categoryId: "materials", label: "Materials", section: "cost_of_sales",
    units: ["-2270000","-2210000","-2440000","-2340000","-2240000","-2550000","-2390000","-2560000"] },
  { categoryId: "inbound-freight", label: "Inbound freight", section: "cost_of_sales",
    units: ["-303000","-292000","-336000","-325000","-299000","-347000","-329000","-355000"] },
  { categoryId: "payroll", label: "Payroll", section: "operating_expenses",
    units: ["-1920000","-1920000","-1920000","-2010000","-2010000","-2010000","-2010000","-2070000"] },
  { categoryId: "logistics", label: "Logistics", section: "operating_expenses",
    units: ["-664020","-603075","-681240","-631860","-624025","-710580","-665535","-726090"] },
  { categoryId: "rent-and-utilities", label: "Rent and utilities", section: "operating_expenses",
    units: ["-350000","-350000","-350000","-350000","-350000","-350000","-350000","-350000"] },
  { categoryId: "software", label: "Software and subscriptions", section: "operating_expenses",
    units: ["-156600","-156600","-162150","-162150","-162150","-166600","-166600","-166600"] },
  // A real expense pattern: quarterly, so half the cells are a true zero rather
  // than no data. The table must render those differently.
  { categoryId: "professional-fees", label: "Professional fees", section: "operating_expenses",
    units: ["-78000","0","-125500","0","-66500","0","-192000","0"] },
];

const money = (minorUnits: string): Money => ({ minorUnits, currencyCode: CURRENCY });

/** Sums one index across many lines, skipping blocked ones. */
function columnSum(lines: readonly RawLine[], index: number): string {
  return sumMinorUnits(lines.flatMap((l) => (l.units ? [l.units[index]!] : [])));
}

function buildSection(id: SectionId): ReportSection {
  const raw = RAW.filter((l) => l.section === id);
  const subtotals = PERIODS.map((_, i) => money(columnSum(raw, i)));
  const total = money(sumMinorUnits(subtotals.map((m) => m.minorUnits)));

  const lines: ReportLine[] = raw.map((l) => {
    if (!l.units) {
      return {
        categoryId: l.categoryId,
        label: l.label,
        section: id,
        values: PERIODS.map(() => null),
        total: null,
        blockedReason: l.blockedReason,
      };
    }
    return {
      categoryId: l.categoryId,
      label: l.label,
      section: id,
      values: l.units.map((u) => money(u)),
      total: money(sumMinorUnits(l.units)),
    };
  });

  return { id, label: SECTION_LABEL[id], lines, subtotals, total };
}

/**
 * The report for a period range.
 *
 * `async` although nothing awaits: the signature is the one the RPC will have,
 * so swapping the body does not ripple into every caller's control flow.
 */
export async function getReport(_params: { from: Period; to: Period }): Promise<Report> {
  const sections = (["revenue", "cost_of_sales", "operating_expenses"] as const).map(buildSection);

  const revenue = sections.find((s) => s.id === "revenue")!;
  const expenseSections = sections.filter((s) => s.id !== "revenue");

  const netByPeriod = PERIODS.map((_, i) =>
    money(sumMinorUnits(sections.map((s) => s.subtotals[i]!.minorUnits))),
  );
  const netTotal = money(sumMinorUnits(netByPeriod.map((m) => m.minorUnits)));
  const expensesTotal = money(
    sumMinorUnits(expenseSections.map((s) => s.total.minorUnits)),
  );

  // Percent of revenue, computed from the revenue total this same call derived.
  for (const section of sections) {
    for (const line of section.lines) {
      if (line.total) {
        const pct = percentOf(line.total.minorUnits, revenue.total.minorUnits, "en");
        if (pct !== undefined) (line as ReportLine).percentOfRevenue = pct;
      }
    }
  }

  const opening = "15230000";
  const moneyIn = sumMinorUnits(
    RAW.flatMap((l) => (l.units ?? []).filter((u) => !u.startsWith("-"))),
  );
  const moneyOut = sumMinorUnits(
    RAW.flatMap((l) => (l.units ?? []).filter((u) => u.startsWith("-"))),
  );
  const transfers = "-418000";
  // Closing is derived, never stated. The strip proves nothing was dropped, and
  // a strip whose closing figure was typed in proves nothing at all.
  const closing = sumMinorUnits([opening, moneyIn, moneyOut, transfers]);

  return {
    currencyCode: CURRENCY,
    periods: PERIODS,
    // Every fixture row is bank-sourced, so the whole report is cash-basis.
    basis: "cash",
    sourceKinds: ["bank"],
    sections,
    netByPeriod,
    netTotal,
    revenueTotal: revenue.total,
    expensesTotal,
    reconciliation: {
      opening: money(opening),
      moneyIn: money(moneyIn),
      moneyOut: money(moneyOut),
      transfers: money(transfers),
      closing: money(closing),
    },
    provenance: {
      taxonomyVersion: "v3",
      rulesetVersion: "v11",
      engineVersion: "0.4.2",
    },
    unreviewedAmount: money("184220"),
  };
}

/* --------------------------------------------------------------------------
 * Drill-down: the transactions behind one figure.
 * ----------------------------------------------------------------------- */

export interface DrilldownRow {
  id: string;
  bookedOn: string;
  description: string;
  amount: Money;
  categoryLabel: string;
  layer: EngineLayer;
  /** 0..1. Below the threshold a row goes to the review queue instead. */
  confidence: number;
  /** D4 match evidence, where a bank row was matched to a ledger document. */
  evidence?: string;
}

export interface Drilldown {
  categoryId: string;
  categoryLabel: string;
  period: Period;
  amount: Money;
  rows: readonly DrilldownRow[];
  provenance: Provenance;
  /** True when this figure's transactions are not in the fixture set. */
  rowsUnavailable?: boolean;
}

const DRILLDOWN_ROWS: readonly DrilldownRow[] = [
  { id: "t1", bookedOn: "2026-03-04", description: "VEKTOR LOGISTIKA OPLATA SCHET 4471",
    amount: money("-184500"), categoryLabel: "Logistics", layer: "L1", confidence: 0.98 },
  { id: "t2", bookedOn: "2026-03-09", description: "DHL EXPRESS INVOICE 88214",
    amount: money("-96240"), categoryLabel: "Logistics", layer: "L0", confidence: 1,
    evidence: "ledger doc 88214, ±0 days" },
  { id: "t3", bookedOn: "2026-03-14", description: "TRANSPORT NORD AS",
    amount: money("-212300"), categoryLabel: "Logistics", layer: "L1", confidence: 0.94 },
  { id: "t4", bookedOn: "2026-03-21", description: "PALLET HIRE Q1",
    amount: money("-68900"), categoryLabel: "Logistics", layer: "L2", confidence: 0.71 },
  { id: "t5", bookedOn: "2026-03-28", description: "VEKTOR LOGISTIKA OPLATA SCHET 4519",
    amount: money("-119300"), categoryLabel: "Logistics", layer: "L1", confidence: 0.97 },
];

/**
 * One cell carries a transaction list: Logistics × March. Populating every cell
 * would mean inventing several hundred transactions, and the plan bans a demo
 * on invented data. Every other cell opens with its real figure and says so.
 */
export async function getDrilldown(params: {
  categoryId: string;
  period: Period;
}): Promise<Drilldown> {
  const report = await getReport({ from: params.period, to: params.period });
  const line = report.sections
    .flatMap((s) => s.lines)
    .find((l) => l.categoryId === params.categoryId);
  const index = report.periods.indexOf(params.period);
  const amount = line?.values[index] ?? money("0");

  const populated = params.categoryId === "logistics" && params.period === "2026-03";
  return {
    categoryId: params.categoryId,
    categoryLabel: line?.label ?? params.categoryId,
    period: params.period,
    amount,
    rows: populated ? DRILLDOWN_ROWS : [],
    rowsUnavailable: !populated,
    provenance: report.provenance,
  };
}

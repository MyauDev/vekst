/**
 * The Management P&L and the transactions behind any figure in it.
 *
 * Real from here on: `GetManagementPNL` and `ListLineTransactions`.
 *
 * The fixture's `sections: ReportSection[]` (three fixed groups: revenue,
 * cost_of_sales, operating_expenses) has no backend equivalent and could not
 * be faked honestly: the real taxonomy has seven top-level sections plus five
 * computed lines, printed flat and interleaved (design D3 -- "01 · 02 · GM ·
 * 03 · NM · 04 · 05 · CM · 06 · IBT · 07 · NI"), never grouped into three.
 * `lines: ReportLine[]` replaces it, in that same flat print order --
 * `core/internal/report/pnl.go`'s own `Order` is exactly twelve entries, not
 * one row per leaf category, so this is a smaller change than it first
 * looks. `buckets` is new: what the table could not include (unclassified,
 * non-P&L, unallocated, other basis) is a real, required part of the
 * response and CLAUDE.md's own invariant ("below the table, in order:
 * unclassified, excluded non-P&L, unallocated, other basis"), which the
 * fixture never rendered because it never had that data to render.
 *
 * `revenueTotal`, `expensesTotal`, `netTotal`, `netByPeriod`,
 * `unreviewedAmount` and `reconciliation` keep their old shape and meaning,
 * derived from `lines`/`buckets` rather than typed in: `revenueTotal` is
 * NET SALES (01); `netTotal`/`netByPeriod` is NI (95), the bottom line;
 * `expensesTotal` is `revenueTotal - CM` (93), since CM = NET SALES - CS -
 * OCS - OPEX - OIE and there is no single "total expenses" line to read;
 * `unreviewedAmount` is the unclassified bucket. `StatTiles` and
 * `Reconciliation` therefore need no changes at all.
 *
 * Every figure is in the organisation's base currency, converted at the rate
 * stored on each row -- never re-derived at read time (design's own words for
 * `versions`, which applies here too).
 *
 * `blockedReason` (per-line, "mixed sources, no confirmed match") is gone:
 * the real backend has no such field on `ReportLine`, because a report is
 * computed from one `source_kind` for its whole duration, chosen before any
 * line is computed -- CLAUDE.md's invariant is enforced by refusing the
 * report's basis, not by blocking individual lines. `getReport` derives that
 * basis from the entity's own imported batches (design §2.5) and throws
 * `MixedBasisError` when they are not uniform, which `ReportScreen` renders
 * as the one whole-report blocked state that replaces the old per-line one.
 */
import { createClient } from "@connectrpc/connect";

import { transport } from "../transport";
import { requireSession } from "./session";
import { listBatches } from "./imports";
import {
  ReportService,
  ReportBasis as ProtoReportBasis,
  ReportGranularity,
  ReportBucketKind,
  ReportAnswerKind,
} from "../gen/vekst/v1/report_pb";
import type {
  ReportLine as ProtoReportLine,
  ReportFigure as ProtoReportFigure,
  ReportBucketLine as ProtoReportBucketLine,
  ReconciliationLine as ProtoReconciliationLine,
  ReportVersions as ProtoReportVersions,
  DrillTransaction as ProtoDrillTransaction,
  ReportOperand as ProtoReportOperand,
} from "../gen/vekst/v1/report_pb";
import type { Money as ProtoMoney } from "../gen/vekst/type/v1/money_pb";
import type { Basis, EngineLayer, Money, Period, Provenance } from "./types";

const client = createClient(ReportService, transport);

function money(m: ProtoMoney | undefined): Money {
  return m ? { minorUnits: m.minorUnits, currencyCode: m.currencyCode } : { minorUnits: "0", currencyCode: "" };
}

/** BigInt subtraction over minor-unit strings. Never a float, the same
 *  discipline `sumMinorUnits` already keeps for addition. */
function subMinorUnits(a: string, b: string): string {
  return (BigInt(a) - BigInt(b)).toString();
}

export class MixedBasisError extends Error {
  constructor() {
    super("report: this entity's imports mix bank and ledger data; no basis can be chosen automatically");
    this.name = "MixedBasisError";
  }
}

/**
 * The basis this report is computed from, derived from the entity's own
 * imported batches (design §2.5) rather than chosen by a human -- a report
 * drawing on both without a confirmed D4 match would count an invoice and
 * its payment twice.
 */
async function deriveBasis(): Promise<ProtoReportBasis> {
  const batches = await listBatches();
  const kinds = new Set(batches.filter((b) => b.state === "imported").map((b) => b.sourceKind));
  if (kinds.size > 1) throw new MixedBasisError();
  return kinds.has("ledger") ? ProtoReportBasis.LEDGER : ProtoReportBasis.BANK;
}

function basisFromProto(b: ProtoReportBasis): Basis {
  return b === ProtoReportBasis.LEDGER ? "accrual" : "cash";
}

export type SectionId = "revenue" | "cost_of_sales" | "operating_expenses";

export interface ReportLine {
  /** The taxonomy code -- '01' for NET SALES, '91' for GM -- stable in a way
   *  a name is not. Addresses the drill-down URL. */
  categoryId: string;
  label: string;
  /** True for GM, NM, CM, IBT, NI: a computed line has operands, not
   *  transactions of its own -- `getDrilldown` opens the two differently. */
  computed: boolean;
  /** One entry per period. Always present: the real backend computes a
   *  figure for every period, even a zero one -- unlike the fixture, there
   *  is no "no data" state at this grain to distinguish from a true zero. */
  values: readonly Money[];
  total: Money;
  /** A ratio, not money -- a JS `number` is correct here, the same call the
   *  wire's own `percent_of_revenue` makes. Absent when revenue is zero. */
  percentOfRevenue?: number;
}

function figureMoney(f: ProtoReportFigure | undefined): Money {
  return money(f?.amount);
}

function lineFromProto(l: ProtoReportLine): ReportLine {
  return {
    categoryId: l.code,
    label: l.label,
    computed: l.computed,
    values: l.byPeriod.map(figureMoney),
    total: figureMoney(l.total),
    percentOfRevenue: l.total?.percentOfRevenue,
  };
}

export type BucketKind = "unclassified" | "non_pnl" | "unallocated" | "other_basis";

const BUCKET_KIND: Record<ReportBucketKind, BucketKind | undefined> = {
  [ReportBucketKind.UNSPECIFIED]: undefined,
  [ReportBucketKind.UNCLASSIFIED]: "unclassified",
  [ReportBucketKind.NON_PNL]: "non_pnl",
  [ReportBucketKind.UNALLOCATED]: "unallocated",
  [ReportBucketKind.OTHER_BASIS]: "other_basis",
};

export interface ReportBucket {
  kind: BucketKind;
  values: readonly Money[];
  total: Money;
}

function bucketFromProto(b: ProtoReportBucketLine): ReportBucket | undefined {
  const kind = BUCKET_KIND[b.kind];
  if (!kind) return undefined;
  return { kind, values: b.byPeriod.map(money), total: money(b.total) };
}

/** Opening + in + out + transfers = closing. What proves nothing was dropped. */
export interface Reconciliation {
  opening: Money;
  moneyIn: Money;
  moneyOut: Money;
  transfers: Money;
  closing: Money;
}

const ZERO: Money = { minorUnits: "0", currencyCode: "" };

/**
 * One strip for the whole range, aggregated from the wire's one-per-period
 * array: opening is the first period's, in/out/transfers are summed across
 * every period, and closing is derived from those four -- never read off any
 * period's own `closing` field, even at the aggregate. The wire's `in` and
 * `out` are positive magnitudes ("that is how they read on a page"); this
 * negates `out` and `transfers` for display, matching the sign convention
 * every other outflow in this product already uses.
 */
function aggregateReconciliation(lines: readonly ProtoReconciliationLine[]): Reconciliation {
  if (lines.length === 0) return { opening: ZERO, moneyIn: ZERO, moneyOut: ZERO, transfers: ZERO, closing: ZERO };

  const currency = lines[0]!.opening?.currencyCode ?? "";
  const sum = (pick: (l: ProtoReconciliationLine) => ProtoMoney | undefined): bigint =>
    lines.reduce((acc, l) => acc + BigInt(pick(l)?.minorUnits ?? "0"), 0n);

  const opening = BigInt(lines[0]!.opening?.minorUnits ?? "0");
  const moneyIn = sum((l) => l.in);
  const moneyOut = sum((l) => l.out);
  const transfers = sum((l) => l.transfers);
  const closing = opening + moneyIn - moneyOut - transfers;

  const m = (v: bigint): Money => ({ minorUnits: v.toString(), currencyCode: currency });
  return {
    opening: m(opening),
    moneyIn: m(moneyIn),
    moneyOut: m(-moneyOut),
    transfers: m(-transfers),
    closing: m(closing),
  };
}

/**
 * One month's actual money movement: what came in, what went out, and the
 * difference.
 *
 * `aggregateReconciliation` above collapses the same wire field to a single
 * strip for the whole range, which is what proves nothing was dropped -- and
 * which answers "did it balance", not "when did the money move". They are
 * different questions and the second one needs the months kept.
 *
 * This is not the P&L's revenue and expenses, and the difference is the point
 * of having both on one screen. `expensesByPeriod` is `NET SALES - CM`: the
 * cost lines of the profit and loss, which exclude CAPEX, exclude everything
 * classified out of the P&L, and (on an accrual basis) are dated to when a
 * cost was incurred rather than when it was paid. This is the bank: every
 * franc that actually left, whatever it was for. A business can be profitable
 * on the first and out of money on the second, and that is precisely the month
 * an owner needs to see.
 *
 * Transfers are deliberately absent. The wire counts them apart from in and
 * out because an organisation moving its own money between its own accounts is
 * neither -- counting it would show a business trading with itself. The
 * reconciliation strip is where they are accounted for, and it stays on screen
 * under both tabs.
 */
export interface CashPeriod {
  period: Period;
  /** Positive: money in. */
  moneyIn: Money;
  /** Negative, the sign convention every outflow in this product uses. */
  moneyOut: Money;
  /** `moneyIn + moneyOut`, so a month that took in less than it spent is
   *  negative -- derived, never read off the wire. */
  net: Money;
}

function cashByPeriod(
  periods: readonly string[],
  lines: readonly ProtoReconciliationLine[],
  currency: string,
): CashPeriod[] {
  const byPeriod = new Map(lines.map((l) => [l.period, l]));
  return periods.map((period) => {
    const line = byPeriod.get(period);
    // The wire's `in` and `out` are both positive magnitudes ("that is how
    // they read on a page"); `out` is negated here for the same reason the
    // aggregate strip negates it.
    const moneyIn = BigInt(line?.in?.minorUnits ?? "0");
    const moneyOut = -BigInt(line?.out?.minorUnits ?? "0");
    const m = (v: bigint): Money => ({ minorUnits: v.toString(), currencyCode: currency });
    return { period, moneyIn: m(moneyIn), moneyOut: m(moneyOut), net: m(moneyIn + moneyOut) };
  });
}

function provenanceFromProto(v: ProtoReportVersions | undefined): Provenance {
  const join = (xs: readonly string[]) => xs.join(", ");
  return {
    taxonomyVersion: join(v?.taxonomy ?? []),
    rulesetVersion: join(v?.ruleset ?? []),
    engineVersion: join(v?.engine ?? []),
  };
}

export interface Report {
  currencyCode: string;
  periods: readonly Period[];
  basis: Basis;
  /** The table, flat and in print order -- sections and computed lines
   *  interleaved, never grouped. */
  lines: readonly ReportLine[];
  /** What the table could not include, in the order CLAUDE.md fixes:
   *  unclassified, non-P&L, unallocated, other basis. */
  buckets: readonly ReportBucket[];
  /** Per-period net result, then the net total. NI (95), the bottom line. */
  netByPeriod: readonly Money[];
  netTotal: Money;
  /** NET SALES (01). */
  revenueTotal: Money;
  /** Derived: revenueTotal - CM (93). CS + OCS + OPEX + OIE, the cost lines
   *  between NET SALES and CM, summed the only way that does not require a
   *  line the backend does not compute. */
  expensesTotal: Money;
  /** The same derivation, per period -- `RevenueExpenseChart`'s own need,
   *  kept here rather than duplicated as BigInt arithmetic inside a chart. */
  expensesByPeriod: readonly Money[];
  /** What actually moved through the accounts each month -- see `CashPeriod`
   *  on why this is not `expensesByPeriod` with a different sign. */
  cash: readonly CashPeriod[];
  reconciliation: Reconciliation;
  provenance: Provenance;
  /** The unclassified bucket's total. A headline figure because an
   *  unreviewed row is a wrong number in this very report. */
  unreviewedAmount: Money;
}

const NET_SALES = "01";
const CM = "93";
const NI = "95";

function reportFromProto(
  basis: Basis,
  periods: readonly string[],
  protoLines: readonly ProtoReportLine[],
  protoBuckets: readonly ProtoReportBucketLine[],
  protoReconciliation: readonly ProtoReconciliationLine[],
  versions: ProtoReportVersions | undefined,
): Report {
  const lines = protoLines.map(lineFromProto);
  const buckets = protoBuckets.map(bucketFromProto).filter((b): b is ReportBucket => b !== undefined);
  const byCode = new Map(lines.map((l) => [l.categoryId, l]));

  const revenue = byCode.get(NET_SALES);
  const cm = byCode.get(CM);
  const ni = byCode.get(NI);
  const unclassified = buckets.find((b) => b.kind === "unclassified");

  const revenueTotal = revenue?.total ?? ZERO;
  const currency = revenueTotal.currencyCode || lines[0]?.total.currencyCode || "";

  return {
    currencyCode: currency,
    periods,
    basis,
    lines,
    buckets,
    netByPeriod: ni?.values ?? periods.map(() => ({ minorUnits: "0", currencyCode: currency })),
    netTotal: ni?.total ?? { minorUnits: "0", currencyCode: currency },
    revenueTotal,
    expensesTotal: {
      minorUnits: subMinorUnits(revenueTotal.minorUnits, (cm?.total ?? ZERO).minorUnits),
      currencyCode: currency,
    },
    expensesByPeriod: periods.map((_, i) => ({
      minorUnits: subMinorUnits(
        (revenue?.values[i] ?? ZERO).minorUnits,
        (cm?.values[i] ?? ZERO).minorUnits,
      ),
      currencyCode: currency,
    })),
    cash: cashByPeriod(periods, protoReconciliation, currency),
    reconciliation: aggregateReconciliation(protoReconciliation),
    provenance: provenanceFromProto(versions),
    unreviewedAmount: unclassified?.total ?? { minorUnits: "0", currencyCode: currency },
  };
}

/**
 * The report for a period range. Basis is derived, never chosen (design
 * §2.5); granularity is fixed at monthly, the only one any screen offers.
 */
export async function getReport(params: { from: Period; to: Period }): Promise<Report> {
  const { orgId, entityId } = requireSession();
  const basis = await deriveBasis();
  const res = await client.getManagementPNL({
    organizationId: orgId,
    entityId,
    from: params.from,
    to: params.to,
    granularity: ReportGranularity.MONTH,
    basis,
  });
  return reportFromProto(
    basisFromProto(res.basis),
    res.periods,
    res.lines,
    res.buckets,
    res.reconciliation,
    res.versions,
  );
}

/* --------------------------------------------------------------------------
 * Drill-down: the transactions -- or, for a computed line, the operands --
 * behind one figure.
 * ----------------------------------------------------------------------- */

export interface DrilldownRow {
  id: string;
  bookedOn: string;
  description: string;
  amount: Money;
  categoryLabel: string;
  layer: EngineLayer;
  /** Absent on an unclassified row -- the bucket drill-downs reach this,
   *  same as any line's. */
  confidence?: number;
  /** D4 match evidence, where a bank row was matched to a ledger document. */
  evidence?: string;
}

function rowFromProto(t: ProtoDrillTransaction): DrilldownRow {
  return {
    id: t.id,
    bookedOn: t.bookedOn,
    description: t.description,
    amount: money(t.amount),
    categoryLabel: t.categoryName,
    layer: (t.engineLayer || "L0") as EngineLayer,
    confidence: t.confidence,
    evidence: t.evidence || undefined,
  };
}

/** One line a computed line is made of -- itself openable, in turn. */
export interface DrilldownOperand {
  categoryId: string;
  label: string;
  subtracted: boolean;
}

function operandFromProto(o: ProtoReportOperand): DrilldownOperand {
  return { categoryId: o.code, label: o.label, subtracted: o.subtracted };
}

export interface Drilldown {
  categoryId: string;
  categoryLabel: string;
  period: Period;
  amount: Money;
  /** "transactions" for a section or a bucket; "operands" for a computed
   *  line, which has none of its own -- GM is NET SALES minus CS, and both
   *  of those have transactions. */
  kind: "transactions" | "operands";
  rows: readonly DrilldownRow[];
  operands: readonly DrilldownOperand[];
  provenance: Provenance;
}

/**
 * Opens one cell: `categoryId` is a line's code ('01', '91') or a bucket's
 * name ('unclassified', 'non_pnl', 'unallocated', 'other_basis') -- the same
 * two closed, disjoint vocabularies `ListLineTransactionsRequest.line` takes.
 * `basis`/`granularity`/`from`/`to` have to match the report this figure came
 * from, or the rows returned are not the rows the figure was summed from.
 *
 * The label is resolved by reading the report this cell is part of, the same
 * way the fixture always did (`getReport` was already called a second time
 * here, before this change) -- a bucket's label is not looked up this way
 * (it comes from `i18n` in the component: a bucket has no human name on the
 * wire, only a kind), but a line's does, so the panel reads "NET SALES", not
 * "01".
 */
export async function getDrilldown(params: {
  categoryId: string;
  period: Period;
  from: Period;
  to: Period;
}): Promise<Drilldown> {
  const { orgId, entityId } = requireSession();
  const [basis, report] = await Promise.all([
    deriveBasis(),
    getReport({ from: params.from, to: params.to }),
  ]);
  const line = report.lines.find((l) => l.categoryId === params.categoryId);

  const res = await client.listLineTransactions({
    organizationId: orgId,
    entityId,
    basis,
    granularity: ReportGranularity.MONTH,
    from: params.from,
    to: params.to,
    period: params.period,
    line: params.categoryId,
    cursor: "",
    limit: 500,
  });

  return {
    categoryId: params.categoryId,
    categoryLabel: line?.label ?? params.categoryId,
    period: params.period,
    amount: money(res.total),
    kind: res.kind === ReportAnswerKind.OPERANDS ? "operands" : "transactions",
    rows: res.transactions.map(rowFromProto),
    operands: res.operands.map(operandFromProto),
    provenance: report.provenance,
  };
}

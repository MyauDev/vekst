/**
 * The review queue: transactions the engine could not classify confidently,
 * grouped by counterparty and sorted by amount then repeat count.
 *
 * Grouped because the decision is about the counterparty, not the row: a person
 * who decides that VEKTOR LOGISTIKA is Logistics has decided it for every row
 * that will ever bear that name, and approving them one at a time is the same
 * decision typed fifty times.
 */
import { sumMinorUnits } from "../money";
import type { EngineLayer, Money } from "./types";

export interface ReviewTransaction {
  id: string;
  bookedOn: string;
  description: string;
  amount: Money;
}

export interface ReviewGroup {
  counterpartyKey: string;
  counterpartyLabel: string;
  /** Summed from the rows, never stated. */
  total: Money;
  transactions: readonly ReviewTransaction[];
  suggestedCategoryId?: string;
  suggestedLayer?: EngineLayer;
  suggestedConfidence?: number;
}

const money = (minorUnits: string): Money => ({ minorUnits, currencyCode: "EUR" });

const RAW = [
  {
    counterpartyKey: "vektor-logistika", counterpartyLabel: "VEKTOR LOGISTIKA",
    suggestedCategoryId: "logistics", suggestedLayer: "L2" as EngineLayer, suggestedConfidence: 0.68,
    rows: [
      { id: "r1", bookedOn: "2026-08-04", description: "VEKTOR LOGISTIKA OPLATA SCHET 4602", units: "-142300" },
      { id: "r2", bookedOn: "2026-08-19", description: "VEKTOR LOGISTIKA OPLATA SCHET 4655", units: "-98400" },
    ],
  },
  {
    counterpartyKey: "kontur-service", counterpartyLabel: "KONTUR SERVICE",
    suggestedCategoryId: "software", suggestedLayer: "L2" as EngineLayer, suggestedConfidence: 0.55,
    rows: [
      { id: "r3", bookedOn: "2026-08-11", description: "KONTUR SERVICE PODPISKA", units: "-24900" },
    ],
  },
] as const;

export async function listReviewGroups(): Promise<readonly ReviewGroup[]> {
  return RAW.filter((g) => !decided.has(g.counterpartyKey)).map((g) => ({
    counterpartyKey: g.counterpartyKey,
    counterpartyLabel: g.counterpartyLabel,
    total: money(sumMinorUnits(g.rows.map((r) => r.units))),
    transactions: g.rows.map((r) => ({
      id: r.id, bookedOn: r.bookedOn, description: r.description, amount: money(r.units),
    })),
    suggestedCategoryId: g.suggestedCategoryId,
    suggestedLayer: g.suggestedLayer,
    suggestedConfidence: g.suggestedConfidence,
  }));
}

/** What the rail's badge counts, and what the report's headline states. */
export async function reviewSummary(): Promise<{ groups: number; rows: number; amount: Money }> {
  // Reads the same filtered list the screen does, so the badge cannot disagree
  // with the queue it counts.
  const groups = await listReviewGroups();
  return {
    groups: groups.length,
    rows: groups.reduce((n, g) => n + g.transactions.length, 0),
    amount: money(sumMinorUnits(groups.map((g) => g.total.minorUnits))),
  };
}

/**
 * The categories a reviewer can assign with a digit.
 *
 * Short on purpose: the queue is worked by keyboard, and a list longer than the
 * digits is a list that needs a second interaction to reach. The real taxonomy
 * arrives with change 3.1 and is a tree; what a reviewer needs in front of them
 * is the handful this counterparty is plausibly one of.
 */
export interface Category {
  id: string;
  label: string;
}

const CATEGORIES: readonly Category[] = [
  { id: "logistics", label: "Logistics" },
  { id: "materials", label: "Materials" },
  { id: "software", label: "Software and subscriptions" },
  { id: "professional-fees", label: "Professional fees" },
  { id: "rent-and-utilities", label: "Rent and utilities" },
  { id: "payroll", label: "Payroll" },
];

export async function listCategories(): Promise<readonly Category[]> {
  return CATEGORIES;
}

/** How a group leaves the queue. Codes, never sentences. */
export type ReviewDecision =
  | { kind: "classify"; categoryId: string }
  | { kind: "internal_transfer" }
  | { kind: "not_in_pnl" };

/**
 * Decisions taken this session. The RPC replaces the body, not the callers.
 *
 * This is the one thing a fixture layer has that a real one does not: mutable
 * state living for the lifetime of the module rather than in a database. It
 * leaks between anything sharing the module -- which is how five review tests
 * started failing the moment one of them approved a group.
 */
const decided = new Set<string>();

/**
 * Returns the fixtures to their initial state.
 *
 * Exists because the state above exists, and disappears with it when the RPC
 * lands. Tests need each case to start from the same queue; without this they
 * depend on the order they happen to run in, which is the kind of test that
 * passes alone and fails in a suite.
 */
export function resetFixtures() {
  decided.clear();
}

/**
 * Approving a *group* rather than a row is the whole design. A person who
 * decides VEKTOR LOGISTIKA is Logistics has decided it for every row that will
 * ever carry that name, and approving them one at a time is the same decision
 * typed fifty times. The real backend also writes vendor memory here, which is
 * what stops the next import asking again.
 */
export async function decideGroup(input: {
  counterpartyKey: string;
  decision: ReviewDecision;
}): Promise<void> {
  decided.add(input.counterpartyKey);
}

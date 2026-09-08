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
  return RAW.map((g) => ({
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
  const groups = await listReviewGroups();
  return {
    groups: groups.length,
    rows: groups.reduce((n, g) => n + g.transactions.length, 0),
    amount: money(sumMinorUnits(groups.map((g) => g.total.minorUnits))),
  };
}

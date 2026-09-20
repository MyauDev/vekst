/**
 * The review queue: transactions the engine could not classify confidently,
 * grouped by counterparty and sorted by amount then repeat count.
 *
 * Real from here on: `ListReviewGroups`, `ListGroupTransactions`,
 * `ResolveGroup`, `UndoDecision`, `ListCategories`.
 *
 * The fixture's `suggestedCategoryId`/`suggestedLayer`/`suggestedConfidence`
 * are gone rather than faked: neither `ReviewGroup` nor `QueuedTransaction`
 * carries an engine suggestion on the wire. A per-transaction proposal is not
 * part of this change's contract (non-goals), so a field that would always be
 * undefined is a worse interface than no field.
 *
 * `ReviewGroup` no longer carries its own `transactions`, because
 * `ListReviewGroups` does not return them -- only a group's own summary.
 * `listGroupTransactions(counterpartyKey)` is the separate, on-demand call
 * the real API actually offers, and `ReviewScreen` fetches it for whichever
 * group is currently open rather than for all of them eagerly.
 *
 * `ReviewDecision`'s classify variant carries `categoryCode`, not
 * `categoryId`: `ResolveGroupRequest`'s own comment is explicit about why --
 * "a code rather than an identifier: the client renders a taxonomy it was
 * given, and an id it could return is an id it could invent."
 *
 * `organizationId`, not `orgId`: unlike `import.proto`, `review.proto`'s own
 * requests spell the field `organization_id`. `requireSession()`'s own
 * `Session.orgId` is unaffected -- only the wire field name differs.
 */
import { createClient } from "@connectrpc/connect";

import { transport } from "../transport";
import { requireSession } from "./session";
import { ReviewService, ReviewOutcome } from "../gen/vekst/v1/review_pb";
import type {
  ReviewGroup as ProtoReviewGroup,
  QueuedTransaction as ProtoQueuedTransaction,
  Category as ProtoCategory,
} from "../gen/vekst/v1/review_pb";
import type { Money as ProtoMoney } from "../gen/vekst/type/v1/money_pb";
import type { Money } from "./types";

const client = createClient(ReviewService, transport);

/** The whole queue arrives on one page. Pagination is a later change's
 *  problem (non-goals); a queue this size is what the Demo's pilots carry. */
const PAGE_SIZE = 500;

function money(m: ProtoMoney | undefined): Money {
  return m ? { minorUnits: m.minorUnits, currencyCode: m.currencyCode } : { minorUnits: "0", currencyCode: "" };
}

export interface ReviewTransaction {
  id: string;
  bookedOn: string;
  description: string;
  amount: Money;
}

function transactionFromProto(t: ProtoQueuedTransaction): ReviewTransaction {
  return { id: t.id, bookedOn: t.bookedOn, description: t.description, amount: money(t.amount) };
}

export interface ReviewGroup {
  counterpartyKey: string;
  counterpartyLabel: string;
  rowCount: number;
  /** Summed server-side, never stated. */
  total: Money;
  firstSeen: string;
  lastSeen: string;
}

function groupFromProto(g: ProtoReviewGroup): ReviewGroup {
  return {
    counterpartyKey: g.counterpartyKey,
    counterpartyLabel: g.displayName,
    rowCount: g.rowCount,
    total: money(g.total),
    firstSeen: g.firstSeen,
    lastSeen: g.lastSeen,
  };
}

export async function listReviewGroups(): Promise<readonly ReviewGroup[]> {
  const { orgId, entityId } = requireSession();
  const res = await client.listReviewGroups({
    organizationId: orgId,
    entityId,
    limit: PAGE_SIZE,
    offset: 0,
  });
  return res.groups.map(groupFromProto);
}

export async function listGroupTransactions(counterpartyKey: string): Promise<readonly ReviewTransaction[]> {
  const { orgId, entityId } = requireSession();
  const res = await client.listGroupTransactions({ organizationId: orgId, entityId, counterpartyKey });
  return res.transactions.map(transactionFromProto);
}

/** What the rail's badge counts, and what the report's headline states. A
 *  minimal page: this reads the server's own totals, not a client-side sum
 *  over rows it otherwise has no use for. */
export async function reviewSummary(): Promise<{ groups: number; rows: number; amount: Money }> {
  const { orgId, entityId } = requireSession();
  const res = await client.listReviewGroups({ organizationId: orgId, entityId, limit: 1, offset: 0 });
  return {
    groups: res.totalCounterpartyCount,
    rows: res.totalRowCount,
    amount: money(res.totalAbsolute),
  };
}

/**
 * The categories a reviewer can assign with a digit.
 *
 * `label` here is `Category.name` on the wire: the fixture's field name is
 * kept because `ReviewScreen` reads it and nothing about the rename would
 * change what the screen shows, only what this module calls it internally.
 */
export interface Category {
  id: string;
  code: string;
  label: string;
}

function categoryFromProto(c: ProtoCategory): Category {
  return { id: c.id, code: c.code, label: c.name };
}

export async function listCategories(): Promise<readonly Category[]> {
  const { orgId } = requireSession();
  const res = await client.listCategories({ organizationId: orgId });
  return res.categories.map(categoryFromProto);
}

/** How a group leaves the queue. Codes, never sentences. */
export type ReviewDecision =
  | { kind: "classify"; categoryCode: string }
  | { kind: "internal_transfer" }
  | { kind: "not_in_pnl" };

function toProtoOutcome(d: ReviewDecision): ReviewOutcome {
  switch (d.kind) {
    case "classify":
      return ReviewOutcome.CATEGORISED;
    case "internal_transfer":
      return ReviewOutcome.INTERNAL_TRANSFER;
    case "not_in_pnl":
      return ReviewOutcome.NON_PNL;
  }
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
  const { orgId, entityId } = requireSession();
  await client.resolveGroup({
    organizationId: orgId,
    entityId,
    counterpartyKey: input.counterpartyKey,
    outcome: toProtoOutcome(input.decision),
    categoryCode: input.decision.kind === "classify" ? input.decision.categoryCode : "",
  });
}

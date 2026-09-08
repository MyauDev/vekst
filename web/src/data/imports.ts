/**
 * Import batches and their validation results.
 *
 * The counts on a batch -- rows imported, duplicates skipped, internal
 * transfers found, matches proposed -- are what `DESIGN.md` §2 lists among the
 * things a minimal pass may never remove, so they are part of the shape rather
 * than a detail a screen may drop.
 */
import type { Money, Period, SourceKind } from "./types";

export type BatchState = "pending" | "parsing" | "rejected" | "imported";

export interface BatchCounts {
  rowsImported: number;
  duplicatesSkipped: number;
  internalTransfersFound: number;
  matchesProposed: number;
}

export interface Batch {
  id: string;
  fileName: string;
  sourceKind: SourceKind;
  state: BatchState;
  uploadedAt: string;
  periodFrom?: Period;
  periodTo?: Period;
  counts?: BatchCounts;
  /** A code, not a sentence. The client turns it into one. */
  rejectionReason?: string;
}

/**
 * A validation error, keyed by the line number in the **original file**.
 *
 * Never the parsed row index. This is an invariant in `CLAUDE.md`, not a
 * preference: a person fixing the file opens it in a spreadsheet and goes to a
 * line, and a parsed index does not name any line they can see.
 */
export interface ValidationError {
  fileLine: number;
  code: string;
  detail?: string;
}

export interface BatchDetail extends Batch {
  errors: readonly ValidationError[];
  /** Opening + movements = closing, with zero tolerance. The one completeness
   *  check that decides whether a file can be trusted at all. */
  balanceCheck?: { opening: Money; movements: Money; closing: Money; declared: Money };
}

const money = (minorUnits: string): Money => ({ minorUnits, currencyCode: "EUR" });

const BATCHES: readonly BatchDetail[] = [
  {
    id: "b-2026-08-nordea", fileName: "nordea-2026-08.csv", sourceKind: "bank",
    state: "imported", uploadedAt: "2026-09-02T09:14:00Z",
    periodFrom: "2026-08", periodTo: "2026-08",
    counts: { rowsImported: 412, duplicatesSkipped: 9, internalTransfersFound: 4, matchesProposed: 0 },
    errors: [],
  },
  {
    id: "b-2026-07-nordea", fileName: "nordea-2026-07.csv", sourceKind: "bank",
    state: "imported", uploadedAt: "2026-08-03T08:41:00Z",
    periodFrom: "2026-07", periodTo: "2026-07",
    counts: { rowsImported: 388, duplicatesSkipped: 2, internalTransfersFound: 6, matchesProposed: 0 },
    errors: [],
  },
  {
    id: "b-ledger-h1", fileName: "ledger-h1-2026.xlsx", sourceKind: "ledger",
    state: "rejected", uploadedAt: "2026-08-28T16:02:00Z",
    rejectionReason: "balance_mismatch",
    // A rejected file persists nothing. The errors are the whole content of the
    // screen, and they are keyed by the line the reader will open.
    errors: [
      { fileLine: 3, code: "amount_unparseable", detail: "1 240,00 EUR" },
      { fileLine: 47, code: "date_implausible", detail: "31.02.2026" },
      { fileLine: 118, code: "currency_unknown", detail: "EURO" },
      { fileLine: 119, code: "debit_and_credit_both_set" },
      { fileLine: 204, code: "description_missing" },
    ],
    balanceCheck: {
      opening: money("15230000"), movements: money("8452785"),
      closing: money("23682785"), declared: money("23684000"),
    },
  },
];

/** Uploads made this session, newest first. Replaced wholesale by the RPC. */
const uploaded: BatchDetail[] = [];

export async function listBatches(): Promise<readonly Batch[]> {
  return [...uploaded, ...BATCHES];
}

/**
 * Accepts a file and its `source_kind`.
 *
 * The tag is a required argument rather than an optional one because it decides
 * the accounting basis and cannot be recovered from the file: a CSV of payments
 * looks identical whether it came from a bank or a ledger, and guessing wrong
 * makes every report built on it wrong in a way nothing downstream can detect.
 *
 * Nothing is parsed here. Ingest is a River job in `core` (changes 2.1–2.3), so
 * this returns the batch in the state a real upload leaves it in -- pending --
 * and the list shows it as such. That is the honest fake: the same shape and
 * the same state machine, without inventing a validation result.
 */
export async function uploadBatch(input: {
  file: File;
  sourceKind: SourceKind;
}): Promise<Batch> {
  const batch: BatchDetail = {
    id: `b-local-${Date.now()}`,
    fileName: input.file.name,
    sourceKind: input.sourceKind,
    state: "pending",
    uploadedAt: new Date().toISOString(),
    errors: [],
  };
  uploaded.unshift(batch);
  return batch;
}

export async function getBatch(id: string): Promise<BatchDetail | undefined> {
  return BATCHES.find((b) => b.id === id);
}

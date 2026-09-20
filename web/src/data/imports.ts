/**
 * Import batches and their validation results.
 *
 * Real from here on: `ListImportBatches` and `GetImportBatch`. A single
 * batch's detail composes three calls -- `GetImportBatch`,
 * `GetValidationReport`, `GetDedupSummary` -- into the one `BatchDetail`
 * shape this module has always exported, because that is three separate
 * tables in the real schema and one screen's worth of facts about a file.
 *
 * Two fixture-only fields had no backend behind them and are gone rather than
 * faked: `matchesProposed` (a D4 match-proposal count nothing computes yet)
 * and `periodFrom`/`periodTo` (no RPC returns a batch's period; only its
 * validation report's balance check does, and only when the file declared
 * one). Dropping them is a real shape change, on the same reasoning
 * `review.ts` drops its unsupported suggestion fields: a field that never
 * carries real data is a worse interface than no field.
 *
 * The eleven real `ImportStatus` values collapse onto the four-word batch
 * vocabulary `docs/DESIGN.md` §7 already fixes, rather than growing it: a
 * fifth chip colour or a tenth word is a bigger decision than this module
 * should take on its own (`ui/StateChip.tsx`'s own comment makes the same
 * call about a fourth tone). Before parsing starts is "pending"; anything
 * mid-pipeline is "parsing"; every way a batch ends badly -- rejected,
 * abandoned, failed -- is "rejected"; only a clean finish is "imported".
 */
import { createClient } from "@connectrpc/connect";
import { timestampDate } from "@bufbuild/protobuf/wkt";

import { transport } from "../transport";
import { requireSession } from "./session";
import { createImportBatch, putUpload, confirmImportUpload } from "./importUpload";
import { ImportService, ImportStatus, SourceKind as ProtoSourceKind } from "../gen/vekst/v1/import_pb";
import type {
  ImportBatch as ProtoImportBatch,
  ValidationError as ProtoValidationError,
  BalanceMismatchDetail as ProtoBalanceMismatchDetail,
} from "../gen/vekst/v1/import_pb";
import type { Money, SourceKind } from "./types";

const client = createClient(ImportService, transport);

export type BatchState = "pending" | "parsing" | "rejected" | "imported";

const STATE_BY_STATUS: Record<ImportStatus, BatchState> = {
  [ImportStatus.UNSPECIFIED]: "pending",
  [ImportStatus.AWAITING_UPLOAD]: "pending",
  [ImportStatus.UPLOADED]: "pending",
  [ImportStatus.PARSING]: "parsing",
  [ImportStatus.PARSED]: "parsing",
  [ImportStatus.VALIDATING]: "parsing",
  [ImportStatus.VALIDATED]: "parsing",
  [ImportStatus.PERSISTING]: "parsing",
  [ImportStatus.REJECTED]: "rejected",
  [ImportStatus.ABANDONED]: "rejected",
  [ImportStatus.FAILED]: "rejected",
  [ImportStatus.IMPORTED]: "imported",
};

function fromProtoSourceKind(k: ProtoSourceKind): SourceKind {
  return k === ProtoSourceKind.LEDGER ? "ledger" : "bank";
}

export interface BatchCounts {
  rowsImported: number;
  duplicatesSkipped: number;
  internalTransfersFound: number;
}

export interface Batch {
  id: string;
  fileName: string;
  sourceKind: SourceKind;
  state: BatchState;
  uploadedAt: string;
  /** A code, not a sentence. Populated for `state === "rejected"`. */
  rejectionReason?: string;
}

function batchFromProto(b: ProtoImportBatch): Batch {
  return {
    id: b.id,
    fileName: b.fileName,
    sourceKind: fromProtoSourceKind(b.sourceKind),
    state: STATE_BY_STATUS[b.status],
    uploadedAt: b.createdAt ? timestampDate(b.createdAt).toISOString() : new Date(0).toISOString(),
    rejectionReason: b.failureCode || undefined,
  };
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

function errorFromProto(e: ProtoValidationError): ValidationError {
  return { fileLine: e.line, code: e.code, detail: e.raw || undefined };
}

export interface BatchDetail extends Batch {
  errors: readonly ValidationError[];
  counts?: BatchCounts;
  /** Opening + movements = closing, with zero tolerance. The one completeness
   *  check that decides whether a file can be trusted at all. */
  balanceCheck?: { opening: Money; movements: Money; closing: Money; declared: Money };
}

/**
 * `closing` is computed (opening + movements), never the field the same name
 * suggests reading off the wire: `BalanceMismatchDetail.closing` is what the
 * statement itself declared, which is `declared` here -- the same distinction
 * the fixture always drew, now sourced from two different places in one
 * message instead of typed in twice.
 */
function balanceCheckFromProto(bm: ProtoBalanceMismatchDetail) {
  const money = (minorUnits: bigint): Money => ({ minorUnits: minorUnits.toString(), currencyCode: bm.currency });
  return {
    opening: money(bm.opening),
    movements: money(bm.movements),
    closing: money(bm.opening + bm.movements),
    declared: money(bm.closing),
  };
}

export async function listBatches(): Promise<readonly Batch[]> {
  const { orgId, entityId } = requireSession();
  const res = await client.listImportBatches({ orgId, entityId });
  return res.batches.map(batchFromProto);
}

/**
 * Accepts a file and its `source_kind`, and reserves the upload.
 *
 * The tag is a required argument rather than an optional one because it
 * decides the accounting basis and cannot be recovered from the file. This
 * delegates the actual reservation and PUT to `importUpload.ts`
 * (`add-file-upload`, already real) rather than duplicating them, and returns
 * once the object is confirmed uploaded -- parsing and validation run
 * asynchronously after that, the same way `getBatch`'s caller polls for them.
 */
export async function uploadBatch(input: { file: File; sourceKind: SourceKind }): Promise<Batch> {
  const { orgId, entityId } = requireSession();
  const reserved = await createImportBatch({
    orgId,
    entityId,
    sourceKind: input.sourceKind,
    fileName: input.file.name,
    declaredBytes: input.file.size,
    declaredType: input.file.type || "application/octet-stream",
  });
  await putUpload(reserved, input.file);
  await confirmImportUpload({ orgId, batchId: reserved.batchId });
  const detail = await getBatch(reserved.batchId);
  if (!detail) throw new Error("uploadBatch: batch vanished immediately after confirming its upload");
  return detail;
}

export async function getBatch(id: string): Promise<BatchDetail | undefined> {
  const { orgId } = requireSession();

  const batchRes = await client.getImportBatch({ orgId, batchId: id });
  if (!batchRes.batch) return undefined;
  const batch = batchFromProto(batchRes.batch);

  // Absent until the batch has reached at least `validated`: a batch still
  // parsing has no report to read yet, and that is a fact about timing, not a
  // failure to fetch one.
  const [validation, dedup] = await Promise.all([
    client.getValidationReport({ orgId, batchId: id }).catch(() => undefined),
    client.getDedupSummary({ orgId, batchId: id }).catch(() => undefined),
  ]);

  const report = validation?.report;
  const balanceMismatch = report?.warnings.find((w) => w.balanceMismatch)?.balanceMismatch;
  const summary = dedup?.summary;

  return {
    ...batch,
    errors: report ? report.errors.map(errorFromProto) : [],
    counts: summary
      ? {
          rowsImported: summary.importedRows,
          duplicatesSkipped: summary.skippedInBatch + summary.skippedCrossBatch,
          internalTransfersFound: summary.internalTransfers,
        }
      : undefined,
    balanceCheck: balanceMismatch ? balanceCheckFromProto(balanceMismatch) : undefined,
  };
}

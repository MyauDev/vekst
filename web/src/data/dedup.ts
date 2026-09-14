/**
 * The four RPCs add-dedup adds: a batch's dedup summary and skipped-row
 * list, and an entity's internal transfers plus dismissing one.
 *
 * No money field crosses this module at all (design's own proto comment):
 * a skipped row's amount is read from the transaction it matched, through
 * that transaction's own Money, never carried a second time here.
 */
import { createClient } from "@connectrpc/connect";
import { timestampDate } from "@bufbuild/protobuf/wkt";

import { transport } from "../transport";
import { ImportService } from "../gen/vekst/v1/import_pb";
import type {
  DedupSummary as ProtoDedupSummary,
  SkippedRow as ProtoSkippedRow,
  InternalTransfer as ProtoInternalTransfer,
} from "../gen/vekst/v1/import_pb";

const client = createClient(ImportService, transport);

export interface DedupSummaryView {
  importedRows: number;
  skippedInBatch: number;
  skippedCrossBatch: number;
  internalTransfers: number;
}

function summaryFromProto(s: ProtoDedupSummary): DedupSummaryView {
  return {
    importedRows: s.importedRows,
    skippedInBatch: s.skippedInBatch,
    skippedCrossBatch: s.skippedCrossBatch,
    internalTransfers: s.internalTransfers,
  };
}

export async function getDedupSummary(input: { orgId: string; batchId: string }): Promise<DedupSummaryView> {
  const res = await client.getDedupSummary({ orgId: input.orgId, batchId: input.batchId });
  if (!res.summary) {
    throw new Error("getDedupSummary: response carried no summary");
  }
  return summaryFromProto(res.summary);
}

export interface SkippedRowView {
  lineNo: number;
  postingNo: number;
  /** D2 | D3 */
  level: string;
  dedupHash: string;
  /** Empty for D2, where the original is another line of this same file. */
  matchedTransactionId: string;
  matchedBatchId: string;
  createdAt: Date;
}

function skippedRowFromProto(r: ProtoSkippedRow): SkippedRowView {
  return {
    lineNo: r.lineNo,
    postingNo: r.postingNo,
    level: r.level,
    dedupHash: r.dedupHash,
    matchedTransactionId: r.matchedTransactionId,
    matchedBatchId: r.matchedBatchId,
    createdAt: r.createdAt ? timestampDate(r.createdAt) : new Date(0),
  };
}

export async function listSkippedRows(input: { orgId: string; batchId: string }): Promise<SkippedRowView[]> {
  const res = await client.listSkippedRows({ orgId: input.orgId, batchId: input.batchId });
  return res.rows.map(skippedRowFromProto);
}

export interface InternalTransferView {
  id: string;
  outTransactionId: string;
  inTransactionId: string;
  detectedAt: Date;
  /** Empty when the pair is still active (excluded from the P&L). */
  dismissedByUserId: string;
  dismissedAt?: Date;
}

function transferFromProto(t: ProtoInternalTransfer): InternalTransferView {
  return {
    id: t.id,
    outTransactionId: t.outTransactionId,
    inTransactionId: t.inTransactionId,
    detectedAt: t.detectedAt ? timestampDate(t.detectedAt) : new Date(0),
    dismissedByUserId: t.dismissedByUserId,
    dismissedAt: t.dismissedAt ? timestampDate(t.dismissedAt) : undefined,
  };
}

export async function listInternalTransfers(input: {
  orgId: string;
  entityId: string;
}): Promise<InternalTransferView[]> {
  const res = await client.listInternalTransfers({ orgId: input.orgId, entityId: input.entityId });
  return res.transfers.map(transferFromProto);
}

/**
 * Excluded from the P&L on detection, reversible by dismissal (design D4,
 * confirmed with the founder) -- dismissing returns a pair to the P&L.
 */
export async function dismissInternalTransfer(input: {
  orgId: string;
  transferId: string;
}): Promise<InternalTransferView> {
  const res = await client.dismissInternalTransfer({ orgId: input.orgId, transferId: input.transferId });
  if (!res.transfer) {
    throw new Error("dismissInternalTransfer: response carried no transfer");
  }
  return transferFromProto(res.transfer);
}

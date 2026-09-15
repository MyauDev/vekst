/**
 * The upload handshake: the two RPCs `add-file-upload` adds, plus the raw PUT
 * that actually carries the bytes.
 *
 * core never receives the file (design D1). `createImportBatch` returns a
 * presigned PUT; the browser sends the file straight to the object store with
 * `putUpload`; `confirmImportUpload` tells core to go measure what arrived.
 * Nothing here parses or validates anything -- that is 2.2 and 2.3 -- and
 * nothing here renders anything: the Imports screen that drives this flow
 * belongs to `add-web-experience` §6.
 */
import { createClient } from "@connectrpc/connect";

import { transport } from "../transport";
import { ImportService, SourceKind as ProtoSourceKind } from "../gen/vekst/v1/import_pb";
import type { SourceKind } from "./types";

const client = createClient(ImportService, transport);

function toProtoSourceKind(kind: SourceKind): ProtoSourceKind {
  return kind === "ledger" ? ProtoSourceKind.LEDGER : ProtoSourceKind.BANK;
}

export interface ReservedUpload {
  batchId: string;
  uploadUrl: string;
  /**
   * Headers to send with the PUT, verbatim. Never includes Content-Length --
   * the store's signature does bind one, but a browser computes that header
   * itself from the request body and refuses to let script set it (it is on
   * the Fetch spec's forbidden-header list), so there is nothing for a caller
   * to do with it here.
   */
  uploadHeaders: Record<string, string>;
}

/**
 * Reserves a batch and returns where to PUT the file.
 *
 * declaredBytes and declaredType are signing conditions, not facts (design
 * D2): core re-measures the object itself once the upload is confirmed, and
 * nothing downstream ever reads the numbers passed in here.
 */
export async function createImportBatch(input: {
  orgId: string;
  entityId: string;
  sourceKind: SourceKind;
  fileName: string;
  declaredBytes: number;
  declaredType: string;
}): Promise<ReservedUpload> {
  const res = await client.createImportBatch({
    orgId: input.orgId,
    entityId: input.entityId,
    sourceKind: toProtoSourceKind(input.sourceKind),
    fileName: input.fileName,
    declaredBytes: BigInt(input.declaredBytes),
    declaredType: input.declaredType,
  });
  if (!res.batch) {
    throw new Error("createImportBatch: response carried no batch");
  }
  return {
    batchId: res.batch.id,
    uploadUrl: res.uploadUrl,
    uploadHeaders: res.uploadHeaders,
  };
}

/**
 * The browser's half of the handshake: PUT the file to the object store with
 * exactly the signed headers. A mismatched length or type is rejected by the
 * store itself (403), never observed here as anything but a failed fetch.
 */
export async function putUpload(upload: ReservedUpload, file: File): Promise<void> {
  const res = await fetch(upload.uploadUrl, {
    method: "PUT",
    headers: upload.uploadHeaders,
    body: file,
  });
  if (!res.ok) {
    throw new Error(`upload to the object store failed: ${res.status}`);
  }
}

/**
 * Tells core the PUT finished. This call itself records nothing: it enqueues
 * the job that measures the object core actually received, because nothing
 * the client says about its own upload is a fact (design D2).
 */
export async function confirmImportUpload(input: { orgId: string; batchId: string }): Promise<void> {
  await client.confirmImportUpload({ orgId: input.orgId, batchId: input.batchId });
}

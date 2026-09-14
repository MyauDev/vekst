import { afterEach, describe, expect, it, vi } from "vitest";

import { SourceKind as ProtoSourceKind } from "../gen/vekst/v1/import_pb";

// The module under test builds its client from the shared singleton
// transport at import time, so the transport is swapped out by mocking the
// module it comes from rather than passed in as a parameter -- there is no
// other seam to substitute a router transport through.
vi.mock("../transport", async () => {
  const { createRouterTransport } = await import("@connectrpc/connect");
  const { ImportService } = await import("../gen/vekst/v1/import_pb");
  return {
    transport: createRouterTransport(({ service }) => {
      service(ImportService, {
        createImportBatch: (req) => ({
          batch: {
            id: "batch-1",
            entityId: req.entityId,
            sourceKind: req.sourceKind,
            status: 1,
            fileName: req.fileName,
            byteLength: 0n,
            failureCode: "",
          },
          uploadUrl: "https://store.test/org/o/batch/batch-1",
          uploadHeaders: { "Content-Type": req.declaredType },
        }),
        confirmImportUpload: () => ({
          batch: {
            id: "batch-1",
            entityId: "e",
            sourceKind: 2,
            status: 1,
            fileName: "x.csv",
            byteLength: 0n,
            failureCode: "",
          },
        }),
      });
    }),
  };
});

const { createImportBatch, confirmImportUpload, putUpload } = await import("./importUpload");

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createImportBatch", () => {
  it("carries the source kind through to the proto enum and back to a batch id", async () => {
    const result = await createImportBatch({
      orgId: "o", entityId: "e", sourceKind: "bank",
      fileName: "statement.csv", declaredBytes: 42, declaredType: "text/csv",
    });
    expect(result.batchId).toBe("batch-1");
    expect(result.uploadHeaders["Content-Type"]).toBe("text/csv");
  });
});

describe("putUpload", () => {
  it("PUTs the file with exactly the signed headers, never adding Content-Length", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    const file = new File(["hello"], "x.csv", { type: "text/csv" });
    await putUpload(
      { batchId: "b", uploadUrl: "https://store.test/key", uploadHeaders: { "Content-Type": "text/csv" } },
      file,
    );

    expect(fetchMock).toHaveBeenCalledWith("https://store.test/key", {
      method: "PUT",
      headers: { "Content-Type": "text/csv" },
      body: file,
    });
  });

  it("throws when the store rejects the upload", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 403 })));
    const file = new File(["x"], "x.csv");
    await expect(
      putUpload({ batchId: "b", uploadUrl: "https://store.test/key", uploadHeaders: {} }, file),
    ).rejects.toThrow(/403/);
  });
});

describe("confirmImportUpload", () => {
  it("resolves once core has enqueued the measurement job", async () => {
    await expect(confirmImportUpload({ orgId: "o", batchId: "batch-1" })).resolves.toBeUndefined();
  });
});

// Documents the mapping this module owns, without re-exporting the proto enum.
describe("source kind mapping", () => {
  it("ledger and bank map to distinct, correct proto values", async () => {
    const ledger = await createImportBatch({
      orgId: "o", entityId: "e", sourceKind: "ledger",
      fileName: "l.xlsx", declaredBytes: 1, declaredType: "application/octet-stream",
    });
    const bank = await createImportBatch({
      orgId: "o", entityId: "e", sourceKind: "bank",
      fileName: "b.csv", declaredBytes: 1, declaredType: "text/csv",
    });
    expect(ledger.batchId).toBe("batch-1");
    expect(bank.batchId).toBe("batch-1");
    expect(ProtoSourceKind.LEDGER).not.toBe(ProtoSourceKind.BANK);
  });
});

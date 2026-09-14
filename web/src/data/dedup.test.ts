import { describe, expect, it, vi } from "vitest";

vi.mock("../transport", async () => {
  const { createRouterTransport } = await import("@connectrpc/connect");
  const { ImportService } = await import("../gen/vekst/v1/import_pb");
  const { timestampFromDate } = await import("@bufbuild/protobuf/wkt");

  return {
    transport: createRouterTransport(({ service }) => {
      service(ImportService, {
        getDedupSummary: () => ({
          summary: { importedRows: 10, skippedInBatch: 0, skippedCrossBatch: 2, internalTransfers: 1 },
        }),
        listSkippedRows: () => ({
          rows: [
            {
              lineNo: 7,
              postingNo: 0,
              level: "D3",
              dedupHash: "abc123",
              matchedTransactionId: "t1",
              matchedBatchId: "b0",
              createdAt: timestampFromDate(new Date("2026-09-13T12:00:00Z")),
            },
          ],
        }),
        listInternalTransfers: () => ({
          transfers: [
            {
              id: "tr1",
              outTransactionId: "t1",
              inTransactionId: "t2",
              detectedAt: timestampFromDate(new Date("2026-09-13T12:00:00Z")),
              dismissedByUserId: "",
            },
          ],
        }),
        dismissInternalTransfer: (req) => ({
          transfer: {
            id: req.transferId,
            outTransactionId: "t1",
            inTransactionId: "t2",
            detectedAt: timestampFromDate(new Date("2026-09-13T12:00:00Z")),
            dismissedByUserId: "u1",
            dismissedAt: timestampFromDate(new Date("2026-09-14T09:00:00Z")),
          },
        }),
      });
    }),
  };
});

const { getDedupSummary, listSkippedRows, listInternalTransfers, dismissInternalTransfer } = await import(
  "./dedup"
);

describe("getDedupSummary", () => {
  it("maps every count", async () => {
    const summary = await getDedupSummary({ orgId: "o", batchId: "b1" });
    expect(summary).toEqual({ importedRows: 10, skippedInBatch: 0, skippedCrossBatch: 2, internalTransfers: 1 });
  });
});

describe("listSkippedRows", () => {
  it("carries the matched transaction and a real date", async () => {
    const rows = await listSkippedRows({ orgId: "o", batchId: "b1" });
    expect(rows).toHaveLength(1);
    expect(rows[0]?.level).toBe("D3");
    expect(rows[0]?.matchedTransactionId).toBe("t1");
    expect(rows[0]?.createdAt).toBeInstanceOf(Date);
  });
});

describe("listInternalTransfers", () => {
  it("leaves an active pair's dismissedAt undefined", async () => {
    const transfers = await listInternalTransfers({ orgId: "o", entityId: "e1" });
    expect(transfers).toHaveLength(1);
    expect(transfers[0]?.dismissedByUserId).toBe("");
    expect(transfers[0]?.dismissedAt).toBeUndefined();
  });
});

describe("dismissInternalTransfer", () => {
  it("carries the dismissal back with a real date", async () => {
    const transfer = await dismissInternalTransfer({ orgId: "o", transferId: "tr1" });
    expect(transfer.dismissedByUserId).toBe("u1");
    expect(transfer.dismissedAt).toBeInstanceOf(Date);
  });
});

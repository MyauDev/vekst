import { describe, expect, it, vi } from "vitest";

vi.mock("../transport", async () => {
  const { createRouterTransport } = await import("@connectrpc/connect");
  const { ImportService, ImportStatus, SourceKind, ValidationOutcome } = await import(
    "../gen/vekst/v1/import_pb"
  );
  const { timestampFromDate } = await import("@bufbuild/protobuf/wkt");

  return {
    transport: createRouterTransport(({ service }) => {
      service(ImportService, {
        getImportBatch: () => ({
          batch: {
            id: "b1", entityId: "e1", sourceKind: SourceKind.BANK,
            status: ImportStatus.REJECTED, fileName: "statement.csv",
            byteLength: 2048n, failureCode: "balance_mismatch",
            createdAt: timestampFromDate(new Date("2026-09-01T00:00:00Z")),
          },
        }),
        getValidationReport: () => ({
          report: {
            batchId: "b1", outcome: ValidationOutcome.REJECTED,
            rowCount: 10, errorCount: 0, warningCount: 1,
            errors: [],
            warnings: [
              {
                code: "balance_mismatch",
                balanceMismatch: {
                  // BYN: exponent 2, and deliberately not the fixture's EUR --
                  // a module that hard-codes an exponent or a currency string
                  // would still pass a EUR-only test. opening + movements
                  // (1500.00) deliberately differs from the wire's own
                  // "closing" (1515.00, what the statement declared) -- a
                  // real mismatch, and the reason this test can tell the two
                  // apart rather than have them coincide.
                  opening: 100000n, movements: 50000n,
                  closing: 151500n, difference: 1500n, currency: "BYN",
                },
              },
            ],
            overriddenByUserId: "", overrideReason: "",
          },
        }),
        getDedupSummary: () => {
          throw new Error("not found");
        },
      });
    }),
  };
});

const { getBatch } = await import("./imports");
const { setSession } = await import("./session");

setSession({ orgId: "o1", entityId: "e1", baseCurrency: "BYN" });

describe("getBatch's balance check", () => {
  it("survives a non-base-currency amount round trip, as a string", async () => {
    const detail = await getBatch("b1");
    const check = detail?.balanceCheck;
    expect(check).toBeDefined();

    for (const leg of [check!.opening, check!.movements, check!.closing, check!.declared]) {
      expect(leg.currencyCode).toBe("BYN");
      // A minor-unit field is never a JavaScript number: it must survive
      // round-tripping through this module as a string, exactly, with no
      // precision lost or reintroduced by a float in between.
      expect(typeof leg.minorUnits).toBe("string");
    }

    expect(check!.opening.minorUnits).toBe("100000");
    expect(check!.movements.minorUnits).toBe("50000");
    // closing is computed (opening + movements), never read off the wire's
    // own "closing" field -- that field is what the statement declared,
    // which is `declared` here, and the two differ by exactly the mismatch
    // this warning exists to report.
    expect(check!.closing.minorUnits).toBe("150000");
    expect(check!.declared.minorUnits).toBe("151500");
  });
});

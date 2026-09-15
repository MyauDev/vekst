import { describe, expect, it, vi } from "vitest";

vi.mock("../transport", async () => {
  const { createRouterTransport } = await import("@connectrpc/connect");
  const { ImportService, ValidationOutcome } = await import("../gen/vekst/v1/import_pb");
  const { timestampFromDate } = await import("@bufbuild/protobuf/wkt");

  return {
    transport: createRouterTransport(({ service }) => {
      service(ImportService, {
        getValidationReport: () => ({
          report: {
            batchId: "b1",
            outcome: ValidationOutcome.VALID_WITH_WARNINGS,
            rowCount: 10,
            errorCount: 0,
            warningCount: 1,
            balanceCheckPassed: false,
            errors: [],
            warnings: [{
              code: "balance_mismatch",
              balanceMismatch: { opening: 1000n, movements: 500n, closing: 1501n, difference: -1n, currency: "EUR" },
            }],
            overriddenByUserId: "",
            overrideReason: "",
          },
        }),
        overrideValidation: (req) => ({
          report: {
            batchId: req.batchId,
            outcome: ValidationOutcome.VALID_WITH_WARNINGS,
            rowCount: 10,
            errorCount: 0,
            warningCount: 1,
            errors: [],
            warnings: [],
            overriddenByUserId: "u1",
            overrideReason: req.reason,
            overriddenAt: timestampFromDate(new Date("2026-09-13T12:00:00Z")),
          },
        }),
      });
    }),
  };
});

const { getValidationReport, overrideValidation } = await import("./validation");

describe("getValidationReport", () => {
  it("maps the proto outcome and balance mismatch detail", async () => {
    const report = await getValidationReport({ orgId: "o", batchId: "b1" });
    expect(report.outcome).toBe("valid_with_warnings");
    expect(report.balanceCheckPassed).toBe(false);
    expect(report.warnings).toHaveLength(1);
    expect(report.warnings[0]?.code).toBe("balance_mismatch");
    expect(report.warnings[0]?.balanceMismatch?.difference).toBe(-1n);
  });
});

describe("overrideValidation", () => {
  it("carries the override back with a real date", async () => {
    const report = await overrideValidation({ orgId: "o", batchId: "b1", reason: "confirmed by phone" });
    expect(report.overriddenByUserId).toBe("u1");
    expect(report.overrideReason).toBe("confirmed by phone");
    expect(report.overriddenAt).toBeInstanceOf(Date);
  });
});

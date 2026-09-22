import { describe, expect, it, vi } from "vitest";

vi.mock("../transport", async () => {
  const { createRouterTransport } = await import("@connectrpc/connect");
  const { ImportService, ImportStatus, SourceKind } = await import("../gen/vekst/v1/import_pb");
  const {
    ReportService, ReportBasis, ReportGranularity, ReportBucketKind, ReportAnswerKind,
  } = await import("../gen/vekst/v1/report_pb");
  const { timestampFromDate } = await import("@bufbuild/protobuf/wkt");

  // KWD: exponent 3, deliberately not the fixture's EUR -- a module that
  // hard-coded an exponent or a currency string would still pass a
  // EUR-only test.
  const m = (minor: string) => ({ minorUnits: minor, currencyCode: "KWD" });

  return {
    transport: createRouterTransport(({ service }) => {
      service(ImportService, {
        listImportBatches: () => ({
          batches: [
            {
              id: "b1", entityId: "e1", sourceKind: SourceKind.BANK,
              status: ImportStatus.IMPORTED, fileName: "statement.csv",
              byteLength: 1024n, failureCode: "",
              createdAt: timestampFromDate(new Date("2026-08-01T00:00:00Z")),
            },
          ],
        }),
      });
      service(ReportService, {
        getManagementPNL: () => ({
          basis: ReportBasis.BANK,
          granularity: ReportGranularity.MONTH,
          from: "2026-01", to: "2026-01",
          baseCurrency: "KWD",
          periods: ["2026-01"],
          lines: [
            {
              code: "01", label: "NET SALES", formula: "", computed: false,
              byPeriod: [{ amount: m("500000"), percentOfRevenue: 1 }],
              total: { amount: m("500000"), percentOfRevenue: 1 },
            },
            {
              code: "95", label: "NI", formula: "IBT - CIT", computed: true,
              byPeriod: [{ amount: m("120000"), percentOfRevenue: 0.24 }],
              total: { amount: m("120000"), percentOfRevenue: 0.24 },
            },
            {
              code: "93", label: "CM", formula: "NM - OPEX - OIE", computed: true,
              byPeriod: [{ amount: m("150000"), percentOfRevenue: 0.3 }],
              total: { amount: m("150000"), percentOfRevenue: 0.3 },
            },
          ],
          buckets: [
            { kind: ReportBucketKind.UNCLASSIFIED, byPeriod: [m("7000")], total: m("7000") },
          ],
          versions: { taxonomy: ["v1"], ruleset: ["v1"], engine: ["0.1.0"], normalize: ["v1"] },
          reconciliation: [
            {
              period: "2026-01",
              opening: m("1000000"), in: m("500000"), out: m("300000"), transfers: m("0"),
              transferRowCount: 0, closing: m("1200000"), balances: true, derived: true,
            },
          ],
        }),
        listLineTransactions: () => ({
          kind: ReportAnswerKind.TRANSACTIONS,
          transactions: [],
          operands: [],
          rowCount: 0,
          total: m("0"),
          nextCursor: "",
        }),
      });
    }),
  };
});

const { getReport } = await import("./report");
const { setSession } = await import("./session");

setSession({ orgId: "o1", entityId: "e1", baseCurrency: "KWD" });

describe("getReport", () => {
  it("survives a non-base-currency amount round trip, as a string", async () => {
    const report = await getReport({ from: "2026-01", to: "2026-01" });

    for (const money of [
      report.revenueTotal, report.netTotal, report.expensesTotal,
      report.reconciliation.opening, report.reconciliation.closing,
      report.unreviewedAmount,
    ]) {
      expect(money.currencyCode).toBe("KWD");
      expect(typeof money.minorUnits).toBe("string");
    }
  });

  it("derives revenueTotal, netTotal and expensesTotal from the real lines, not typed in", async () => {
    const report = await getReport({ from: "2026-01", to: "2026-01" });

    // revenueTotal is NET SALES (01), read directly.
    expect(report.revenueTotal.minorUnits).toBe("500000");
    // netTotal is NI (95), read directly.
    expect(report.netTotal.minorUnits).toBe("120000");
    // expensesTotal is revenueTotal - CM (93): 500000 - 150000.
    expect(report.expensesTotal.minorUnits).toBe("350000");
    // unreviewedAmount is the unclassified bucket's total.
    expect(report.unreviewedAmount.minorUnits).toBe("7000");
  });

  it("derives the reconciliation strip's closing figure, never reads the wire's own field", async () => {
    const report = await getReport({ from: "2026-01", to: "2026-01" });
    const r = report.reconciliation;
    // moneyOut and transfers are already negated for display (matching
    // every other outflow in this product), so reconstructing is a straight
    // sum of the four parts, the same shape the fixture always used -- not
    // the wire's own "1200000", which this test deliberately recomputes
    // rather than trusts.
    expect(r.closing.minorUnits).toBe(
      (BigInt(r.opening.minorUnits) + BigInt(r.moneyIn.minorUnits) + BigInt(r.moneyOut.minorUnits) + BigInt(r.transfers.minorUnits)).toString(),
    );
    expect(r.moneyOut.minorUnits).toBe("-300000"); // negated from the wire's positive magnitude
    expect(r.closing.minorUnits).toBe("1200000");
  });
});

import { describe, expect, it, vi } from "vitest";

vi.mock("../transport", async () => {
  const { createRouterTransport } = await import("@connectrpc/connect");
  const { ReviewService } = await import("../gen/vekst/v1/review_pb");

  return {
    transport: createRouterTransport(({ service }) => {
      service(ReviewService, {
        listReviewGroups: () => ({
          groups: [
            {
              counterpartyKey: "k1", displayName: "A Counterparty", rowCount: 3,
              // KWD: exponent 3, deliberately not EUR -- a module that
              // hard-coded an exponent or a currency string would still pass
              // a EUR-only test.
              total: { minorUnits: "-1234567", currencyCode: "KWD" },
              firstSeen: "2026-08-01", lastSeen: "2026-08-20",
            },
          ],
          totalRowCount: 3,
          totalCounterpartyCount: 1,
          totalAbsolute: { minorUnits: "1234567", currencyCode: "KWD" },
        }),
      });
    }),
  };
});

const { listReviewGroups, reviewSummary } = await import("./review");
const { setSession } = await import("./session");

setSession({ orgId: "o1", entityId: "e1", baseCurrency: "KWD" });

describe("listReviewGroups", () => {
  it("survives a non-base-currency total round trip, as a string", async () => {
    const [group] = await listReviewGroups();
    expect(group).toBeDefined();
    expect(group!.total.currencyCode).toBe("KWD");
    expect(typeof group!.total.minorUnits).toBe("string");
    expect(group!.total.minorUnits).toBe("-1234567");
  });
});

describe("reviewSummary", () => {
  it("reads the server's own total, not a client-side sum", async () => {
    const summary = await reviewSummary();
    expect(summary.amount.currencyCode).toBe("KWD");
    expect(typeof summary.amount.minorUnits).toBe("string");
    expect(summary.amount.minorUnits).toBe("1234567");
    expect(summary.rows).toBe(3);
    expect(summary.groups).toBe(1);
  });
});

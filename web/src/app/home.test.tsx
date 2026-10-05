import { render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { describe, expect, it, vi } from "vitest";

// `imports.ts` imports `transport` statically from "../transport" and builds
// its Connect client from it at module scope -- the same convention
// `dedup.test.ts` mocks around, and `imports.test.tsx` mirrors for the same
// reason: a transport passed only to `makeRouter` never reaches a data
// module's own static import, so the two have to be the same mocked
// instance.
vi.mock("../transport", async () => {
  const { stubTransport, signedInUser } = await import("../testTransport");
  const { ImportService, ImportStatus, SourceKind } = await import("../gen/vekst/v1/import_pb");
  const { ReviewService } = await import("../gen/vekst/v1/review_pb");
  const { ReportService, ReportBasis } = await import("../gen/vekst/v1/report_pb");
  const { timestampFromDate } = await import("@bufbuild/protobuf/wkt");

  // Three batches, newest first -- HomeScreen reads data[0] as the latest.
  const batch = (id: string, fileName: string, createdAt: Date) => ({
    id, entityId: "e1", sourceKind: SourceKind.BANK, status: ImportStatus.IMPORTED,
    fileName, byteLength: 1024n, failureCode: "", createdAt: timestampFromDate(createdAt),
  });

  return {
    transport: stubTransport({
      user: signedInUser,
      extend: (router) => {
        router.service(ImportService, {
          listImportBatches: () => ({
            batches: [
              batch("b1", "nordea-2026-08.csv", new Date("2026-09-02T09:14:00Z")),
              batch("b2", "nordea-2026-07.csv", new Date("2026-08-03T08:41:00Z")),
              batch("b3", "ledger-h1-2026.xlsx", new Date("2026-08-28T16:02:00Z")),
            ],
          }),
        });
        // Two groups -- the same number the rail's own badge shows, because
        // both read ["reviewSummary"].
        router.service(ReviewService, {
          listReviewGroups: () => ({
            groups: [],
            totalRowCount: 3,
            totalCounterpartyCount: 2,
            totalAbsolute: { minorUnits: "0", currencyCode: "EUR" },
          }),
        });
        // A fixed net result regardless of the requested range -- the same
        // "ignores its range" shape the fixture always had for this card.
        router.service(ReportService, {
          getManagementPNL: (req) => ({
            basis: ReportBasis.BANK,
            granularity: req.granularity,
            from: req.from, to: req.to,
            baseCurrency: "EUR",
            periods: [],
            lines: [
              {
                code: "95", label: "NI", formula: "IBT - CIT", computed: true,
                byPeriod: [],
                total: { amount: { minorUnits: "8926125", currencyCode: "EUR" } },
              },
            ],
            buckets: [],
            versions: undefined,
            reconciliation: [],
          }),
        });
      },
    }),
  };
});

const { makeRouter } = await import("../router");
const { transport } = await import("../transport");
const { signedInUser } = await import("../testTransport");
const { t } = await import("../i18n");

function renderAt(path: string) {
  const router = makeRouter(transport, createMemoryHistory({ initialEntries: [path] }));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

describe("Home", () => {
  it("is where /app lands", async () => {
    const router = renderAt("/app");
    await screen.findByRole("heading", { name: t("home.title") });
    expect(router.state.location.pathname).toBe("/app/home");
  });

  it("shows the real batch count and the latest batch, not a placeholder", async () => {
    renderAt("/app/home");
    // Three batches in the stub; asserting the exact figure is what keeps
    // this test from passing whether or not the card is reading real data.
    expect(await screen.findByText("3")).toBeDefined();
    expect(screen.getByText("nordea-2026-08.csv")).toBeDefined();
  });

  it("shows the real awaiting-review count, the same one the rail badges", async () => {
    renderAt("/app/home");
    await screen.findByRole("heading", { name: t("home.title") });
    expect(await screen.findByText(t("home.review.awaiting"))).toBeDefined();
    const card = screen.getByText(t("home.review.awaiting")).closest("a");
    expect(card?.textContent).toContain("2");
  });

  it("shows the real net result, computed the same way the report itself computes it", async () => {
    renderAt("/app/home");
    // The stub's getManagementPNL ignores the requested range and always
    // returns the same NI total, so this figure is deterministic regardless
    // of the current date.
    expect(await screen.findByText("89,261.25")).toBeDefined();
  });

  it("links each card to its own destination", async () => {
    renderAt("/app/home");
    await screen.findByRole("heading", { name: t("home.title") });

    // Scoped to the content area: the rail carries its own "Imports"/"Review"
    // links with the identical accessible name, and an unscoped query matches
    // both it and the card.
    const main = screen.getByRole("main");
    const hrefFor = (name: string) =>
      within(main).getByRole("link", { name: new RegExp(name) }).getAttribute("href");
    expect(hrefFor(t("nav.imports"))).toBe("/app/imports");
    expect(hrefFor(t("nav.review"))).toBe("/app/review");
  });

  it("shows the signed-in identity and a way to sign out", async () => {
    renderAt("/app/home");
    await screen.findByRole("heading", { name: t("home.title") });

    // Scoped to the content area: the rail's own account slot (AppLayout)
    // renders the identical email and sign-out button.
    const main = screen.getByRole("main");
    expect(within(main).getByText(signedInUser.email)).toBeDefined();
    expect(within(main).getByRole("button", { name: t("signedIn.signOut") })).toBeDefined();
  });
});

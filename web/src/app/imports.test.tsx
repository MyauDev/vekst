import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { describe, expect, it, vi } from "vitest";

// `imports.ts` (like every real data module) imports `transport` statically
// from "../transport" and builds its Connect client from it at module scope
// -- the same convention `dedup.test.ts` mocks around. Rendering a whole
// screen through the router additionally needs Health and Identity answered,
// which is what `stubTransport` already does -- so this mocks "../transport"
// itself to be a `stubTransport` instance carrying an ImportService stub too,
// rather than building a second, disconnected transport for the router alone.
vi.mock("../transport", async () => {
  const { stubTransport, signedInUser } = await import("../testTransport");
  const { ImportService, ImportStatus, SourceKind, ValidationOutcome } = await import(
    "../gen/vekst/v1/import_pb"
  );
  const { timestampFromDate } = await import("@bufbuild/protobuf/wkt");

  const REJECTED_ID = "b-rejected";
  const IMPORTED_ID = "b-imported";

  const batch = (id: string, fileName: string, sourceKind: number, status: number, createdAt: Date, failureCode = "") => ({
    id, entityId: "e1", sourceKind, status, fileName,
    byteLength: 1024n, failureCode, createdAt: timestampFromDate(createdAt),
  });

  return {
    transport: stubTransport({
      user: signedInUser,
      extend: (router) => {
        router.service(ImportService, {
          listImportBatches: () => ({
            batches: [
              batch(IMPORTED_ID, "nordea-2026-08.csv", SourceKind.BANK, ImportStatus.IMPORTED, new Date("2026-09-02T09:14:00Z")),
              batch(REJECTED_ID, "ledger-h1-2026.xlsx", SourceKind.LEDGER, ImportStatus.REJECTED, new Date("2026-08-28T16:02:00Z"), "balance_mismatch"),
            ],
          }),
          getImportBatch: (req) => ({
            batch:
              req.batchId === REJECTED_ID
                ? batch(REJECTED_ID, "ledger-h1-2026.xlsx", SourceKind.LEDGER, ImportStatus.REJECTED, new Date("2026-08-28T16:02:00Z"), "balance_mismatch")
                : batch(IMPORTED_ID, "nordea-2026-08.csv", SourceKind.BANK, ImportStatus.IMPORTED, new Date("2026-09-02T09:14:00Z")),
          }),
          getValidationReport: (req) => {
            if (req.batchId === REJECTED_ID) {
              return {
                report: {
                  batchId: REJECTED_ID,
                  outcome: ValidationOutcome.REJECTED,
                  rowCount: 300, errorCount: 5, warningCount: 1,
                  // Keyed by the line in the ORIGINAL file (CLAUDE.md), never
                  // a parsed index -- the fixture's own lines were 3, 47,
                  // 118, 119, 204; two of them is enough to prove the key
                  // survived the trip.
                  errors: [
                    { line: 3, field: "amount", code: "amount_unparseable", raw: "1 240,00 EUR" },
                    { line: 118, field: "currency", code: "currency_unknown", raw: "EURO" },
                  ],
                  warnings: [
                    {
                      code: "balance_mismatch",
                      balanceMismatch: {
                        opening: 15230000n, movements: 8452785n,
                        closing: 23684000n, difference: 1215n, currency: "EUR",
                      },
                    },
                  ],
                  overriddenByUserId: "", overrideReason: "",
                },
              };
            }
            return {
              report: {
                batchId: IMPORTED_ID, outcome: ValidationOutcome.VALID,
                rowCount: 412, errorCount: 0, warningCount: 0,
                errors: [], warnings: [], overriddenByUserId: "", overrideReason: "",
              },
            };
          },
          getDedupSummary: (req) => {
            if (req.batchId === REJECTED_ID) {
              // A rejected file persists nothing (CLAUDE.md: validation is
              // blocking and atomic), so there is no dedup summary to read.
              throw new Error("not found");
            }
            return {
              summary: { importedRows: 412, skippedInBatch: 6, skippedCrossBatch: 3, internalTransfers: 4 },
            };
          },
        });
      },
    }),
  };
});

const { makeRouter } = await import("../router");
const { transport } = await import("../transport");
const { t } = await import("../i18n");

const REJECTED_ID = "b-rejected";
const IMPORTED_ID = "b-imported";

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

describe("the import list", () => {
  it("shows each batch with its state and source", async () => {
    renderAt("/app/imports");
    expect(await screen.findByText("nordea-2026-08.csv")).toBeDefined();
    expect(screen.getAllByText(t("state.batch.imported")).length).toBeGreaterThan(0);
    expect(screen.getByText(t("state.batch.rejected"))).toBeDefined();
  });

  it("asks where the file is from before it will take one", async () => {
    renderAt("/app/imports");
    await screen.findByRole("heading", { name: t("imports.title") });
    expect(screen.getByText(t("imports.chooseSource"))).toBeDefined();
    expect(screen.getByText(t("imports.source.bank"))).toBeDefined();
    expect(screen.queryByText(t("imports.upload"))).toBeNull();
  });
});

describe("a rejected batch", () => {
  it("keys every error by the line in the original file", async () => {
    renderAt(`/app/imports/${REJECTED_ID}`);
    await screen.findByText(t("batch.errors.title"));
    for (const line of ["3", "118"]) {
      expect(screen.getByText(line), `file line ${line}`).toBeDefined();
    }
    expect(screen.queryByText("0")).toBeNull();
    expect(screen.queryByText("1")).toBeNull();
  });

  it("says why it was rejected, at the top", async () => {
    renderAt(`/app/imports/${REJECTED_ID}`);
    expect(await screen.findByText(t("batch.rejected.balance_mismatch"))).toBeDefined();
  });

  it("carries no counts, because a failed file stores nothing", async () => {
    renderAt(`/app/imports/${REJECTED_ID}`);
    await screen.findByRole("heading", { name: "ledger-h1-2026.xlsx" });
    expect(screen.queryByText(t("imports.counts.imported"))).toBeNull();
  });

  it("shows the balance check that refused it", async () => {
    renderAt(`/app/imports/${REJECTED_ID}`);
    await screen.findByRole("heading", { name: "ledger-h1-2026.xlsx" });
    expect(screen.getByText(t("batch.balance.closing"))).toBeDefined();
    expect(screen.getByText(t("batch.balance.declared"))).toBeDefined();
  });
});

describe("an imported batch", () => {
  it("shows the three counts DESIGN.md §2 forbids removing", async () => {
    renderAt(`/app/imports/${IMPORTED_ID}`);
    await screen.findByRole("heading", { name: "nordea-2026-08.csv" });
    for (const k of [
      "imports.counts.imported",
      "imports.counts.duplicates",
      "imports.counts.transfers",
    ] as const) {
      expect(screen.getByText(t(k)), k).toBeDefined();
    }
  });
});

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
          listBatchTransactions: (req) => {
            if (req.batchId === REJECTED_ID) {
              // A rejected file persists nothing, so there is nothing to list.
              return { rows: [] };
            }
            return {
              rows: [
                {
                  id: "r1", bookedOn: "2026-08-02", lineNo: 4, postingNo: 0, documentRef: "",
                  amount: { minorUnits: "-9640", currencyCode: "EUR" },
                  counterpartyRaw: "DHL EXPRESS", description: "DHL EXPRESS INVOICE 88214",
                  regulatedCode: "",
                  categoryCode: "04", categoryName: "Operating expenses",
                  engineLayer: "L1", evidence: "counterparty rule", confidence: 0.95,
                },
                {
                  id: "r2", bookedOn: "2026-08-03", lineNo: 5, postingNo: 0, documentRef: "",
                  amount: { minorUnits: "125000", currencyCode: "EUR" },
                  counterpartyRaw: "UNKNOWN VENDOR LLC", description: "UNKNOWN VENDOR LLC PAYMENT",
                  regulatedCode: "",
                  // Not yet reached by the review queue -- every classification
                  // field on the wire comes back empty, which is what the
                  // screen has to render as "unclassified", not blank.
                  categoryCode: "", categoryName: "", engineLayer: "", evidence: "",
                },
              ],
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

  it("shows the rows the file persisted, each with its own category", async () => {
    renderAt(`/app/imports/${IMPORTED_ID}`);
    await screen.findByRole("heading", { name: "nordea-2026-08.csv" });

    expect(await screen.findByText(t("batch.rows.title"))).toBeDefined();
    // The counterparty, not the description -- the screen prefers whichever
    // names who the money moved with, the same choice ReviewScreen's own
    // group label makes.
    expect(await screen.findByText("DHL EXPRESS")).toBeDefined();
    expect(screen.getByText("Operating expenses")).toBeDefined();
  });

  it("says a row is unclassified rather than leaving its category blank", async () => {
    renderAt(`/app/imports/${IMPORTED_ID}`);
    await screen.findByRole("heading", { name: "nordea-2026-08.csv" });

    expect(await screen.findByText("UNKNOWN VENDOR LLC")).toBeDefined();
    expect(screen.getByText(t("batch.rows.unclassified"))).toBeDefined();
  });

  it("shows no rows section for a rejected batch, which persisted nothing", async () => {
    renderAt(`/app/imports/${REJECTED_ID}`);
    await screen.findByText(t("batch.rejected.balance_mismatch"));
    expect(screen.queryByText(t("batch.rows.title"))).toBeNull();
  });
});

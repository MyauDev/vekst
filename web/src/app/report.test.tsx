import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { afterEach, describe, expect, it, vi } from "vitest";

const web = (...p: string[]) => join(dirname(fileURLToPath(import.meta.url)), "..", "..", ...p);

// Shared with the mock factory below via vi.hoisted, the same shape
// `router.test.tsx`'s own restructure uses: which source kinds the
// entity's batches carry, so one dedicated test can trigger the
// mixed-basis refusal without every other test in this file inheriting it.
const state = vi.hoisted(() => ({ batchSourceKinds: ["bank"] as ("bank" | "ledger")[] }));

vi.mock("../transport", async () => {
  const { stubTransport, signedInUser } = await import("../testTransport");
  const { ImportService, ImportStatus, SourceKind } = await import("../gen/vekst/v1/import_pb");
  const { ReviewService } = await import("../gen/vekst/v1/review_pb");
  const {
    ReportService, ReportBasis, ReportBucketKind, ReportAnswerKind,
  } = await import("../gen/vekst/v1/report_pb");
  const { timestampFromDate } = await import("@bufbuild/protobuf/wkt");

  // Eight periods, 2026-01..2026-08. Raw section values, and the computed
  // chain derived from them the same way core/internal/report/pnl.go's own
  // Chain does -- GM = NET SALES - CS, NM = GM - OCS, CM = NM - OPEX - OIE,
  // IBT = CM - FR, NI = IBT - CIT -- so a test asserting the lines add up
  // is asserting something this mock cannot fail to satisfy by accident.
  const PERIODS = ["2026-01", "2026-02", "2026-03", "2026-04", "2026-05", "2026-06", "2026-07", "2026-08"];
  const RAW: Record<string, number[]> = {
    "01": PERIODS.map(() => 800000), // NET SALES
    "02": PERIODS.map(() => -260000), // CS
    "03": PERIODS.map(() => -40000), // OCS
    "04": PERIODS.map(() => -300000), // OPEX
    "05": PERIODS.map(() => -10000), // OIE
    "06": PERIODS.map(() => 0), // FR
    "07": PERIODS.map(() => 0), // CIT
  };
  const sum = (...vs: number[]) => vs.reduce((a, b) => a + b, 0);
  const gm = PERIODS.map((_, i) => sum(RAW["01"]![i]!, RAW["02"]![i]!));
  const nm = PERIODS.map((_, i) => sum(gm[i]!, RAW["03"]![i]!));
  const cm = PERIODS.map((_, i) => sum(nm[i]!, RAW["04"]![i]!, RAW["05"]![i]!));
  const ibt = PERIODS.map((_, i) => sum(cm[i]!, RAW["06"]![i]!));
  const ni = PERIODS.map((_, i) => sum(ibt[i]!, RAW["07"]![i]!));

  const m = (minor: number) => ({ minorUnits: String(minor), currencyCode: "EUR" });
  const figure = (minor: number, percentOfRevenue?: number) => ({
    amount: m(minor), percentOfRevenue,
  });

  function reportLine(code: string, label: string, formula: string, computed: boolean, values: number[]) {
    const total = values.reduce((a, b) => a + b, 0);
    const revenueTotal = RAW["01"]!.reduce((a, b) => a + b, 0);
    return {
      code, label, formula, computed,
      byPeriod: values.map((v) => figure(v, revenueTotal !== 0 ? v / revenueTotal : undefined)),
      total: figure(total, revenueTotal !== 0 ? total / revenueTotal : undefined),
    };
  }

  const LINES = [
    reportLine("01", "NET SALES", "", false, RAW["01"]!),
    reportLine("02", "Cost of sales", "", false, RAW["02"]!),
    reportLine("91", "GM", "NET SALES - CS", true, gm),
    reportLine("03", "Other cost of sales", "", false, RAW["03"]!),
    reportLine("92", "NM", "GM - OCS", true, nm),
    reportLine("04", "Operating expenses", "", false, RAW["04"]!),
    reportLine("05", "Other expenses", "", false, RAW["05"]!),
    reportLine("93", "CM", "NM - OPEX - OIE", true, cm),
    reportLine("06", "Financial result", "", false, RAW["06"]!),
    reportLine("94", "IBT", "CM - FR", true, ibt),
    reportLine("07", "Tax", "", false, RAW["07"]!),
    reportLine("95", "NI", "IBT - CIT", true, ni),
  ];

  const BUCKETS = [
    { kind: ReportBucketKind.UNCLASSIFIED, byPeriod: PERIODS.map(() => m(5000)), total: m(5000 * PERIODS.length) },
    { kind: ReportBucketKind.NON_PNL, byPeriod: PERIODS.map(() => m(0)), total: m(0) },
    { kind: ReportBucketKind.UNALLOCATED, byPeriod: PERIODS.map(() => m(0)), total: m(0) },
    { kind: ReportBucketKind.OTHER_BASIS, byPeriod: PERIODS.map(() => m(0)), total: m(0) },
  ];

  const RECONCILIATION = PERIODS.map((period, i) => ({
    period,
    opening: m(i === 0 ? 1000000 : 1000000 + i * 50000),
    in: m(800000),
    out: m(760000),
    transfers: m(0),
    transferRowCount: 0,
    closing: m(1000000 + (i + 1) * 50000),
    balances: true,
    derived: true,
  }));

  interface StubTransaction {
    id: string; bookedOn: string; batchId: string; lineNo: number; postingNo: number;
    documentRef: string; amount: { minorUnits: string; currencyCode: string };
    counterpartyRaw: string; description: string; regulatedCode: string; sourceKind: string;
    categoryCode: string; categoryName: string; engineLayer: string; evidence: string;
    confidence: number; decidedBy: string;
  }
  const TRANSACTIONS_BY_LINE: Record<string, StubTransaction[]> = {
    "04|2026-03": [
      {
        id: "t2", bookedOn: "2026-03-09", batchId: "b1", lineNo: 12, postingNo: 0, documentRef: "",
        amount: m(-96240),
        counterpartyRaw: "DHL EXPRESS", description: "DHL EXPRESS INVOICE 88214",
        regulatedCode: "", sourceKind: "bank",
        categoryCode: "04", categoryName: "Operating expenses",
        engineLayer: "L0", evidence: "ledger doc 88214, ±0 days",
        confidence: 1, decidedBy: "",
      },
    ],
  };

  return {
    transport: stubTransport({
      user: signedInUser,
      extend: (router) => {
        router.service(ImportService, {
          listImportBatches: () => ({
            batches: state.batchSourceKinds.map((kind, i) => ({
              id: `b${i}`, entityId: "e1",
              sourceKind: kind === "bank" ? SourceKind.BANK : SourceKind.LEDGER,
              status: ImportStatus.IMPORTED, fileName: `statement-${i}.csv`,
              byteLength: 1024n, failureCode: "",
              createdAt: timestampFromDate(new Date("2026-08-01T00:00:00Z")),
            })),
          }),
        });
        router.service(ReviewService, {
          listReviewGroups: () => ({
            groups: [], totalRowCount: 0, totalCounterpartyCount: 0,
            totalAbsolute: { minorUnits: "0", currencyCode: "EUR" },
          }),
        });
        router.service(ReportService, {
          getManagementPNL: (req) => ({
            basis: req.basis === ReportBasis.LEDGER ? ReportBasis.LEDGER : ReportBasis.BANK,
            granularity: req.granularity,
            from: req.from, to: req.to,
            baseCurrency: "EUR",
            periods: PERIODS,
            lines: LINES,
            buckets: BUCKETS,
            versions: { taxonomy: ["v3"], ruleset: ["v11"], engine: ["0.4.2"], normalize: ["v1"] },
            reconciliation: RECONCILIATION,
          }),
          listLineTransactions: (req) => {
            if (req.line === "91") {
              // A computed line: no transactions of its own, only operands.
              return {
                kind: ReportAnswerKind.OPERANDS,
                transactions: [],
                operands: [
                  { code: "01", label: "NET SALES", subtracted: false },
                  { code: "02", label: "Cost of sales", subtracted: true },
                ],
                rowCount: 0, total: m(gm[PERIODS.indexOf(req.period)] ?? 0), nextCursor: "",
              };
            }
            const rows = TRANSACTIONS_BY_LINE[`${req.line}|${req.period}`] ?? [];
            const lineIndex = LINES.findIndex((l) => l.code === req.line);
            const total = lineIndex >= 0 ? LINES[lineIndex]!.byPeriod[PERIODS.indexOf(req.period)]!.amount : m(0);
            return {
              kind: ReportAnswerKind.TRANSACTIONS,
              transactions: rows,
              operands: [],
              rowCount: rows.length,
              total,
              nextCursor: "",
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
const { defaultRange } = await import("../ui/period");

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

afterEach(() => {
  document.documentElement.removeAttribute("data-theme");
  state.batchSourceKinds = ["bank"];
});

describe("the report", () => {
  it("names its basis with the table, not in a footnote", async () => {
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    expect(await screen.findByText(t("report.basis.cash"))).toBeDefined();
    // Derived from the data's source, never chosen -- and the screen says so.
    expect(screen.getByText(t("report.basis.note"))).toBeDefined();
  });

  it("refuses the whole report when the entity mixes bank and ledger imports", async () => {
    // The per-line "blocked" state is gone: a report is computed from one
    // source_kind for its whole duration, so the refusal is now of the
    // whole report, not of one line (design §2.5).
    state.batchSourceKinds = ["bank", "ledger"];
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    expect(await screen.findByText(t("report.blocked"))).toBeDefined();
    expect(screen.getByText(t("report.blocked.mixed_basis"))).toBeDefined();
  });

  it("shows the reconciliation strip, which is what proves nothing was dropped", async () => {
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });

    for (const k of ["recon.opening", "recon.in", "recon.out", "recon.transfers", "recon.closing"] as const) {
      expect(screen.getByText(t(k)), k).toBeDefined();
    }
  });

  it("shows the four exclusion buckets below the table", async () => {
    // CLAUDE.md's own invariant: below the table, in order, unclassified,
    // excluded non-P&L, unallocated, other basis. Never rendered by the
    // fixture, which never had this data.
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(screen.getByText(t("report.bucket.unclassified"))).toBeDefined();
    expect(screen.getByText(t("report.bucket.non_pnl"))).toBeDefined();
  });

  it("carries the currency once, in the header, not in every cell", async () => {
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(screen.getAllByText(/EUR/)).toHaveLength(1);
  });

  it("defaults to the table tab and puts that in the URL", async () => {
    const router = renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(router.state.location.search.view).toBe("table");
    // The P&L table's own % of revenue column -- unique to it, unlike
    // "Category", which every chart's own jsdom table fallback also shows.
    expect(screen.getByText(t("report.percentOfRevenue"))).toBeDefined();
    expect(screen.queryByText(t("chart.moneyflow.title"))).toBeNull();
  });

  it("switches to the charts tab without leaving the URL behind", async () => {
    const user = userEvent.setup();
    const router = renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });

    await user.click(screen.getByRole("link", { name: t("report.tab.charts") }));

    expect(await screen.findByText(t("chart.moneyflow.title"))).toBeDefined();
    expect(screen.queryByText(t("report.percentOfRevenue"))).toBeNull();
    expect(router.state.location.search.view).toBe("charts");
    // A tab switch is not a new range -- the period survives it.
    expect(router.state.location.search.from).toBe("2026-01");
  });

  it("keeps the reconciliation strip on both tabs", async () => {
    // DESIGN.md §2: a minimal pass may never remove it. It is not a
    // property of one tab.
    const user = userEvent.setup();
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(screen.getByText(t("recon.closing"))).toBeDefined();

    await user.click(screen.getByRole("link", { name: t("report.tab.charts") }));
    await screen.findByText(t("chart.moneyflow.title"));
    expect(screen.getByText(t("recon.closing"))).toBeDefined();
  });

  it("falls back to year-to-date when the link carries no range", async () => {
    // add-web-experience §4.11.
    const router = renderAt("/app/reports/pnl");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(router.state.location.search).toEqual({ ...defaultRange(), view: "table" });
  });

  it("keeps a truncated range from breaking the link", async () => {
    const router = renderAt("/app/reports/pnl?from=nonsense");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(router.state.location.search.from).toBe(defaultRange().from);
  });
});

describe("the drill-down is a route, not a state flag", () => {
  it("opens over a report that stays mounted", async () => {
    renderAt("/app/reports/pnl/cell/04/2026-03?from=2026-01&to=2026-08");

    expect(await screen.findByText(t("drilldown.close"))).toBeDefined();
    // The panel says which figure it is: a shared link arrives with no memory
    // of the click.
    expect(await screen.findByRole("heading", { name: "Operating expenses" })).toBeDefined();
    // And the report is still there underneath.
    expect(screen.getByRole("heading", { name: t("report.title") })).toBeDefined();
  });

  it("closes on the back button and leaves the report", async () => {
    // add-web-experience §4.10. This is the whole reason the panel is a route.
    const router = renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });

    await router.navigate({
      to: "/app/reports/pnl/cell/$categoryId/$period",
      params: { categoryId: "04", period: "2026-03" },
      search: { from: "2026-01", to: "2026-08", view: "table" },
    });
    await screen.findByText(t("drilldown.close"));

    router.history.back();
    await waitFor(() => expect(screen.queryByText(t("drilldown.close"))).toBeNull());
    expect(screen.getByRole("heading", { name: t("report.title") })).toBeDefined();
  });

  it("admits when a cell has no transactions rather than inventing them", async () => {
    // "05" (Other expenses) at 2026-05 has no entry in TRANSACTIONS_BY_LINE,
    // so the mock answers with zero rows -- a real, honest empty, not the
    // fixture's "not in the sample data".
    renderAt("/app/reports/pnl/cell/05/2026-05?from=2026-01&to=2026-08");
    expect(await screen.findByText(t("drilldown.empty"))).toBeDefined();
  });

  it("opens a computed line onto its operands, not transactions", async () => {
    // GM has none of its own: it is NET SALES minus CS, and both of those
    // have transactions (report.proto's own words for
    // REPORT_ANSWER_KIND_OPERANDS).
    renderAt("/app/reports/pnl/cell/91/2026-03?from=2026-01&to=2026-08");
    await screen.findByText(t("drilldown.close"));

    expect(await screen.findByText(t("drilldown.operands"))).toBeDefined();
    // The report table stays mounted underneath the panel (design D8), so
    // each operand's label legitimately appears twice -- once in the table
    // row, once in the operand link -- hence getAllByText, not getByText.
    expect(screen.getAllByText("NET SALES").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Cost of sales").length).toBeGreaterThan(0);
  });

  it("lists the transactions with the layer and confidence that make them auditable", async () => {
    // Never asserted until now: the list was windowed, and a virtual list renders
    // nothing without a measured viewport -- so this passed by never looking.
    renderAt("/app/reports/pnl/cell/04/2026-03?from=2026-01&to=2026-08");
    await screen.findByText(t("drilldown.close"));

    expect(await screen.findByText(/DHL EXPRESS INVOICE 88214/)).toBeDefined();
    expect(screen.getByText("L0")).toBeDefined();
    expect(screen.getAllByText(/%$/).length).toBeGreaterThan(0);
  });

  it("shows the provenance triple that makes the figure reproducible", async () => {
    renderAt("/app/reports/pnl/cell/04/2026-03?from=2026-01&to=2026-08");
    await screen.findByText(t("drilldown.close"));
    expect(await screen.findByText(/taxonomy v3.*ruleset v11.*engine 0\.4\.2/i)).toBeDefined();
  });
});

describe("both palettes, and print", () => {
  // add-web-experience §4.12. jsdom applies no CSS, so what is testable is that
  // the report renders under either theme and that the print override exists.
  for (const theme of ["light", "dark"] as const) {
    it(`renders under the ${theme} palette`, async () => {
      document.documentElement.setAttribute("data-theme", theme);
      renderAt("/app/reports/pnl?from=2026-01&to=2026-08&view=charts");
      expect(await screen.findByRole("heading", { name: t("report.title") })).toBeDefined();
      expect(screen.getByText(t("recon.closing"))).toBeDefined();

      // Task 8.16: every chart from WORKFLOW.md §5.3 renders under both
      // palettes, on the charts tab. jsdom has no 2D canvas context and no
      // ResizeObserver, so each chart falls back to its own table view --
      // still a real render of the same component tree, just not the
      // report's own table tab (that path is covered by the tests above).
      for (const key of [
        "chart.moneyflow.title",
        "chart.expenses.title",
        "chart.netresult.title",
        "chart.revenueExpense.title",
        "chart.trend.title",
      ] as const) {
        expect(screen.getByText(t(key))).toBeDefined();
      }
    });
  }

  it("forces a light ground when printing, whatever the theme", () => {
    const css = readFileSync(web("src", "index.css"), "utf8");
    const print = css.slice(css.indexOf("@media print"));
    // §9 requires the P&L to print readably in black and white. Without this a
    // dark-mode print is a page of toner.
    expect(print).toMatch(/\[data-theme="dark"\]/);
    expect(print).toMatch(/--vk-surface:\s*white/);
    expect(print).toMatch(/--vk-text:\s*black/);
  });
});

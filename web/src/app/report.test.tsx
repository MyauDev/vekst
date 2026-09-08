import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { afterEach, describe, expect, it } from "vitest";

import { makeRouter } from "../router";
import { stubTransport, signedInUser } from "../testTransport";
import { t } from "../i18n";
import { defaultRange } from "../ui/period";

const web = (...p: string[]) => join(dirname(fileURLToPath(import.meta.url)), "..", "..", ...p);

function renderAt(path: string) {
  const router = makeRouter(
    stubTransport({ user: signedInUser }),
    createMemoryHistory({ initialEntries: [path] }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

afterEach(() => document.documentElement.removeAttribute("data-theme"));

describe("the report", () => {
  it("names its basis with the table, not in a footnote", async () => {
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    expect(await screen.findByText(t("report.basis.cash"))).toBeDefined();
    // Derived from the data's source, never chosen -- and the screen says so.
    expect(screen.getByText(t("report.basis.note"))).toBeDefined();
  });

  it("blocks a line and names the reason in the same view", async () => {
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });

    // "Blocked" without the missing input named is a dead end -- DESIGN.md §2.
    expect(screen.getByText(t("report.blocked"))).toBeDefined();
    expect(
      screen.getByText(t("report.blocked.mixed_sources_no_match")),
    ).toBeDefined();
  });

  it("shows the reconciliation strip, which is what proves nothing was dropped", async () => {
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });

    for (const k of ["recon.opening", "recon.in", "recon.out", "recon.transfers", "recon.closing"] as const) {
      expect(screen.getByText(t(k)), k).toBeDefined();
    }
  });

  it("carries the currency once, in the header, not in every cell", async () => {
    renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(screen.getAllByText(/EUR/).length).toBe(1);
  });

  it("falls back to year-to-date when the link carries no range", async () => {
    // add-web-experience §4.11.
    const router = renderAt("/app/reports/pnl");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(router.state.location.search).toEqual(defaultRange());
  });

  it("keeps a truncated range from breaking the link", async () => {
    const router = renderAt("/app/reports/pnl?from=nonsense");
    await screen.findByRole("heading", { name: t("report.title") });
    expect(router.state.location.search.from).toBe(defaultRange().from);
  });
});

describe("the drill-down is a route, not a state flag", () => {
  it("opens over a report that stays mounted", async () => {
    renderAt("/app/reports/pnl/cell/logistics/2026-03?from=2026-01&to=2026-08");

    expect(await screen.findByText(t("drilldown.close"))).toBeDefined();
    // The panel says which figure it is: a shared link arrives with no memory
    // of the click.
    expect(await screen.findByRole("heading", { name: "Logistics" })).toBeDefined();
    // And the report is still there underneath.
    expect(screen.getByRole("heading", { name: t("report.title") })).toBeDefined();
  });

  it("closes on the back button and leaves the report", async () => {
    // add-web-experience §4.10. This is the whole reason the panel is a route.
    const router = renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
    await screen.findByRole("heading", { name: t("report.title") });

    await router.navigate({
      to: "/app/reports/pnl/cell/$categoryId/$period",
      params: { categoryId: "logistics", period: "2026-03" },
      search: { from: "2026-01", to: "2026-08" },
    });
    await screen.findByText(t("drilldown.close"));

    router.history.back();
    await waitFor(() => expect(screen.queryByText(t("drilldown.close"))).toBeNull());
    expect(screen.getByRole("heading", { name: t("report.title") })).toBeDefined();
  });

  it("admits when a figure's transactions are absent rather than inventing them", async () => {
    renderAt("/app/reports/pnl/cell/payroll/2026-05?from=2026-01&to=2026-08");
    expect(await screen.findByText(t("drilldown.unavailable"))).toBeDefined();
  });

  it("shows the provenance triple that makes the figure reproducible", async () => {
    renderAt("/app/reports/pnl/cell/logistics/2026-03?from=2026-01&to=2026-08");
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
      renderAt("/app/reports/pnl?from=2026-01&to=2026-08");
      expect(await screen.findByRole("heading", { name: t("report.title") })).toBeDefined();
      expect(screen.getByText(t("recon.closing"))).toBeDefined();
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

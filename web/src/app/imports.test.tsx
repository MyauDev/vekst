import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { beforeEach, describe, expect, it } from "vitest";

import { makeRouter } from "../router";
import { stubTransport, signedInUser } from "../testTransport";
import { t } from "../i18n";
import { resetFixtures } from "../data/imports";

beforeEach(resetFixtures);

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

describe("the import list", () => {
  it("shows each batch with its state and source", async () => {
    renderAt("/app/imports");
    // Wait for a row, not the heading: the heading renders outside the query,
    // so waiting on it resolves against the loading state and asserts nothing.
    expect(await screen.findByText("nordea-2026-08.csv")).toBeDefined();
    expect(screen.getAllByText(t("state.batch.imported")).length).toBeGreaterThan(0);
    expect(screen.getByText(t("state.batch.rejected"))).toBeDefined();
  });

  it("asks where the file is from before it will take one", async () => {
    renderAt("/app/imports");
    await screen.findByRole("heading", { name: t("imports.title") });

    // The tag decides the accounting basis and cannot be read from the file, so
    // the choice gates the input rather than sitting beside it.
    expect(screen.getByText(t("imports.chooseSource"))).toBeDefined();
    expect(screen.getByText(t("imports.source.bank"))).toBeDefined();
    expect(screen.queryByText(t("imports.upload"))).toBeNull();
  });
});

describe("a rejected batch", () => {
  it("keys every error by the line in the original file", async () => {
    // add-web-experience §6.6, and an invariant in CLAUDE.md. The fixture's
    // file lines are 3, 47, 118, 119 and 204 while their parsed indices are
    // 0..4 -- so a screen showing indices would render 0 and 1, and this fails.
    renderAt("/app/imports/b-ledger-h1");
    await screen.findByText(t("batch.errors.title"));

    for (const line of ["3", "47", "118", "204"]) {
      expect(screen.getByText(line), `file line ${line}`).toBeDefined();
    }
    expect(screen.queryByText("0")).toBeNull();
    expect(screen.queryByText("1")).toBeNull();
  });

  it("says why it was rejected, at the top", async () => {
    renderAt("/app/imports/b-ledger-h1");
    expect(await screen.findByText(t("batch.rejected.balance_mismatch"))).toBeDefined();
  });

  it("carries no counts, because a failed file stores nothing", async () => {
    renderAt("/app/imports/b-ledger-h1");
    await screen.findByRole("heading", { name: "ledger-h1-2026.xlsx" });
    expect(screen.queryByText(t("imports.counts.imported"))).toBeNull();
  });

  it("shows the balance check that refused it", async () => {
    renderAt("/app/imports/b-ledger-h1");
    await screen.findByRole("heading", { name: "ledger-h1-2026.xlsx" });
    expect(screen.getByText(t("batch.balance.closing"))).toBeDefined();
    expect(screen.getByText(t("batch.balance.declared"))).toBeDefined();
  });
});

describe("an imported batch", () => {
  it("shows the four counts DESIGN.md §2 forbids removing", async () => {
    renderAt("/app/imports/b-2026-08-nordea");
    await screen.findByRole("heading", { name: "nordea-2026-08.csv" });

    for (const k of [
      "imports.counts.imported",
      "imports.counts.duplicates",
      "imports.counts.transfers",
      "imports.counts.matches",
    ] as const) {
      expect(screen.getByText(t(k)), k).toBeDefined();
    }
  });
});

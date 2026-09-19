import { render, screen, within } from "@testing-library/react";
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

describe("Home", () => {
  it("is where /app lands", async () => {
    const router = renderAt("/app");
    await screen.findByRole("heading", { name: t("home.title") });
    expect(router.state.location.pathname).toBe("/app/home");
  });

  it("shows the real batch count and the latest batch, not a placeholder", async () => {
    renderAt("/app/home");
    // The fixture carries three batches (add-web-experience's imports fixture);
    // asserting the exact figure is what keeps this test from passing whether
    // or not the card is reading real data.
    expect(await screen.findByText("3")).toBeDefined();
    expect(screen.getByText("nordea-2026-08.csv")).toBeDefined();
  });

  it("shows the real awaiting-review count, the same one the rail badges", async () => {
    renderAt("/app/home");
    await screen.findByRole("heading", { name: t("home.title") });
    // Two groups in the fixture -- the same number the rail's own badge
    // shows, because both read ["reviewSummary"].
    expect(await screen.findByText(t("home.review.awaiting"))).toBeDefined();
    const card = screen.getByText(t("home.review.awaiting")).closest("a");
    expect(card?.textContent).toContain("2");
  });

  it("shows the real net result, computed the same way the report itself computes it", async () => {
    renderAt("/app/home");
    // getReport() ignores its range and always returns the same fixture, so
    // this figure is deterministic regardless of the current date.
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

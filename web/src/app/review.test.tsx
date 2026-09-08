import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { beforeEach, describe, expect, it } from "vitest";

import { makeRouter } from "../router";
import { stubTransport, signedInUser } from "../testTransport";
import { t } from "../i18n";
import { resetFixtures } from "../data/review";

beforeEach(resetFixtures);

function renderReview() {
  const router = makeRouter(
    stubTransport({ user: signedInUser }),
    createMemoryHistory({ initialEntries: ["/app/review"] }),
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe("the review queue", () => {
  it("shows one counterparty group with its transactions", async () => {
    renderReview();
    expect(await screen.findByText("VEKTOR LOGISTIKA")).toBeDefined();

    // The rows render. This is the assertion the windowed lists in section 4 and
    // section 6 were silently failing -- see design D14.
    expect(
      await screen.findByText(/VEKTOR LOGISTIKA OPLATA SCHET 4602/),
    ).toBeDefined();
  });

  it("keeps the key legend visible rather than behind a help control", async () => {
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    // DESIGN.md §8. A keyboard-first screen whose keys are hidden is a screen
    // nobody works twice.
    expect(screen.getByText(t("review.legend"))).toBeDefined();
    for (const k of ["review.key.digit", "review.key.enter", "review.key.transfer",
                     "review.key.notPnl", "review.key.clear"] as const) {
      expect(screen.getByText(t(k)), k).toBeDefined();
    }
  });
});

describe("worked entirely from the keyboard", () => {
  // add-web-experience §7.7. No pointer events anywhere in this block: if the
  // queue needs a click to start, it is not keyboard-first.
  it("picks a category with a digit and approves the group with Enter", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.keyboard("1");
    await user.keyboard("{Enter}");

    // Approving empties the group from the queue and the next one takes its place.
    await waitFor(() => expect(screen.queryByText("VEKTOR LOGISTIKA")).toBeNull());
    expect(await screen.findByText("KONTUR SERVICE")).toBeDefined();
  });

  it("marks an internal transfer with T", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.keyboard("t");
    await waitFor(() => expect(screen.queryByText("VEKTOR LOGISTIKA")).toBeNull());
  });

  it("marks a group out of the P&L with N", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.keyboard("n");
    await waitFor(() => expect(screen.queryByText("VEKTOR LOGISTIKA")).toBeNull());
  });

  it("does nothing on Enter with no category chosen", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    // Enter approves a classification. Without one there is nothing to approve,
    // and guessing which category was meant is the one thing this queue exists
    // to avoid.
    await user.keyboard("{Enter}");
    expect(screen.getByText("VEKTOR LOGISTIKA")).toBeDefined();
  });

  it("clears a selection with Escape", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.keyboard("1");
    await user.keyboard("{Escape}");
    await user.keyboard("{Enter}");
    expect(screen.getByText("VEKTOR LOGISTIKA")).toBeDefined();
  });

  it("moves between groups with the arrow keys", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.keyboard("{ArrowDown}");
    expect(await screen.findByText("KONTUR SERVICE")).toBeDefined();
    await user.keyboard("{ArrowUp}");
    expect(await screen.findByText("VEKTOR LOGISTIKA")).toBeDefined();
  });
});

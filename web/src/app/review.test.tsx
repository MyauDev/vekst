import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Shared with the mock factory below via vi.hoisted -- a resolved group has
// to disappear from subsequent listReviewGroups calls (the same behaviour
// the deleted `decided` fixture Set gave), and a test needs to be able to
// clear it between cases the way `resetFixtures` used to.
const { resolvedKeys } = vi.hoisted(() => ({ resolvedKeys: new Set<string>() }));

// `review.ts` imports `transport` statically from "../transport", the same
// convention `dedup.test.ts` and `imports.test.tsx` mock around.
vi.mock("../transport", async () => {
  const { stubTransport, signedInUser } = await import("../testTransport");
  const { ReviewService } = await import("../gen/vekst/v1/review_pb");

  const GROUPS = [
    {
      counterpartyKey: "vektor-logistika", displayName: "VEKTOR LOGISTIKA", rowCount: 2,
      total: { minorUnits: "-240700", currencyCode: "EUR" },
      firstSeen: "2026-08-04", lastSeen: "2026-08-19",
    },
    {
      counterpartyKey: "kontur-service", displayName: "KONTUR SERVICE", rowCount: 1,
      total: { minorUnits: "-24900", currencyCode: "EUR" },
      firstSeen: "2026-08-11", lastSeen: "2026-08-11",
    },
  ];
  const TRANSACTIONS: Record<
    string,
    { id: string; bookedOn: string; direction: string; amount: { minorUnits: string; currencyCode: string }; description: string; counterpartyRaw: string; regulatedCode: string; sourceKind: string }[]
  > = {
    "vektor-logistika": [
      {
        id: "r1", bookedOn: "2026-08-04", direction: "expense",
        amount: { minorUnits: "-142300", currencyCode: "EUR" },
        description: "VEKTOR LOGISTIKA OPLATA SCHET 4602",
        counterpartyRaw: "", regulatedCode: "", sourceKind: "bank",
      },
      {
        id: "r2", bookedOn: "2026-08-19", direction: "expense",
        amount: { minorUnits: "-98400", currencyCode: "EUR" },
        description: "VEKTOR LOGISTIKA OPLATA SCHET 4655",
        counterpartyRaw: "", regulatedCode: "", sourceKind: "bank",
      },
    ],
    "kontur-service": [
      {
        id: "r3", bookedOn: "2026-08-11", direction: "expense",
        amount: { minorUnits: "-24900", currencyCode: "EUR" },
        description: "KONTUR SERVICE PODPISKA",
        counterpartyRaw: "", regulatedCode: "", sourceKind: "bank",
      },
    ],
  };
  const CATEGORIES = [
    { id: "c1", code: "0402010202", name: "Logistics", path: "OPEX > Marketing and Sales > Marketing > MRK Services > Logistics", isPnl: true },
    { id: "c2", code: "04020102", name: "Software and subscriptions", path: "OPEX > Marketing and Sales > Marketing > MRK Services", isPnl: true },
  ];

  return {
    transport: stubTransport({
      user: signedInUser,
      extend: (router) => {
        router.service(ReviewService, {
          listReviewGroups: () => {
            const remaining = GROUPS.filter((g) => !resolvedKeys.has(g.counterpartyKey));
            return {
              groups: remaining,
              totalRowCount: remaining.reduce((n, g) => n + g.rowCount, 0),
              totalCounterpartyCount: remaining.length,
              totalAbsolute: { minorUnits: "0", currencyCode: "EUR" },
            };
          },
          listGroupTransactions: (req) => ({ transactions: TRANSACTIONS[req.counterpartyKey] ?? [] }),
          listCategories: () => ({ categories: CATEGORIES }),
          resolveGroup: (req) => {
            resolvedKeys.add(req.counterpartyKey);
            return {
              decisionId: "d1", coveredCount: 1,
              coveredTotal: { minorUnits: "0", currencyCode: "EUR" },
            };
          },
          undoDecision: () => ({ retractedCount: 0 }),
        });
      },
    }),
  };
});

const { makeRouter } = await import("../router");
const { transport } = await import("../transport");
const { t } = await import("../i18n");

beforeEach(() => resolvedKeys.clear());

function renderReview() {
  const router = makeRouter(transport, createMemoryHistory({ initialEntries: ["/app/review"] }));
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

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

// Shared with the mock factory below via vi.hoisted -- a resolved group has
// to disappear from subsequent listReviewGroups calls (the same behaviour
// the deleted `decided` fixture Set gave), and a test needs to be able to
// clear it between cases the way `resetFixtures` used to.
const { resolvedKeys, failNextResolve } = vi.hoisted(() => ({
  resolvedKeys: new Set<string>(),
  // A test's way of making resolveGroup answer the way the real backend does
  // when a decision is refused -- a coded failure, not a network error.
  failNextResolve: { code: null as string | null },
}));

// `review.ts` imports `transport` statically from "../transport", the same
// convention `dedup.test.ts` and `imports.test.tsx` mock around.
vi.mock("../transport", async () => {
  const { stubTransport, signedInUser } = await import("../testTransport");
  const { ReviewService } = await import("../gen/vekst/v1/review_pb");
  const { Code, ConnectError } = await import("@connectrpc/connect");

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
  // Ten, not two: change 5.4's own reason for existing is that "1"-"9" alone
  // cannot address the tenth, and a fixture with only two categories could
  // never prove that a click reaches it.
  const CATEGORIES = [
    { id: "c1", code: "0402010202", name: "Logistics", path: "OPEX > Marketing and Sales > Marketing > MRK Services > Logistics", isPnl: true },
    { id: "c2", code: "04020102", name: "Software and subscriptions", path: "OPEX > Marketing and Sales > Marketing > MRK Services", isPnl: true },
    { id: "c3", code: "0401020201", name: "Office rent", path: "OPEX > Admin > Office rent", isPnl: true },
    { id: "c4", code: "0401020202", name: "Utilities", path: "OPEX > Admin > Utilities", isPnl: true },
    { id: "c5", code: "0401020203", name: "Insurance", path: "OPEX > Admin > Insurance", isPnl: true },
    { id: "c6", code: "0401020204", name: "Legal fees", path: "OPEX > Admin > Legal fees", isPnl: true },
    { id: "c7", code: "0401020205", name: "Bank commission", path: "OPEX > Admin > Bank commission", isPnl: true },
    { id: "c8", code: "0401020206", name: "Travel", path: "OPEX > Admin > Travel", isPnl: true },
    { id: "c9", code: "0401020207", name: "Training", path: "OPEX > Admin > Training", isPnl: true },
    { id: "c10", code: "0401020208", name: "Equipment", path: "OPEX > Admin > Equipment", isPnl: true },
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
            if (failNextResolve.code) {
              const code = failNextResolve.code;
              failNextResolve.code = null;
              throw new ConnectError(code, Code.InvalidArgument);
            }
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

beforeEach(() => {
  resolvedKeys.clear();
  failNextResolve.code = null;
});

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

describe("worked from a click, digit shortcuts included", () => {
  it("picks a category by clicking it and approves with the button", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.click(screen.getByRole("button", { name: /Logistics/ }));
    expect(screen.getByText("Logistics", { selector: "span" })).toBeDefined();

    await user.click(screen.getByRole("button", { name: t("review.action.approve") }));
    await waitFor(() => expect(screen.queryByText("VEKTOR LOGISTIKA")).toBeNull());
    expect(await screen.findByText("KONTUR SERVICE")).toBeDefined();
  });

  // A coded failure (a stale category, a race with another reviewer) must be
  // readable, not silent -- the two look identical to a person watching the
  // screen otherwise. Regression for the bug where approving did nothing.
  it("shows a message when the backend refuses the decision, and the group stays", async () => {
    const user = userEvent.setup();
    failNextResolve.code = "review_unknown_category";
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.click(screen.getByRole("button", { name: /Logistics/ }));
    await user.click(screen.getByRole("button", { name: t("review.action.approve") }));

    expect((await screen.findByRole("alert")).textContent).toBe(t("error.review_unknown_category"));
    // The group was not silently dropped: it is still there to retry.
    expect(screen.getByText("VEKTOR LOGISTIKA")).toBeDefined();
  });

  it("reaches a category past the ninth, which no digit key can address", async () => {
    // "Equipment" is CATEGORIES[9] -- the tenth entry, one past every digit
    // key this screen has. Before this change there was no way to select it
    // at all, mouse or keyboard.
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    const equipment = screen.getByRole("button", { name: /Equipment/ });
    // Only the first nine carry a key cap; the tenth has none to click by mistake.
    expect(equipment.textContent).toBe("Equipment");

    await user.click(equipment);
    await user.click(screen.getByRole("button", { name: t("review.action.approve") }));
    await waitFor(() => expect(screen.queryByText("VEKTOR LOGISTIKA")).toBeNull());
  });

  it("marks a transfer and not-in-P&L with their own buttons, not only T and N", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.click(screen.getByRole("button", { name: t("review.action.transfer") }));
    await waitFor(() => expect(screen.queryByText("VEKTOR LOGISTIKA")).toBeNull());

    await screen.findByText("KONTUR SERVICE");
    await user.click(screen.getByRole("button", { name: t("review.action.notPnl") }));
    await waitFor(() => expect(screen.queryByText("KONTUR SERVICE")).toBeNull());
  });

  it("leaves Approve disabled until something is selected", async () => {
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");
    const approve = screen.getByRole("button", { name: t("review.action.approve") }) as HTMLButtonElement;
    expect(approve.disabled).toBe(true);
  });

  it("says in words what a digit only underlines", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    expect(screen.queryByText(new RegExp(t("review.selected")))).toBeNull();
    await user.keyboard("1");
    expect(screen.getByText(new RegExp(t("review.selected")))).toBeDefined();
    expect(screen.getByText("Logistics", { selector: "span" })).toBeDefined();
  });

  it("filters the category list, and renumbers 1-9 onto what matched", async () => {
    // "1" stays a shortcut even while the filter field has focus -- no
    // category label in this taxonomy contains a digit, so there is nothing
    // real for it to conflict with, and filter-then-digit is the point.
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    const search = screen.getByLabelText(t("review.search.placeholder"));
    await user.type(search, "Bank commission");

    expect(screen.queryByRole("button", { name: /Logistics/ })).toBeNull();
    // "1" now addresses the one filtered match, not CATEGORIES[0].
    await user.keyboard("1");
    expect(await screen.findByText("Bank commission", { selector: "span" })).toBeDefined();
  });

  it("does not treat T or N as shortcuts while the filter field has focus", async () => {
    // Regression: the global T/N shortcuts used to fire even while this
    // field had focus -- "Training" would have marked an internal transfer
    // on its very first letter instead of ever reaching the search box.
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    const search = screen.getByLabelText(t("review.search.placeholder")) as HTMLInputElement;
    await user.click(search);
    await user.keyboard("Training");
    expect(search.value).toBe("Training");
    // The group is still here -- typing "T" did not resolve it out from
    // under the person still typing.
    expect(screen.getByText("VEKTOR LOGISTIKA")).toBeDefined();
  });
});

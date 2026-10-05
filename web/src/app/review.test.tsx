import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
  // A tree, because the picker is one. "Costs > Admin" holds ten leaves, not
  // two: "1"-"9" alone cannot address the tenth, and a fixture with fewer
  // could never prove that a click reaches it. Two leaves are called
  // "Salary" -- the reason the picker became a tree -- and one of them sits
  // under a single-child branch, which the tree folds into its parent.
  //
  // Codes are "98…"/"99…", deliberately outside the real taxonomy:
  // `categoryName()` translates a known code regardless of what the fixture
  // calls it, and a code this suite invented colliding with a real one would
  // have this fixture's own "Bank commission" silently overridden by the
  // production Russian catalogue's name for a completely different leaf. An
  // org-scoped code this build has never heard of is also the more
  // representative case -- most of an organisation's own taxonomy is exactly
  // that.
  const admin = (code: string, name: string) => ({
    id: `c${code}`, code, name, path: `Costs > Admin > ${name}`, isPnl: true,
  });
  const CATEGORIES = [
    admin("990101", "Logistics"),
    admin("990102", "Office rent"),
    admin("990103", "Utilities"),
    admin("990104", "Insurance"),
    admin("990105", "Legal fees"),
    admin("990106", "Bank commission"),
    admin("990107", "Travel"),
    admin("990108", "Training"),
    admin("990109", "Software"),
    admin("990110", "Equipment"),
    { id: "c11", code: "990201", name: "Salary", path: "Costs > Developers > Salary", isPnl: true },
    { id: "c12", code: "99030101", name: "Salary", path: "Costs > Sales team > Staff > Salary", isPnl: true },
    { id: "c13", code: "990302", name: "Sales tools", path: "Costs > Sales team > Sales tools", isPnl: true },
    { id: "c14", code: "9801", name: "Consulting", path: "Revenue > Consulting", isPnl: true },
    // A real taxonomy code, unlike the rest -- for the one test that needs
    // `categoryName()` and the section and branch names to actually translate
    // something rather than fall back to what the wire sent.
    { id: "c15", code: "0401010203", name: "Audit", path: "OPEX > Administration > Finance > FI Services > Audit", isPnl: true },
    // A real income leaf: every group in GROUPS is money going out, so this
    // one is offered only after asking for the other side.
    { id: "c16", code: "050202", name: "Return of payment", path: "OIE > OTHER INCOME > Return of payment", isPnl: true },
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
const { setLocale } = await import("../ui/preferences");

beforeEach(() => {
  resolvedKeys.clear();
  failNextResolve.code = null;
});

// setLocale's emit() notifies every mounted useLocale() subscriber
// synchronously, which is a real state update outside of any event React
// already knows to batch -- act() is what tells it this one is deliberate.
afterEach(() => act(() => setLocale("en")));

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
                     "review.key.notPnl", "review.key.back", "review.key.clear"] as const) {
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

    // Costs, then Admin, then Logistics: a digit opens a branch and picks a leaf.
    await user.keyboard("111");
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

    await user.keyboard("111");
    await user.keyboard("{Escape}");
    await user.keyboard("{Enter}");
    expect(screen.getByText("VEKTOR LOGISTIKA")).toBeDefined();
    // And the picker is back at the top of the tree.
    expect(screen.getByRole("button", { name: /Costs/ })).toBeDefined();
  });

  it("goes back up a level with Backspace", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.keyboard("11");
    expect(screen.getByRole("button", { name: /Logistics/ })).toBeDefined();
    await user.keyboard("{Backspace}");
    expect(screen.queryByRole("button", { name: /Logistics/ })).toBeNull();
    expect(screen.getByRole("button", { name: /Developers/ })).toBeDefined();
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

    await user.click(screen.getByRole("button", { name: /Costs/ }));
    await user.click(screen.getByRole("button", { name: /Admin/ }));
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

    await user.keyboard("111");
    await user.click(screen.getByRole("button", { name: t("review.action.approve") }));

    expect((await screen.findByRole("alert")).textContent).toBe(t("error.review_unknown_category"));
    // The group was not silently dropped: it is still there to retry.
    expect(screen.getByText("VEKTOR LOGISTIKA")).toBeDefined();
  });

  it("reaches a category past the ninth, which no digit key can address", async () => {
    // "Equipment" is the tenth leaf under Costs > Admin, one past every digit
    // key this screen has. Before change 5.4 there was no way to select it
    // at all, mouse or keyboard.
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");
    await user.keyboard("11");

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
    await user.keyboard("111");
    // The whole path, not the leaf alone: the leaf alone is "Salary" five times.
    expect(screen.getByText(new RegExp(`${t("review.selected")}: Costs › Admin ›`))).toBeDefined();
    expect(screen.getByText("Logistics", { selector: "span" })).toBeDefined();
  });

  it("offers an outflow only expense categories until asked for the rest", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    expect(screen.getByText(t("review.side.expense"))).toBeDefined();
    const search = screen.getByLabelText(t("review.search.placeholder"));
    await user.type(search, "Return of payment");
    expect(screen.queryByRole("button", { name: /Return of payment/ })).toBeNull();

    await user.click(screen.getByRole("button", { name: t("review.side.showAll") }));
    expect(screen.getByRole("button", { name: /Return of payment/ })).toBeDefined();

    // Hiding the side again drops a choice made from it, rather than leaving
    // it armed behind Enter where nobody can see it.
    await user.click(screen.getByRole("button", { name: /Return of payment/ }));
    await user.click(screen.getByRole("button", { name: t("review.side.showMatching") }));
    const approve = screen.getByRole("button", { name: t("review.action.approve") }) as HTMLButtonElement;
    expect(approve.disabled).toBe(true);
  });

  it("tells two leaves with the same name apart by where they sit", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    // Searching shows every match with its path.
    await user.type(screen.getByLabelText(t("review.search.placeholder")), "Salary");
    const matches = screen.getAllByRole("button", { name: /Salary/ });
    expect(matches.map((b) => b.textContent)).toEqual([
      "1Costs › Developers › Salary",
      "2Costs › Sales team › Staff › Salary",
    ]);
  });

  it("folds a branch with a single child into it, below the sections", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    await user.click(screen.getByRole("button", { name: /Costs/ }));
    await user.click(screen.getByRole("button", { name: /Sales team/ }));
    // "Staff" holds only "Salary": one click, not two, and both names shown.
    await user.click(screen.getByRole("button", { name: /Staff › Salary/ }));
    expect(screen.getByText(new RegExp(`${t("review.selected")}: Costs › Sales team › Staff ›`))).toBeDefined();

    // A section is never folded, even with a single leaf: "Revenue" is the
    // step that says which side of the P&L this is.
    await user.click(screen.getByRole("button", { name: t("review.tree.root") }));
    expect(screen.getByRole("button", { name: /Revenue/ })).toBeDefined();
    expect(screen.queryByRole("button", { name: /Consulting/ })).toBeNull();
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

  // Regression: a filter typed for one counterparty used to survive into the
  // next one -- approving auto-advances the queue, but neither the filter nor
  // the selection was ever scoped to the group that made them. The picker for
  // the new group looked like it had silently lost most of the taxonomy, when
  // every category was still there under a leftover search term nobody meant
  // to apply to it.
  it("clears the filter and the pending selection once the queue advances", async () => {
    const user = userEvent.setup();
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    const search = screen.getByLabelText(t("review.search.placeholder"));
    await user.type(search, "Bank commission");
    await user.click(screen.getByRole("button", { name: /Bank commission/ }));
    await user.click(screen.getByRole("button", { name: t("review.action.approve") }));

    await screen.findByText("KONTUR SERVICE");

    expect((search as HTMLInputElement).value).toBe("");
    // The whole tree is back from its top, not only the one leaf the old
    // filter matched.
    expect(screen.getByRole("button", { name: /Costs/ })).toBeDefined();
    expect(screen.getByRole("button", { name: /Revenue/ })).toBeDefined();
    // Nothing carried over as a pre-made choice for the new group.
    const approve = screen.getByRole("button", { name: t("review.action.approve") }) as HTMLButtonElement;
    expect(approve.disabled).toBe(true);
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

  it("shows a category's name in Russian while the wire keeps sending English", async () => {
    act(() => setLocale("ru"));
    renderReview();
    await screen.findByText("VEKTOR LOGISTIKA");

    // "Audit" (c15, code 0401010203) is a real taxonomy code; the rest of the
    // fixture deliberately is not, so this is the one place the Russian
    // catalogue in i18n.ts actually renders instead of falling back to what
    // the wire sent -- the section, the branches and the leaf alike.
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: /Операционные расходы/ }));
    await user.click(screen.getByRole("button", { name: /Администрирование/ }));
    // "Finance" holds only "FI Services", which holds only "Audit": folded.
    expect(screen.getByRole("button", { name: /Финансы › Услуги › Аудит/ })).toBeDefined();
    expect(screen.queryByRole("button", { name: /Audit/ })).toBeNull();

    // The rest still show the wire's own English name: they are not in the
    // static catalogue, and a translation nobody wrote is not owed to any
    // code the frontend does not recognise.
    await user.click(screen.getByRole("button", { name: t("review.tree.root", "ru") }));
    expect(screen.getByRole("button", { name: /Costs/ })).toBeDefined();
  });
});

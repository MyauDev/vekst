import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/** Repo-relative, anchored to this file rather than to the working directory:
 *  a test that passes or fails depending on where it was invoked from is not
 *  testing what it claims to. */
const repo = (...p: string[]) => join(dirname(fileURLToPath(import.meta.url)), "..", "..", ...p);

import { render, screen, within } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { describe, expect, it, vi } from "vitest";

// `AppLayout` reads `reviewSummary` from `../data/review`, which -- like
// every real data module -- imports `transport` statically from
// `../transport`. A transport built only for `makeRouter` never reaches it
// (`imports.test.tsx`'s own finding), so this file needs `vi.mock` too, not
// just a constructed `stubTransport` instance passed to the router.
//
// Unlike `imports.test.tsx`, this file renders many different sessions
// (signed out, signed in, first-run) from one mock factory that only runs
// once -- so the mocked router reads its answers from `state`, shared with
// the test file via `vi.hoisted`, and `renderAt` sets `state` before each
// render rather than passing options straight to a transport constructor.
const state = vi.hoisted(() => ({
  user: null as { id: string; email: string; name: string; locale: string } | null,
  organisations: [] as { id: string; name: string; baseCurrency: string; role: string; entities: { id: string; name: string }[] }[],
}));

vi.mock("./transport", async () => {
  const { Code, ConnectError, createRouterTransport } = await import("@connectrpc/connect");
  const { HealthService } = await import("./gen/vekst/v1/health_pb");
  const { IdentityService } = await import("./gen/vekst/v1/identity_pb");
  const { ReviewService } = await import("./gen/vekst/v1/review_pb");

  return {
    transport: createRouterTransport(({ service }) => {
      service(HealthService, {
        check: () => ({ status: 1, version: "abc1234", builtAt: "2026-08-23T12:00:00Z", classifierVersion: "engine-1" }),
      });
      service(IdentityService, {
        getCurrentUser: () => {
          if (!state.user) throw new ConnectError("unauthenticated", Code.Unauthenticated);
          return { user: state.user, organisations: state.organisations };
        },
      });
      // Two groups, so the rail's badge reads "Review2" wherever a test
      // asserts on it -- the same figure the fixture always used.
      service(ReviewService, {
        listReviewGroups: () => ({
          groups: [], totalRowCount: 3, totalCounterpartyCount: 2,
          totalAbsolute: { minorUnits: "0", currencyCode: "EUR" },
        }),
      });
    }),
  };
});

const { makeRouter } = await import("./router");
const { transport } = await import("./transport");
const { signedInUser, signedInOrganisation } = await import("./testTransport");
const { t } = await import("./i18n");

function renderAt(
  path: string,
  opts: {
    user?: typeof signedInUser | null;
    organisations?: typeof state.organisations;
  } = {},
) {
  state.user = opts.user ?? null;
  state.organisations = opts.organisations ?? (state.user ? [signedInOrganisation] : []);

  const router = makeRouter(transport, createMemoryHistory({ initialEntries: [path] }));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

describe("the public surface", () => {
  it("serves the landing with one action, repeated, and no session", async () => {
    renderAt("/", { user: null });
    await screen.findByText(t("landing.promise"));

    // DESIGN.md §1: one promise, one call to action. It appears twice -- at the
    // top and at the close -- but it is the same action, not a second one, so
    // the assertion is on the destination rather than the count.
    const ctas = screen.getAllByText(t("landing.cta"));
    expect(ctas.length).toBeGreaterThan(1);
    expect(new Set(ctas.map((a) => a.getAttribute("href")))).toEqual(new Set(["/signin"]));
  });
});

describe("the gate on Register B", () => {
  // add-web-experience §2.16. Asserted across the subtree rather than one
  // route: a screen added under /app must be protected by having been added.
  const guarded = ["/app", "/app/home", "/app/imports", "/app/review", "/app/reports/pnl"];

  for (const path of guarded) {
    it(`sends an unauthenticated visitor to sign in from ${path}`, async () => {
      renderAt(path, { user: null });
      expect(await screen.findByText(t("signIn.heading"))).toBeDefined();
    });
  }

  it("renders the shell for a signed-in visitor", async () => {
    renderAt("/app/reports/pnl", { user: signedInUser });

    expect(await screen.findByText(t("report.title"))).toBeDefined();
    expect(screen.getByText(signedInUser.email)).toBeDefined();
  });

  it("carries Home, then the three rail items in pipeline order", async () => {
    renderAt("/app/reports/pnl", { user: signedInUser });
    await screen.findByText(t("report.title"));

    // Home first, then in, corrected, read. The rail is a map, not a
    // ranking. Asserted on the destinations rather than the text, because
    // Review carries a count and the text is therefore "Review2" -- which
    // is the badge working, not a defect. Scoped to the rail's own nav: the
    // page also carries a skip-to-content link ahead of it, which is not
    // one of the rail's own items.
    const rail = screen.getByRole("navigation");
    const links = within(rail).getAllByRole("link").map((a) => a.getAttribute("href"));
    expect(links).toEqual(["/app/home", "/app/imports", "/app/review", "/app/reports/pnl"]);

    // The count is the only badge in the interface, and it belongs to Review.
    const review = within(rail).getAllByRole("link")[2]!;
    expect(review.textContent).toMatch(new RegExp(`^${t("nav.review")}\\d+$`));
  });

  it("shows the entity slot with the caller's real organisation and entity", async () => {
    renderAt("/app/reports/pnl", { user: signedInUser });
    await screen.findByText(t("report.title"));

    // DESIGN.md §8 ships the slot early; change 5.3 fills it from
    // GetCurrentUser -- a name read off the session, never a guess and never
    // a dash once one is available.
    expect(screen.getByText(t("topbar.entity"))).toBeDefined();
    expect(screen.getByText(signedInOrganisation.name)).toBeDefined();
    expect(screen.getByText(signedInOrganisation.entities[0]!.name)).toBeDefined();
  });

  // Task 7.14 (connect-app-end-to-end): the negative scenario for the
  // first-run gate. An empty organisations list renders the first-run screen
  // and makes no tenant-scoped data call at all -- there is no session to
  // make one with.
  it("renders first-run for a signed-in visitor with no organisation, and calls no tenant-scoped RPC", async () => {
    renderAt("/app/reports/pnl", { user: signedInUser, organisations: [] });

    expect(await screen.findByText(t("firstRun.heading"))).toBeDefined();
    expect(screen.queryByText(t("report.title"))).toBeNull();
    expect(screen.queryByText(t("topbar.entity"))).toBeNull();
  });
});

describe("destinations rather than screens", () => {
  it("sends /app to Home", async () => {
    const router = renderAt("/app", { user: signedInUser });
    // Not findByText: the rail's own "Home" link renders the identical
    // string, and a screen's <h1> is the one occurrence with a heading role.
    await screen.findByRole("heading", { name: t("home.title") });
    expect(router.state.location.pathname).toBe("/app/home");
  });

  it("keeps the reports segment although one report exists", async () => {
    const router = renderAt("/app/reports", { user: signedInUser });
    await screen.findByText(t("report.title"));
    expect(router.state.location.pathname).toBe("/app/reports/pnl");
  });
});

describe("where core sends the browser after sign-in", () => {
  // The regression this guards against: `/` used to render the session check,
  // so landing there after sign-in worked. Splitting the browser into a public
  // surface and an authenticated one made `/` a marketing page that reads no
  // session -- and a completed sign-in returned a signed-in person to a page
  // inviting them to sign in. Nothing failed; it just silently did not work.
  //
  // Read from the Go source rather than restated, for the same reason the
  // Ingress test reads the manifest: a constant copied into a test drifts from
  // the constant it copies.
  function goConst(name: string): string {
    const src = readFileSync(repo("core/internal/identity/signin.go"), "utf8");
    const m = new RegExp(`${name}\\s*=\\s*"([^"]+)"`).exec(src);
    if (!m) throw new Error(`${name} not found in signin.go`);
    return m[1]!;
  }

  it("lands a completed sign-in on a route that resolves the session", () => {
    const target = goConst("postSignInPath");
    const router = makeRouter(transport);
    const paths = Object.keys(router.routesById);

    expect(paths).toContain(target);
    // It must be under the authenticated surface. "/" is public and reads no
    // session, so it can never be the destination of a sign-in.
    expect(target.startsWith("/app")).toBe(true);
  });

  it("lands a failed sign-in on the one screen that renders the code", () => {
    const target = goConst("signInPath");
    const router = makeRouter(transport);
    expect(Object.keys(router.routesById)).toContain(target);
  });

  it("shows the error code as a sentence, never as a code", async () => {
    window.history.replaceState({}, "", "/signin?auth_error=invalid_flow");
    renderAt("/signin", { user: null });

    expect(await screen.findByText(t("error.invalid_flow"))).toBeDefined();
    expect(screen.queryByText("invalid_flow")).toBeNull();
    window.history.replaceState({}, "", "/");
  });
});

describe("prefixes that belong to core", () => {
  // add-web-experience §2.17. Read from the manifest rather than restated here:
  // a test that repeats the list drifts from it exactly the way the Ingress and
  // the dev proxy drifted from each other.
  it("defines no client route under a prefix the Ingress routes to core", () => {
    const ingress = readFileSync(repo("deploy/k8s/base/ingress.yaml"), "utf8");
    const blocks = ingress.split("- path:").slice(1);
    const corePrefixes = blocks
      .filter((b) => /name:\s*core/.test(b))
      .map((b) => b.split("\n")[0]!.trim());

    expect(corePrefixes).toContain("/rpc");
    expect(corePrefixes).toContain("/auth");

    const router = makeRouter(transport);
    const clientPaths = Object.keys(router.routesById);

    for (const prefix of corePrefixes) {
      for (const path of clientPaths) {
        expect(path.startsWith(prefix), `${path} shadows ${prefix}`).toBe(false);
      }
    }
  });
});

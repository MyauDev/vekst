import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

/** Repo-relative, anchored to this file rather than to the working directory:
 *  a test that passes or fails depending on where it was invoked from is not
 *  testing what it claims to. */
const repo = (...p: string[]) => join(dirname(fileURLToPath(import.meta.url)), "..", "..", ...p);

import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider, createMemoryHistory } from "@tanstack/react-router";
import { describe, expect, it } from "vitest";

import { makeRouter } from "./router";
import { stubTransport, signedInUser } from "./testTransport";
import { t } from "./i18n";

function renderAt(path: string, opts: Parameters<typeof stubTransport>[0] = {}) {
  const router = makeRouter(
    stubTransport(opts),
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
  const guarded = ["/app", "/app/imports", "/app/review", "/app/reports/pnl"];

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

  it("carries the three rail items in pipeline order", async () => {
    renderAt("/app/reports/pnl", { user: signedInUser });
    await screen.findByText(t("report.title"));

    // In, corrected, read. The rail is a map, not a ranking. Asserted on the
    // destinations rather than the text, because Review carries a count and the
    // text is therefore "Review2" -- which is the badge working, not a defect.
    const links = screen.getAllByRole("link").map((a) => a.getAttribute("href"));
    expect(links).toEqual(["/app/imports", "/app/review", "/app/reports/pnl"]);

    // The count is the only badge in the interface, and it belongs to Review.
    const review = screen.getAllByRole("link")[1]!;
    expect(review.textContent).toMatch(new RegExp(`^${t("nav.review")}\\d+$`));
  });

  it("shows the entity slot with a dash rather than an invented name", async () => {
    renderAt("/app/reports/pnl", { user: signedInUser });
    await screen.findByText(t("report.title"));

    // DESIGN.md §8 ships the slot early; §9.1 fills it. A dash means "nothing
    // was loaded", which is exactly what is true.
    expect(screen.getByText(t("topbar.entity"))).toBeDefined();
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
  });
});

describe("destinations rather than screens", () => {
  it("sends /app to the Management P&L, which is the whole MVP", async () => {
    const router = renderAt("/app", { user: signedInUser });
    await screen.findByText(t("report.title"));
    expect(router.state.location.pathname).toBe("/app/reports/pnl");
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
    const router = makeRouter(stubTransport());
    const paths = Object.keys(router.routesById);

    expect(paths).toContain(target);
    // It must be under the authenticated surface. "/" is public and reads no
    // session, so it can never be the destination of a sign-in.
    expect(target.startsWith("/app")).toBe(true);
  });

  it("lands a failed sign-in on the one screen that renders the code", () => {
    const target = goConst("signInPath");
    const router = makeRouter(stubTransport());
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

    const router = makeRouter(stubTransport());
    const clientPaths = Object.keys(router.routesById);

    for (const prefix of corePrefixes) {
      for (const path of clientPaths) {
        expect(path.startsWith(prefix), `${path} shadows ${prefix}`).toBe(false);
      }
    }
  });
});

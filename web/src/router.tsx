import { Suspense, lazy } from "react";
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  redirect,
} from "@tanstack/react-router";
import type { RouterHistory } from "@tanstack/react-router";
import type { Transport } from "@connectrpc/connect";

import { LandingPage } from "./site/LandingPage";
import { SignInPage } from "./site/SignInPage";
import { AppLayout } from "./app/AppLayout";
import { ImportsScreen } from "./app/ImportsScreen";
import { BatchScreen } from "./app/BatchScreen";
import { ReviewScreen } from "./app/ReviewScreen";
import { ReportScreen } from "./app/ReportScreen";
import { DrilldownPanel } from "./app/DrilldownPanel";
import { defaultRange } from "./ui/period";

/** What every route can reach. */
interface RouterContext {
  transport: Transport;
}

/**
 * The root renders an outlet and nothing else.
 *
 * It used to carry the application's heading, so every route inherited it. That
 * is wrong once more than one register exists: the landing, the token sheet and
 * the application are three different frames, and a shell hoisted to the root
 * is one none of them can decline. Each route brings its own.
 */
const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: () => (
    <Suspense fallback={null}>
      <Outlet />
    </Suspense>
  ),
});

/* ---------------------------------------------------------------------------
 * Public — Register A. No session required and none read.
 * ------------------------------------------------------------------------ */

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: LandingPage,
});

const signInRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/signin",
  component: SignInPage,
});

/* ---------------------------------------------------------------------------
 * Application — Register B. One gate for the whole subtree.
 * ------------------------------------------------------------------------ */

const appRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/app",
  component: function App() {
    const { transport } = appRoute.useRouteContext();
    return (
      <AppLayout
        transport={transport}
        // A rendered sign-in rather than a redirect: the person is already
        // where they meant to be, and bouncing them to another URL loses that.
        unauthenticated={<SignInPage />}
      >
        <Outlet />
      </AppLayout>
    );
  },
});

/**
 * `/app` and `/app/reports` are destinations, not screens.
 *
 * The Management P&L is the whole MVP, so `/app` goes straight there rather
 * than through an overview nobody came for. `/app/reports` keeps its segment
 * although one report exists: Sales, OPEX and Cash Flow arrive at Commercial as
 * siblings of `pnl`, and the cost of the segment now is one redirect while the
 * cost of adding it later is every saved link.
 */
const appIndexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/",
  beforeLoad: () => {
    // The range is required on the target, so a redirect has to carry one.
    // Year-to-date: an accountant's year is the unit that matters.
    throw redirect({ to: "/app/reports/pnl", search: defaultRange() });
  },
});

const importsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "imports",
  component: ImportsScreen,
});

const batchRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "imports/$batchId",
  component: BatchScreen,
});

const reviewRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "review",
  component: ReviewScreen,
});

const reportsRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "reports",
  beforeLoad: () => {
    // The range is required on the target, so a redirect has to carry one.
    // Year-to-date: an accountant's year is the unit that matters.
    throw redirect({ to: "/app/reports/pnl", search: defaultRange() });
  },
});

const PERIOD = /^\d{4}-(0[1-9]|1[0-2])$/;

/**
 * The period range lives in the URL because it is the thing a shared link is
 * about. Organisation and entity do not: they come from the session, and a
 * tenant identifier the caller supplies is a tenant identifier the caller chose
 * (design D7).
 *
 * An unparseable range falls back to the default rather than erroring. A link
 * that someone truncated should still open the report.
 */
const pnlRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "reports/pnl",
  validateSearch: (search: Record<string, unknown>): { from: string; to: string } => {
    const fallback = defaultRange();
    const pick = (v: unknown, d: string) =>
      typeof v === "string" && PERIOD.test(v) ? v : d;
    return {
      from: pick(search["from"], fallback.from),
      to: pick(search["to"], fallback.to),
    };
  },
  component: ReportScreen,
});

/**
 * The drill-down is a **route**, rendered as a panel over the report, which
 * stays mounted beneath it.
 *
 * `DESIGN.md` §8 asks for a panel; `FRONTEND_PLAN.md` §5 asks for a link that
 * survives a copy-paste and a back button. A nested route is the one shape that
 * gives both, and it makes the back button close the panel rather than leave
 * the report.
 */
const drilldownRoute = createRoute({
  getParentRoute: () => pnlRoute,
  path: "cell/$categoryId/$period",
  component: DrilldownPanel,
});

/* ---------------------------------------------------------------------------
 * Development only.
 * ------------------------------------------------------------------------ */

/**
 * The token sheet. Both the guard and the dynamic import matter:
 * `import.meta.env.DEV` is replaced with a literal at build time, so this body
 * is dead code in production and the import inside it is dropped with it. A
 * static import at the top of this file would leave the route unreachable but
 * ship the component anyway, which is what the first version of this did.
 */
function devOnlyRoutes() {
  if (!import.meta.env.DEV) return [];
  return [
    createRoute({
      getParentRoute: () => rootRoute,
      path: "/tokens",
      component: lazy(() => import("./ui/TokenSheet").then((m) => ({ default: m.TokenSheet }))),
    }),
  ];
}

/**
 * `/rpc` and `/auth` belong to core and are deliberately absent here. A client
 * route under either shadows the backend in development, in production, or in
 * both -- `add-web-experience` §2.17 asserts it against the Ingress rather than
 * against a list repeated in a test.
 */
export function makeRouter(transport: Transport, history?: RouterHistory) {
  const routeTree = rootRoute.addChildren([
    indexRoute,
    signInRoute,
    appRoute.addChildren([
      appIndexRoute,
      importsRoute,
      batchRoute,
      reviewRoute,
      reportsRoute,
      pnlRoute.addChildren([drilldownRoute]),
    ]),
    ...devOnlyRoutes(),
  ]);
  // `history` is for tests, which need to start somewhere other than "/".
  return createRouter({ routeTree, context: { transport }, ...(history ? { history } : {}) });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof makeRouter>;
  }
}

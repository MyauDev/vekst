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
import { NotFound } from "./site/NotFound";
import { SignInPage } from "./site/SignInPage";
import { AppLayout } from "./app/AppLayout";
import { HomeScreen } from "./app/HomeScreen";
import { ImportsScreen } from "./app/ImportsScreen";
import { BatchScreen } from "./app/BatchScreen";
import { ReviewScreen } from "./app/ReviewScreen";
import { ReportScreen } from "./app/ReportScreen";
import { DrilldownPanel } from "./app/DrilldownPanel";
import { getPeriodRange } from "./ui/periodPreference";

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
 * `/app` redirects to `/app/home`, added 2026-09-19 alongside the home screen
 * itself. Superseded comment, kept for the record: this used to send `/app`
 * straight to the Management P&L "rather than through an overview nobody
 * came for," on the reasoning that the P&L was the whole MVP and a landing
 * screen would be a redirect through nothing. A rail with three destinations
 * and no way back to a start is the thing that stopped being true — Home is
 * that way back, not an overview added for its own sake.
 *
 * `/app/reports` keeps its own segment although one report exists: Sales,
 * OPEX and Cash Flow arrive at Commercial as siblings of `pnl`, and the cost
 * of the segment now is one redirect while the cost of adding it later is
 * every saved link.
 */
const homeRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "home",
  component: HomeScreen,
});

const appIndexRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/app/home" });
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
    // The range is required on the target, so a redirect has to carry one --
    // the last one chosen anywhere in the app, falling back to year-to-date
    // only the first time this browser has ever opened a report.
    throw redirect({ to: "/app/reports/pnl", search: { ...getPeriodRange(), view: "table" } });
  },
});

const PERIOD = /^\d{4}-(0[1-9]|1[0-2])$/;

/**
 * The period range lives in the URL because it is the thing a shared link is
 * about. Organisation and entity do not: they come from the session, and a
 * tenant identifier the caller supplies is a tenant identifier the caller chose
 * (design D7).
 *
 * An unparseable range falls back to the last one chosen anywhere in the app
 * (`ui/periodPreference.ts`) rather than erroring or resetting to year-to-date
 * every time. A link that someone truncated should still open a report, and
 * the range it opens to should be the one a person was just looking at, not
 * a fixed default that discards it.
 */
/**
 * "table" is the default and the fallback for anything unrecognised: the
 * P&L table is the MVP (`ReportScreen`'s own file comment), and a truncated
 * or hand-edited link should land on it rather than on a screen that needs
 * more explaining.
 */
const VIEWS = ["table", "charts"] as const;
export type ReportView = (typeof VIEWS)[number];

const pnlRoute = createRoute({
  getParentRoute: () => appRoute,
  path: "reports/pnl",
  validateSearch: (
    search: Record<string, unknown>,
  ): { from: string; to: string; view: ReportView } => {
    const fallback = getPeriodRange();
    const pick = (v: unknown, d: string) =>
      typeof v === "string" && PERIOD.test(v) ? v : d;
    const view = VIEWS.includes(search["view"] as ReportView)
      ? (search["view"] as ReportView)
      : "table";
    return {
      from: pick(search["from"], fallback.from),
      to: pick(search["to"], fallback.to),
      view,
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
      homeRoute,
      importsRoute,
      batchRoute,
      reviewRoute,
      reportsRoute,
      pnlRoute.addChildren([drilldownRoute]),
    ]),
    ...devOnlyRoutes(),
  ]);
  // `history` is for tests, which need to start somewhere other than "/".
  return createRouter({
    routeTree,
    context: { transport },
    // A single fallback for any unmatched depth, public or signed-in: an
    // unmatched path has no session answer yet, so it cannot inherit either
    // register's chrome. See `site/NotFound.tsx`.
    defaultNotFoundComponent: NotFound,
    ...(history ? { history } : {}),
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof makeRouter>;
  }
}

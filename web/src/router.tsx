import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
} from "@tanstack/react-router";
import type { Transport } from "@connectrpc/connect";

import { HealthCard } from "./HealthCard";

/** What every route can reach. Change 5.2 adds the session here. */
interface RouterContext {
  transport: Transport;
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: () => (
    <main className="mx-auto flex min-h-dvh max-w-lg flex-col justify-center gap-6 p-8">
      <div>
        <h1 className="text-2xl font-semibold text-slate-900">Vekst</h1>
        <p className="text-sm text-slate-500">
          Walking skeleton — every value below crossed both service boundaries.
        </p>
      </div>
      <Outlet />
    </main>
  ),
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: function Index() {
    const { transport } = indexRoute.useRouteContext();
    return <HealthCard transport={transport} />;
  },
});

/**
 * One route today. The router is here so that the screens change 5.2 adds —
 * Imports, Review, P&L — are a route each rather than a refactor.
 */
export function makeRouter(transport: Transport) {
  return createRouter({
    routeTree: rootRoute.addChildren([indexRoute]),
    context: { transport },
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof makeRouter>;
  }
}

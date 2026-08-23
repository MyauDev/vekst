import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { createRouterTransport } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import { makeRouter } from "./router";
import { HealthService } from "./gen/vekst/v1/health_pb";

function stubTransport() {
  return createRouterTransport(({ service }) => {
    service(HealthService, {
      check: () => ({
        status: 1,
        version: "route-test",
        builtAt: "2026-08-23T12:00:00Z",
        classifierVersion: "engine-1",
      }),
    });
  });
}

describe("router", () => {
  it("renders the index route with the transport from context", async () => {
    const router = makeRouter(stubTransport());
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });

    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
    );

    expect(await screen.findByText("route-test")).toBeDefined();
    expect(screen.getByText("engine-1")).toBeDefined();
  });
});

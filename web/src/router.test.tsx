import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { describe, expect, it } from "vitest";

import { makeRouter } from "./router";
import { stubTransport, signedInUser } from "./testTransport";

describe("router", () => {
  it("renders the index route with the transport from context", async () => {
    const router = makeRouter(stubTransport({ user: signedInUser, version: "route-test" }));
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

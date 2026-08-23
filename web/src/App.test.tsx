import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRouterTransport } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import { App } from "./App";
import { HealthService } from "./gen/vekst/v1/health_pb";

/** A transport that answers from memory. No network, no core, no classifier. */
function stubTransport(classifierVersion: string) {
  return createRouterTransport(({ service }) => {
    service(HealthService, {
      check: () => ({
        status: 1,
        version: "abc1234",
        builtAt: "2026-08-23T12:00:00Z",
        classifierVersion,
      }),
    });
  });
}

function renderApp(classifierVersion: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <App transport={stubTransport(classifierVersion)} />
    </QueryClientProvider>,
  );
}

describe("walking skeleton", () => {
  it("renders the values returned by the transport", async () => {
    renderApp("engine-9");

    expect(await screen.findByText("abc1234")).toBeDefined();
    expect(screen.getByText("2026-08-23T12:00:00Z")).toBeDefined();
    expect(screen.getByText("engine-9")).toBeDefined();
    expect(screen.getByText("SERVING")).toBeDefined();
  });

  it("shows the classifier as unreachable without failing the page", async () => {
    // ARCHITECTURE.md 3.5: an empty classifier_version is a valid answer.
    renderApp("");

    expect(await screen.findByText("SERVING")).toBeDefined();
    expect(screen.getByText("unreachable")).toBeDefined();
  });
});

import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import { HealthCard } from "./HealthCard";
import { stubTransport } from "./testTransport";

function renderCard(opts: Parameters<typeof stubTransport>[0] = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <HealthCard transport={stubTransport(opts)} />
    </QueryClientProvider>,
  );
}

// The walking skeleton, and the acceptance test for change 0.1: these values
// travelled browser -> Connect -> core -> gRPC -> classifier and back. The
// assertions moved here when the application shell replaced the scaffold that
// used to render this card.
describe("walking skeleton", () => {
  it("renders the values returned by the transport", async () => {
    renderCard({ version: "abc1234", classifierVersion: "engine-9" });

    expect(await screen.findByText("abc1234")).toBeDefined();
    expect(screen.getByText("2026-08-23T12:00:00Z")).toBeDefined();
    expect(screen.getByText("engine-9")).toBeDefined();
    expect(screen.getByText("SERVING")).toBeDefined();
  });

  it("shows the classifier as unreachable without failing the page", async () => {
    // ARCHITECTURE.md §3.5: an empty classifier_version is a valid answer, not
    // an outage of core.
    renderCard({ classifierVersion: "" });

    expect(await screen.findByText("SERVING")).toBeDefined();
    expect(screen.getByText("unreachable")).toBeDefined();
  });
});

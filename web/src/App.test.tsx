import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";

import { App } from "./App";
import { stubTransport, signedInUser } from "./testTransport";
import { t } from "./i18n";

function renderApp(opts: Parameters<typeof stubTransport>[0] = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <App transport={stubTransport(opts)} />
    </QueryClientProvider>,
  );
}

describe("sign-in gating", () => {
  it("renders the sign-in screen when nobody is signed in", async () => {
    renderApp({ user: null });

    const button = await screen.findByText(t("signIn.google"));
    // A link, not a button: the OIDC flow is a browser redirect, and only a
    // top-level navigation can follow core's 302 to Google.
    expect(button.getAttribute("href")).toBe("/auth/google/start");
  });

  it("renders the shell with a sign-out control when signed in", async () => {
    renderApp({ user: signedInUser });

    expect(await screen.findByText(signedInUser.email)).toBeDefined();
    expect(screen.getByText(t("signedIn.signOut"))).toBeDefined();
  });

  it("says plainly that a signed-in person has no organisation yet", async () => {
    // Change 1.1's absence must look deliberate rather than broken.
    renderApp({ user: signedInUser });

    expect(await screen.findByText(t("signedIn.noOrganisation"))).toBeDefined();
  });
});

describe("walking skeleton", () => {
  it("renders the values returned by the transport once signed in", async () => {
    renderApp({ user: signedInUser, classifierVersion: "engine-9" });

    expect(await screen.findByText("abc1234")).toBeDefined();
    expect(screen.getByText("2026-08-23T12:00:00Z")).toBeDefined();
    expect(screen.getByText("engine-9")).toBeDefined();
    expect(screen.getByText("SERVING")).toBeDefined();
  });

  it("shows the classifier as unreachable without failing the page", async () => {
    // ARCHITECTURE.md 3.5: an empty classifier_version is a valid answer.
    renderApp({ user: signedInUser, classifierVersion: "" });

    expect(await screen.findByText("SERVING")).toBeDefined();
    expect(screen.getByText("unreachable")).toBeDefined();
  });
});

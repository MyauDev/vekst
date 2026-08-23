import { describe, expect, it } from "vitest";

import { RPC_BASE_URL, transport } from "./transport";

describe("transport", () => {
  it("uses a relative base URL", () => {
    // Absolute would mean the browser makes a cross-origin request, and CORS
    // would become a decision somebody has to make under deployment pressure.
    expect(RPC_BASE_URL.startsWith("/")).toBe(true);
    expect(RPC_BASE_URL).not.toMatch(/^https?:/);
  });

  it("routes under the path the Ingress forwards to core", () => {
    expect(RPC_BASE_URL).toBe("/rpc");
  });

  it("is constructed", () => {
    expect(transport).toBeDefined();
  });
});

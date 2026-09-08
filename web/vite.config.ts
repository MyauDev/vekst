import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: true,
    port: 5173,
    // In the cluster the Ingress routes /rpc to core, so the browser is
    // same-origin and needs no CORS (design D6). This proxy makes a bare
    // `vite dev` outside the cluster behave the same way, so the client code
    // has one code path rather than an environment check.
    proxy: {
      // Both prefixes belong to core, and the Ingress routes them the same way
      // in the cluster (deploy/k8s/base/ingress.yaml). /auth is the one
      // non-Connect browser surface -- start, callback and logout -- which
      // cannot be RPCs because a redirect is not a remote procedure call.
      // Without it here, sign-in works in the cluster and 404s under `vite dev`.
      "/rpc": {
        target: process.env.VEKST_CORE_URL ?? "http://localhost:8080",
        changeOrigin: false,
      },
      "/auth": {
        target: process.env.VEKST_CORE_URL ?? "http://localhost:8080",
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
    coverage: {
      provider: "v8",
      // lcov is what SonarQube reads; text keeps the terminal useful.
      reporter: ["text", "lcov"],
      // Generated clients are not ours to cover.
      exclude: [
        "src/gen/**",
        "**/*.config.ts",
        "**/*.test.ts",
        "**/*.test.tsx",
        // Composition root: createRoot and render, no logic to assert.
        "src/main.tsx",
        "src/test-setup.ts",
        // Clickable mockups of the Demo screens. Not production code and not
        // imported by it. See src/mock/README.md; delete with 5.2.
        "src/mock/**",
      ],
    },
  },
});

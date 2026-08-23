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
      "/rpc": {
        target: process.env.VEKST_CORE_URL ?? "http://localhost:8080",
        changeOrigin: false,
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
  },
});

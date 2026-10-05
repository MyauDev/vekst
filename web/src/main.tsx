import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";

import { makeRouter } from "./router";
import { transport } from "./transport";
import { initPreferences } from "./ui/preferences";
import { initPeriodRange } from "./ui/periodPreference";
import "./index.css";

// Before the first render: a theme applied after paint is a flash of the
// wrong palette, and this is the composition root, which is where that
// belongs. initPeriodRange has to run here too and not later -- router.tsx's
// validateSearch reads it synchronously on the very first route match.
initPreferences();
initPeriodRange();

const root = document.getElementById("root");
if (!root) throw new Error("#root is missing from index.html");

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={new QueryClient()}>
      <RouterProvider router={makeRouter(transport)} />
    </QueryClientProvider>
  </StrictMode>,
);

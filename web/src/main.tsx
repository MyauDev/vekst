import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";

import { makeRouter } from "./router";
import { transport } from "./transport";
import { initPreferences } from "./ui/preferences";
import "./index.css";

// Before the first render: a theme applied after paint is a flash of the
// wrong palette, and this is the composition root, which is where that
// belongs.
initPreferences();

const root = document.getElementById("root");
if (!root) throw new Error("#root is missing from index.html");

createRoot(root).render(
  <StrictMode>
    <QueryClientProvider client={new QueryClient()}>
      <RouterProvider router={makeRouter(transport)} />
    </QueryClientProvider>
  </StrictMode>,
);

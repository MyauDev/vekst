import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import "./mock.css";

const root = document.getElementById("mock-root");
if (!root) throw new Error("#mock-root is missing from mock.html");

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

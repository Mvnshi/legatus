import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles/index.css";
import { App } from "./App";

const root = document.getElementById("root");
if (!root) throw new Error("index.html has no #root element");
// createRoot replaces the pre-paint headline in index.html with the real page.
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

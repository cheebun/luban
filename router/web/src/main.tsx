import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import { App } from "./App.tsx";
import { Providers } from "./Providers.tsx";

const container = document.getElementById("root");

if (!container) {
  throw new Error("Root element not found");
}

const root = createRoot(container);

root.render(
  <StrictMode>
    <Providers>
      <App />
    </Providers>
  </StrictMode>,
);

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { installAuthRedirect } from "./lib/auth";

installAuthRedirect();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);

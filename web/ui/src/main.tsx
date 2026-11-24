import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { installAuthRedirect } from "@beacon-shared/auth";

installAuthRedirect();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);

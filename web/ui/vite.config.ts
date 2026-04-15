import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
      // Shared React components are vendored from beacon-stack/web-shared
      // via `git subtree add --prefix=web/ui/src/shared` and sync upstream
      // bugfixes with `git subtree pull`. See README.md for the canaries
      // that would justify graduating to a real package.
      "@beacon-shared": path.resolve(__dirname, "./src/shared"),
      // Force React singleton. Kept as free insurance even after vendoring —
      // if the subtree ever grows a React-coupled transitive dep (Radix,
      // Framer Motion, etc.) the dedupe stops being enough and we'll need
      // a proper published package.
      react: path.resolve(__dirname, "node_modules/react"),
      "react-dom": path.resolve(__dirname, "node_modules/react-dom"),
    },
    dedupe: ["react", "react-dom"],
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
  },
  build: {
    outDir: "../static",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8383",
        changeOrigin: true,
        ws: true,
      },
    },
  },
});

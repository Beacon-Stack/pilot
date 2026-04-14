import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "path";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
      "@beacon-shared": path.resolve(__dirname, "../../../web-shared"),
      // Force React singleton. Without these, web-shared's own
      // node_modules/react (installed for TypeScript type resolution)
      // gets bundled alongside this service's own react, producing two
      // React instances. The first shared hook call then crashes with
      // "Cannot read properties of null (reading 'useState')".
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

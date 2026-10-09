import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The site is served from https://mvnshi.github.io/legatus/, so every URL it emits starts with /legatus/.
export default defineConfig({
  base: "/legatus/",
  plugins: [react()],
  css: {
    // XP.css ships a few selectors that are not valid CSS (a pseudo-element followed by :not()). Browsers ignore
    // them; the minifier would refuse them, so it is told to drop what it cannot parse and keep going.
    lightningcss: { errorRecovery: true },
  },
  build: {
    target: "es2022",
    sourcemap: false,
    chunkSizeWarningLimit: 400,
  },
  server: {
    // The real cockpit screenshots live in ../assets and are imported from there.
    fs: { allow: [".."] },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});

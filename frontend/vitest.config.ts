import { defineConfig } from "vitest/config";
import path from "path";

// The test suite runs against the same transform and resolver as `vite build`,
// so a test importing a component's types is checked the way that build checks
// the component. One config, two scripts, nothing to keep in sync.
export default defineConfig({
  resolve: {
    alias: { "@backend": path.resolve(__dirname, "wailsjs/go/") },
  },
  test: {
    globals: true,
    // Only the pure modules are under test. Nothing here needs a DOM: the UI is
    // not automatable today and building that scaffolding is a bigger decision
    // than wiring up a runner.
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
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
    // Pure modules, plus components rendered to a string. Nothing here needs a
    // DOM: renderToString runs in node, which is enough to catch a component
    // that throws during render -- the failure that blanked the app after
    // Prepare. Interaction still is not automatable; that needs a DOM runner
    // and is a bigger decision than this suite.
    environment: "node",
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
  },
});
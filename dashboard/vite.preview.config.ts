import { defineConfig } from "vite";

export default defineConfig({
  build: {
    emptyOutDir: false,
    lib: {
      entry: "src/features/preview/review-bridge.ts",
      name: "WorkbenchPreviewReview",
      formats: ["iife"],
      fileName: () => "preview-review.js",
    },
  },
});

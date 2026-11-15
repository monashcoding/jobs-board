import { macLintConfig } from "@monashcoding/lint";
import { defineConfig } from "oxlint";

export default defineConfig(
  macLintConfig({
    // Each of these goes back to "error" once its count reaches zero.
    rules: {
      "anti-slop/require-safety-comment-for-type-assertion": "warn",
      "anti-slop/no-runtime-typeof": "warn",
      "anti-slop/no-known-value-widening": "warn",
      "anti-slop/no-conditional-empty-object-spread": "warn",
      "anti-slop/no-unsafe-dictionary-type": "warn",
      "anti-slop/no-array-filter-map": "warn",
      "anti-slop/no-unknown-parameters": "warn",
      "anti-slop/no-unknown-returns": "warn",
    },
  }),
);

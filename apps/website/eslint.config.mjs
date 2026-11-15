import nextCoreWebVitals from "eslint-config-next/core-web-vitals";
import nextTypescript from "eslint-config-next/typescript";

const eslintConfig = [
  ...nextCoreWebVitals,
  ...nextTypescript,
  {
    rules: {
      "react-hooks/set-state-in-effect": "warn",
      // tsconfig has verbatimModuleSyntax: an inline `type` import still loads
      // the module, which pulls server code into the browser bundle.
      "@typescript-eslint/no-import-type-side-effects": "error",
    },
  },
];

export default eslintConfig;

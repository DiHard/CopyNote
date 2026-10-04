import js from "../web/node_modules/@eslint/js/src/index.js";
import globals from "../web/node_modules/globals/index.js";

export default [
  js.configs.recommended,
  { languageOptions: { globals: globals.node } },
  {
    files: ["uxharness/stub.js"],
    languageOptions: { globals: globals.browser },
  },
];

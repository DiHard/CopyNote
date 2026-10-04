module.exports = {
  forbidden: [
    { name: "no-cycles", severity: "error", from: {}, to: { circular: true } },
    {
      name: "actions-use-shared-state",
      comment:
        "Feature actions import appState directly; the facade imports the actions.",
      severity: "error",
      from: { path: "^src/lib/(entries|settings|updates|relocation)\\.ts$" },
      to: { path: "^src/lib/state\\.svelte\\.ts$" },
    },
    {
      name: "shared-state-has-no-feature-dependencies",
      severity: "error",
      from: { path: "^src/lib/appState\\.svelte\\.ts$" },
      to: {
        path: "^src/lib/(state\\.svelte|entries|settings|updates|relocation)\\.ts$",
      },
    },
  ],
  options: {
    doNotFollow: { path: "node_modules" },
    tsConfig: { fileName: "tsconfig.json" },
    enhancedResolveOptions: { extensions: [".ts", ".js", ".svelte"] },
  },
};

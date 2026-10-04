import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { transform } from "esbuild";

const source = await readFile("singlefile.ts", "utf8");
const compiled = await transform(source, { loader: "ts", format: "esm" });
const { singleFile } = await import(
  `data:text/javascript;base64,${Buffer.from(compiled.code).toString("base64")}`
);

function bundle() {
  return {
    "index.html": {
      type: "asset",
      source:
        '<script type="module" crossorigin src="/assets/main.js"></script><link rel="stylesheet" href="/assets/main.css">',
    },
    "assets/main.js": {
      type: "chunk",
      isEntry: true,
      imports: [],
      dynamicImports: [],
      code: 'console.log("$& </script>");',
    },
    "assets/main.css": { type: "asset", source: "body { color: red; }" },
  };
}

test("frontend output is one HTML with intact inline JavaScript and CSS", () => {
  const output = bundle();
  singleFile().generateBundle({}, output);
  assert.deepEqual(Object.keys(output), ["index.html"]);
  assert.match(
    output["index.html"].source,
    /<style>body \{ color: red; \}<\/style>/,
  );
  assert.ok(
    output["index.html"].source.includes('console.log("$& <\\/script>");'),
  );
  assert.doesNotMatch(output["index.html"].source, /\b(?:src|href)=/);
});

test("unreferenced or external assets fail the portable build", () => {
  const missing = bundle();
  missing["index.html"].source = "<html></html>";
  assert.throws(
    () => singleFile().generateBundle({}, missing),
    /Missing HTML reference/,
  );
  const split = bundle();
  split["assets/main.js"].dynamicImports = ["other.js"];
  assert.throws(
    () => singleFile().generateBundle({}, split),
    /Unexpected external/,
  );
  assert.throws(
    () => singleFile().generateBundle({}, {}),
    /Expected index.html/,
  );
});

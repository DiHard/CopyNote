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
      code: `globalThis.__probe = ${JSON.stringify(TRICKY)};`,
    },
    "assets/main.css": { type: "asset", source: "body { color: red; }" },
  };
}

// Everything the HTML parser or String.replace could trip over: a replacement
// pattern, a closing tag in two spellings, and a comment opener ahead of an
// opening tag — the pair that makes the parser ignore the real </script>.
const TRICKY = "$& </script> </SCRIPT > <!-- <script> -->";

test("frontend output is one HTML with intact inline JavaScript and CSS", () => {
  const output = bundle();
  singleFile().generateBundle({}, output);
  assert.deepEqual(Object.keys(output), ["index.html"]);
  const html = output["index.html"].source;
  assert.match(html, /<style>body \{ color: red; \}<\/style>/);
  assert.doesNotMatch(html, /\b(?:src|href)=/);

  const [, script, ...rest] = html.split(/<script type="module">|<\/script>/);
  assert.equal(rest.length, 1, "exactly one closing tag: the real one");
  assert.doesNotMatch(script, /<!--|<\/script/i);
  // The escaping must not change what the code does.
  new Function(script)();
  assert.equal(globalThis.__probe, TRICKY);
  delete globalThis.__probe;
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

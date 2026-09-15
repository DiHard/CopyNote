// Serves web/dist/index.html with the fake Go bridge injected, so the real
// frontend can be opened in a browser without launching copynote.exe.
// See README.md in this folder for why that matters.
//
//   node tools/uxharness/serve.mjs [port]
//
// Both files are re-read on every request: rebuild the frontend, refresh the
// page, no restart.

import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const bundlePath = join(repoRoot, "web", "dist", "index.html");
const stubPath = join(here, "stub.js");

const port = Number(process.argv[2] ?? process.env.PORT ?? 18099);

// A plain <script> in <head> runs before the bundle's own module script,
// which is deferred — so the globals exist by the time the UI boots.
function inject(html, stub) {
  const at = html.indexOf("<head>");
  if (at < 0) throw new Error("no <head> in the bundle — did vite-plugin-singlefile change?");
  const cut = at + "<head>".length;
  return html.slice(0, cut) + "\n<script>\n" + stub + "\n</script>\n" + html.slice(cut);
}

createServer(async (req, res) => {
  try {
    const [html, stub] = await Promise.all([
      readFile(bundlePath, "utf8"),
      readFile(stubPath, "utf8"),
    ]);
    res.writeHead(200, {
      "content-type": "text/html; charset=utf-8",
      "cache-control": "no-store",
    });
    res.end(inject(html, stub));
  } catch (err) {
    res.writeHead(500, { "content-type": "text/plain; charset=utf-8" });
    res.end(
      `${err.message}\n\n` +
        `Expected the built bundle at ${bundlePath}.\n` +
        `Run "npm run build" in web/ first.\n`,
    );
  }
}).listen(port, "127.0.0.1", () => {
  console.log(`harness on http://127.0.0.1:${port}/`);
  console.log("scenarios: ?empty  ?many  ?downloads  ?update  ?hotkeytaken  ?copybusy");
});

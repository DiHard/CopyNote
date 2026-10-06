// Downloads one file and writes it only if its SHA-256 is the one expected.
//
// The hash comes from tools/quality/versions.json, in the repository — not
// from a checksum list published next to the download, which whoever could
// replace the download could replace just as easily.
import { createHash } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import { basename, dirname } from "node:path";

const [url, output, expected] = process.argv.slice(2);
if (!url || !output || !/^[0-9a-f]{64}$/.test(expected ?? ""))
  throw new Error("Usage: download.mjs URL OUTPUT SHA256");

const response = await fetch(url);
if (!response.ok) throw new Error(`${url}: HTTP ${response.status}`);
const bytes = Buffer.from(await response.arrayBuffer());

const actual = createHash("sha256").update(bytes).digest("hex");
if (actual !== expected) {
  throw new Error(
    `${basename(output)}: SHA-256 is ${actual}, versions.json expects ${expected}`,
  );
}
await mkdir(dirname(output), { recursive: true });
await writeFile(output, bytes);

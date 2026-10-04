import { createHash } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import { basename, dirname } from "node:path";

const [url, output, checksumsUrl] = process.argv.slice(2);
if (!url || !output)
  throw new Error("Usage: download.mjs URL OUTPUT [CHECKSUMS_URL]");

async function fetchChecked(address) {
  const response = await fetch(address);
  if (!response.ok) throw new Error(`${address}: HTTP ${response.status}`);
  return response;
}

const bytes = Buffer.from(await (await fetchChecked(url)).arrayBuffer());
if (checksumsUrl) {
  const checksums = await (await fetchChecked(checksumsUrl)).text();
  const line = checksums
    .split(/\r?\n/)
    .find((value) => value.trim().split(/\s+/).at(-1) === basename(output));
  const expected = line?.trim().split(/\s+/)[0];
  const actual = createHash("sha256").update(bytes).digest("hex");
  if (!expected || expected !== actual)
    throw new Error(`Checksum mismatch: ${basename(output)}`);
}
await mkdir(dirname(output), { recursive: true });
await writeFile(output, bytes);

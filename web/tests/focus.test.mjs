import { test } from "node:test";
import assert from "node:assert/strict";
import { build } from "esbuild";

// focus.ts only ever touches getElementById / querySelectorAll / activeElement,
// so a hand-rolled document is enough to pin the navigation rules down.
const bundle = await build({
  entryPoints: ["src/lib/focus.ts"], bundle: true, write: false,
  format: "esm", platform: "browser", conditions: ["browser"],
});
let instance = 0;
const load = () =>
  import(`data:text/javascript;base64,${Buffer.from(bundle.outputFiles[0].text).toString("base64")}#${instance++}`);

function fakeDom(cardCount) {
  const doc = { activeElement: null };
  const search = { name: "search", focus() { doc.activeElement = search; } };
  const cards = Array.from({ length: cardCount }, (_, i) => ({
    name: `card${i}`,
    focus() { doc.activeElement = cards[i]; },
  }));
  doc.getElementById = (id) => (id === "entry-search" ? search : null);
  doc.querySelectorAll = (sel) => (sel === "[data-card-focus]" ? cards : []);
  globalThis.document = doc;
  return { doc, search, cards };
}

test("arrow-down from the search box lands on the first card", async () => {
  const { doc, search, cards } = fakeDom(3);
  const focus = await load();
  doc.activeElement = search;
  focus.moveCardFocus(1);
  assert.equal(doc.activeElement, cards[0]);
});

test("stepping off the top of the list returns to the search box", async () => {
  const { doc, search, cards } = fakeDom(3);
  const focus = await load();
  doc.activeElement = cards[0];
  focus.moveCardFocus(-1);
  assert.equal(doc.activeElement, search, "the user came from the search box");
});

test("the last card does not wrap around to the first", async () => {
  const { doc, cards } = fakeDom(3);
  const focus = await load();
  doc.activeElement = cards[2];
  focus.moveCardFocus(1);
  assert.equal(doc.activeElement, cards[2], "a held-down key must not jump to the start");
});

test("navigation walks the list one card at a time", async () => {
  const { doc, cards } = fakeDom(4);
  const focus = await load();
  doc.activeElement = cards[1];
  focus.moveCardFocus(1);
  assert.equal(doc.activeElement, cards[2]);
  focus.moveCardFocus(-1);
  assert.equal(doc.activeElement, cards[1]);
});

test("an empty list leaves focus alone", async () => {
  const { doc, search } = fakeDom(0);
  const focus = await load();
  doc.activeElement = search;
  focus.moveCardFocus(1);
  focus.focusCardAt(0);
  assert.equal(doc.activeElement, search);
});

test("focusCardAt clamps instead of throwing", async () => {
  const { doc, cards } = fakeDom(2);
  const focus = await load();
  focus.focusCardAt(99);
  assert.equal(doc.activeElement, cards[1]);
  focus.focusCardAt(-5);
  assert.equal(doc.activeElement, cards[0]);
});

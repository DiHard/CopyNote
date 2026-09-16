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

// nextTabStop reads a little more: the tab stops under a root, the attributes
// its selectors ask about, and closest() for buttons inside a card.
function fakeTabDom({ cards = 3, tabbableCard = 0, listButton = false, banner = false } = {}) {
  const doc = { activeElement: null };
  const el = (name, attrs = {}) => {
    const node = {
      name,
      attrs,
      tabIndex: 0,
      disabled: false,
      getClientRects: () => [{}],
      closest: (sel) => (sel === "[data-entry-id]" && attrs.card !== undefined ? node : null),
      focus() { doc.activeElement = node; },
    };
    return node;
  };
  const search = el("search");
  const header = ["pin", "new", "settings", "hide"].map((n) => el(n));
  const bannerButtons = banner ? ["move", "later"].map((n) => el(n)) : [];
  const cardButtons = Array.from({ length: cards }, (_, i) => {
    const b = el(`card${i}`, { card: i });
    b.tabIndex = i === tabbableCard ? 0 : -1;
    return b;
  });
  const edit = el("edit", { card: 0 });
  edit.tabIndex = -1;
  const add = listButton ? [el("add", { listFocus: true })] : [];
  const offscreen = el("display-none");
  offscreen.getClientRects = () => [];
  const all = [search, ...header, ...bannerButtons, ...cardButtons, edit, ...add, offscreen];
  const root = {
    querySelectorAll: () => all,
    querySelector: () =>
      all.find((n) => (n.attrs.card !== undefined && n.tabIndex === 0) || n.attrs.listFocus) ?? null,
  };
  doc.getElementById = (id) => (id === "entry-search" ? search : null);
  globalThis.document = doc;
  return { root, search, header, bannerButtons, cardButtons, edit, add, offscreen };
}

test("Tab from the search box reaches the list before the header buttons", async () => {
  const { root, search, header, cardButtons } = fakeTabDom();
  const focus = await load();
  assert.equal(focus.nextTabStop(root, search, false), cardButtons[0]);
  assert.equal(focus.nextTabStop(root, cardButtons[0], false), header[0]);
  assert.equal(focus.nextTabStop(root, header[0], true), cardButtons[0], "Shift+Tab walks the same order back");
  assert.equal(focus.nextTabStop(root, cardButtons[0], true), search);
});

test("Tab lands on the card that last had focus", async () => {
  const { root, search, cardButtons } = fakeTabDom({ tabbableCard: 2 });
  const focus = await load();
  assert.equal(focus.nextTabStop(root, search, false), cardButtons[2]);
});

test("the rest follows in document order and the order wraps", async () => {
  const { root, search, header, bannerButtons, cardButtons } = fakeTabDom({ banner: true });
  const focus = await load();
  const walk = [search];
  for (let i = 0; i < 8; i++) walk.push(focus.nextTabStop(root, walk[walk.length - 1], false));
  assert.deepEqual(
    walk.map((n) => n.name),
    [search, cardButtons[0], ...header, ...bannerButtons, search].map((n) => n.name),
  );
});

test("with no entries the empty screen's button takes the list's place", async () => {
  const { root, search, add } = fakeTabDom({ cards: 0, listButton: true });
  const focus = await load();
  assert.equal(focus.nextTabStop(root, search, false), add[0]);
});

test("Tab from a card's own button carries on after the card", async () => {
  const { root, header, edit } = fakeTabDom();
  const focus = await load();
  assert.equal(focus.nextTabStop(root, edit, false), header[0]);
});

test("focus outside the order is left to the browser", async () => {
  const { root, offscreen } = fakeTabDom();
  const focus = await load();
  assert.equal(focus.nextTabStop(root, offscreen, false), null);
  assert.equal(focus.nextTabStop(root, null, false), null);
});

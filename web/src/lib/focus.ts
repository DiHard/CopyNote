/**
 * Keyboard focus moves between the search box and the entry list, which live
 * in sibling components. Rather than thread handles through props, both
 * agree on a DOM contract and look each other up — the same approach the
 * drag code already takes with `[data-entry-id]`.
 *
 * `data-card-focus` marks a card's copy button; it is set in EntryCard.svelte.
 */

/** The search box in the header; set in Header.svelte. */
export const SEARCH_ID = "entry-search";

export function focusSearch(): void {
  document.getElementById(SEARCH_ID)?.focus();
}

function cards(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>("[data-card-focus]"));
}

/** Focuses the card at `index`, clamped to the list. No-op on an empty list. */
export function focusCardAt(index: number): void {
  const all = cards();
  if (all.length === 0) return;
  all[Math.max(0, Math.min(index, all.length - 1))].focus();
}

export function focusLastCard(): void {
  focusCardAt(cards().length - 1);
}

/**
 * Moves focus `delta` cards from the one that has it. Stepping off the top
 * returns to the search box, which is where the user came from; stepping off
 * the bottom stays put rather than wrapping, so a held-down key cannot
 * silently jump back to the start of the list.
 */
export function moveCardFocus(delta: number): void {
  const all = cards();
  if (all.length === 0) return;
  const current = all.indexOf(document.activeElement as HTMLElement);
  if (current < 0) {
    focusCardAt(delta >= 0 ? 0 : all.length - 1);
    return;
  }
  if (current + delta < 0) {
    focusSearch();
    return;
  }
  focusCardAt(current + delta);
}

/** Elements Tab may stop on; isTabStop drops the ones it skips. */
const TAB_STOP_CANDIDATES = "input, button, select, textarea, a[href], [tabindex]";

function isTabStop(el: HTMLElement): boolean {
  return el.tabIndex >= 0 && !(el as HTMLButtonElement).disabled && el.getClientRects().length > 0;
}

/**
 * The main view's Tab order: the search box, then the first visible card when
 * entering from search, or the card that last had focus when returning from
 * elsewhere — or the button the empty and no-match screens offer
 * (`data-list-focus`) — then everything else in document order, wrapping
 * around. The header buttons come before the list in the document, so plain
 * Tab went from the search box to "+", ⚙ and ✕ first; now a few letters, Tab
 * and Enter copy an entry.
 *
 * Returns null when focus is somewhere this order does not know, which leaves
 * Tab to the browser.
 */
export function nextTabStop(root: HTMLElement, from: Element | null, backwards: boolean): HTMLElement | null {
  const stops = Array.from(root.querySelectorAll<HTMLElement>(TAB_STOP_CANDIDATES)).filter(isTabStop);
  const search = document.getElementById(SEARCH_ID);
  const rememberedList = root.querySelector<HTMLElement>('[data-card-focus][tabindex="0"], [data-list-focus]');
  const firstCard = root.querySelector<HTMLElement>("[data-card-focus]");
  // Search is the fresh entry point: always start at the first visible card,
  // even if WebView2 restored focus to a lower card while showing the window.
  const list = from === search ? (firstCard ?? rememberedList) : rememberedList;
  const head = [search, list].filter((el): el is HTMLElement =>
    el !== null && (el === firstCard || stops.includes(el)),
  );
  const order = [...head, ...stops.filter((el) => !head.includes(el))];

  let at = order.indexOf(from as HTMLElement);
  // A card's edit and delete buttons are outside the order; Tab from one
  // carries on as if from the card.
  if (at < 0 && list && from?.closest?.("[data-entry-id]")) at = order.indexOf(list);
  if (at < 0) return null;
  return order[(at + (backwards ? order.length - 1 : 1)) % order.length];
}

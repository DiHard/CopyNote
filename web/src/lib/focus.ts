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

import type { EntryMenuRequest } from "./types";

/**
 * The entry context menu is drawn by Go — the tray icon's own popup — so it
 * can run past a window that is only a card or two tall. Opening it returns
 * at once; the choice comes back later through window.__entryMenuClosed.
 *
 * Every request carries a token. A menu replaced by a newer one resolves to
 * "" straight away, so Go's late answer for it cannot be taken for the new
 * menu's.
 */
let lastToken = 0;
let pending: { token: number; resolve: (id: string) => void } | null = null;

function settle(token: number, id: string): void {
  if (!pending || pending.token !== token) return;
  const { resolve } = pending;
  pending = null;
  resolve(id);
}

/** Resolves to the picked item's id, or "" when the menu was dismissed. */
export function showEntryMenu(request: Omit<EntryMenuRequest, "token">): Promise<string> {
  if (pending) settle(pending.token, "");
  const token = ++lastToken;
  window.__entryMenuClosed = settle;
  return new Promise((resolve) => {
    pending = { token, resolve };
    window.showEntryMenu({ ...request, token }).catch(() => settle(token, ""));
  });
}

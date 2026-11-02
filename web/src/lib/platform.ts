/**
 * Which desktop the page runs on. Read from the user agent rather than asked
 * of Go: WKWebView reports "Macintosh", and knowing it synchronously means
 * the very first render already uses the right words and keys.
 */
export const isMac: boolean =
  typeof navigator !== "undefined" &&
  /Macintosh|Mac OS X/.test(navigator.userAgent);

/** The modifier that moves an entry with ↑ ↓. On a Mac Ctrl+arrows switch
 *  Spaces before the page ever sees them, so it is ⌘ there. */
export function isMoveModifier(e: KeyboardEvent): boolean {
  return isMac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey;
}

/** Edit the focused entry: F2 everywhere, plus ⌘E on a Mac, whose laptops
 *  only reach F2 through fn. */
export function isEditKey(e: KeyboardEvent): boolean {
  return e.key === "F2" || (isMac && e.metaKey && e.code === "KeyE");
}

/** Delete the focused entry: Delete everywhere, plus ⌘⌫ on a Mac — Finder's
 *  key for it, and the only one a Mac laptop has. */
export function isDeleteKey(e: KeyboardEvent): boolean {
  return e.key === "Delete" || (isMac && e.metaKey && e.key === "Backspace");
}

/** Key names as each platform prints them, for menus and Settings. */
export const keyNames = isMac
  ? {
      enter: "↩",
      edit: "⌘E",
      delete: "⌘⌫",
      moveUp: "⌘↑",
      moveDown: "⌘↓",
      move: "⌘ ↑ ↓",
      escape: "⎋",
    }
  : {
      enter: "Enter",
      edit: "F2",
      delete: "Delete",
      moveUp: "Ctrl+↑",
      moveDown: "Ctrl+↓",
      move: "Ctrl + ↑ ↓",
      escape: "Esc",
    };

/** For aria-keyshortcuts, which spells keys the same way everywhere. */
export const ariaKeys = isMac
  ? { edit: "F2 Meta+E", delete: "Delete Meta+Backspace" }
  : { edit: "F2", delete: "Delete" };

const macModifiers: Record<string, string> = {
  Ctrl: "⌃",
  Alt: "⌥",
  Shift: "⇧",
  Win: "⌘",
};

/**
 * A stored global shortcut ("Ctrl+Alt+N") the way the platform writes it. On
 * a Mac that is "⌃⌥N", in Apple's modifier order; the stored text stays
 * Windows-shaped, so a backup moves between the two unchanged.
 */
export function displayHotkey(spec: string): string {
  if (!isMac) return spec;
  const parts = spec.split("+");
  const key = parts.pop() ?? "";
  const order = ["Ctrl", "Alt", "Shift", "Win"];
  const mods = order
    .filter((m) => parts.includes(m))
    .map((m) => macModifiers[m]);
  return mods.join("") + key;
}

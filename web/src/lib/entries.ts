import type { Entry } from "./types";
import { api } from "./api";
import { state } from "./appState.svelte";

/**
 * Case-insensitive substring match on label OR value.
 * Returns entries already sorted by order (server-side guarantee).
 */
export function filterEntries(entries: Entry[], query: string): Entry[] {
  const q = query.trim().toLowerCase();
  if (!q) return entries;
  return entries.filter(
    (e) =>
      e.label.toLowerCase().includes(q) || e.value.toLowerCase().includes(q),
  );
}

export async function refresh(): Promise<void> {
  state.loading = true;
  state.loadError = null;
  try {
    state.entries = await api.list();
  } catch (e) {
    state.loadError = String(e);
  } finally {
    state.loading = false;
  }
}

export async function createEntry(
  label: string,
  value: string,
): Promise<Entry> {
  const wasEmpty = state.entries.length === 0;
  const created = await api.create(label, value);
  // Server sets order=0 for the new entry and shifts existing ones.
  // Mirror locally: bump all existing orders by 1, prepend the new entry.
  state.entries = [
    created,
    ...state.entries.map((e) => ({ ...e, order: e.order + 1 })),
  ];
  if (wasEmpty) state.showFirstCopyHint = true;
  return created;
}

export async function updateEntry(
  id: string,
  label: string,
  value: string,
): Promise<Entry> {
  const updated = await api.update(id, label, value);
  // Replace the entry in-place, preserving order.
  state.entries = state.entries.map((e) => (e.id === id ? updated : e));
  return updated;
}

export async function deleteEntry(id: string): Promise<void> {
  await api.remove(id);
  // Remove locally and repack order values.
  state.entries = state.entries
    .filter((e) => e.id !== id)
    .map((e, i) => ({ ...e, order: i }));
}

/**
 * Turns a failed clipboard write into something the error line can phrase.
 * Go reports "OpenClipboard busy" once another program has held the
 * clipboard through all of its retries (clipboard.openClipboardWithRetry).
 */
export function describeCopyError(error: unknown): {
  busy: boolean;
  detail: string;
} {
  const detail = String(error)
    .replace(/^(Error:\s*)+/, "")
    .replace(/^clipboard:\s*/, "");
  return { busy: /OpenClipboard busy/.test(detail), detail };
}

/** Rejects when the clipboard write fails, after recording why in copyError. */
export async function copyEntry(id: string): Promise<void> {
  try {
    await api.copy(id);
  } catch (error) {
    state.copyError = describeCopyError(error);
    throw error;
  }
  state.copyError = null;
  // The hint has done its job the moment a copy succeeds.
  state.showFirstCopyHint = false;
}

export function dismissFirstCopyHint(): void {
  state.showFirstCopyHint = false;
}

/** Report a rejected native hide without leaving an unhandled event promise. */
export async function hideWindow(): Promise<void> {
  try {
    await window.hide();
  } catch (error) {
    state.operationError = String(error);
  }
}

/**
 * Copies whatever the current filter puts at the top of the list — the
 * "type a few letters, press Enter" path. Resolves false when nothing
 * matches or the clipboard write failed, so the caller can leave the window
 * open instead of hiding it on a no-op.
 */
export async function copyTopMatch(): Promise<boolean> {
  const [first] = filterEntries(state.entries, state.query);
  if (!first) return false;
  try {
    await copyEntry(first.id);
    return true;
  } catch {
    // copyEntry has recorded why; the window stays up to show it.
    return false;
  }
}

/**
 * The window is parked off-screen rather than destroyed, so reopening it
 * would otherwise show last session's search query, view, scroll position and
 * keyboard entry point. Called from Go after the window has been parked
 * off-screen. A modal remains intact, but its background list is reset too.
 */
export function resetAfterHide(): void {
  resetListPosition();
  if (state.modal) return;
  state.query = "";
  state.view = "main";
  state.operationError = null;
  state.copyError = null;
}

/**
 * The window is coming back on screen after having been parked.
 * resetAfterHide left the page clean at the time; this drops what has reached
 * it since — the parked window keeps the keyboard until the user clicks
 * elsewhere. The view is the caller's: Go knows which one it asked for.
 */
export function resetForShow(): void {
  if (state.modal) return;
  state.query = "";
  state.operationError = null;
  state.copyError = null;
}

/** Reset only the transient list position, without changing the current view. */
export function resetListPosition(): void {
  state.listResetToken++;
}

/**
 * Apply a new order to entries. Optimistically updates the local list
 * (so the UI feels instant) and reconciles with the server result. On
 * failure, falls back to a fresh refresh() so the UI cannot diverge
 * from disk.
 */
export async function reorderEntries(orderedIds: string[]): Promise<void> {
  const byId = new Map(state.entries.map((e) => [e.id, e]));
  const next: Entry[] = [];
  for (let i = 0; i < orderedIds.length; i++) {
    const e = byId.get(orderedIds[i]);
    if (!e) continue;
    next.push({ ...e, order: i });
  }
  if (next.length !== state.entries.length) {
    // Caller passed a malformed list; ignore and refresh instead.
    void refresh();
    return;
  }
  state.operationError = null;
  state.entries = next;
  try {
    await api.reorder(orderedIds);
  } catch (error) {
    await refresh();
    state.operationError = String(error);
  }
}

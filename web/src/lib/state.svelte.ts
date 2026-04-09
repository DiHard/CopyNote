import type { Entry, ModalState } from "./types";
import { api } from "./api";

// Single source of truth for the UI. All mutations go through the
// action functions below, which call the Go backend and mirror the
// server response into `state.entries`.
export const state = $state<{
  entries: Entry[];
  query: string;
  modal: ModalState;
  loading: boolean;
  loadError: string | null;
}>({
  entries: [],
  query: "",
  modal: null,
  loading: false,
  loadError: null,
});

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
  const created = await api.create(label, value);
  // Re-list to pick up the order shift applied server-side.
  state.entries = await api.list();
  return created;
}

export async function updateEntry(
  id: string,
  label: string,
  value: string,
): Promise<Entry> {
  const updated = await api.update(id, label, value);
  state.entries = await api.list();
  return updated;
}

export async function deleteEntry(id: string): Promise<void> {
  await api.remove(id);
  state.entries = await api.list();
}

export function openCreate(): void {
  state.modal = { kind: "create" };
}
export function openEdit(entry: Entry): void {
  state.modal = { kind: "edit", entry };
}
export function openDelete(entry: Entry): void {
  state.modal = { kind: "delete", entry };
}
export function closeModal(): void {
  state.modal = null;
}

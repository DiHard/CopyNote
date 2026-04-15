import type { Entry, ModalState, UserSettings, ViewMode } from "./types";
import { api } from "./api";
import { setLocale, systemLocale } from "./i18n";

// Single source of truth for the UI. All mutations go through the
// action functions below, which call the Go backend and mirror the
// server response into `state.entries`.
export const state = $state<{
  entries: Entry[];
  query: string;
  modal: ModalState;
  loading: boolean;
  loadError: string | null;
  view: ViewMode;
  settings: UserSettings;
}>({
  entries: [],
  query: "",
  modal: null,
  loading: false,
  loadError: null,
  view: "main",
  settings: { autorun: false, theme: "system", locale: "system", topmost: true },
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
  // Server sets order=0 for the new entry and shifts existing ones.
  // Mirror locally: bump all existing orders by 1, prepend the new entry.
  state.entries = [
    created,
    ...state.entries.map((e) => ({ ...e, order: e.order + 1 })),
  ];
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

export async function copyEntry(id: string): Promise<void> {
  await api.copy(id);
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

// ── View navigation ──────────────────────────────────────────────

export function openSettings(): void {
  state.modal = null;
  state.view = "settings";
}

export function closeSettings(): void {
  state.view = "main";
}

// ── Import / Export ──────────────────────────────────────────────

export async function exportData(): Promise<void> {
  await api.exportData();
}

export async function importData(): Promise<void> {
  await api.importData();
  // The Go side calls __refreshAfterImport via Eval after import
  // succeeds, which triggers refresh + loadSettings below.
}

/** Called from Go after a successful import to reload everything. */
export function refreshAfterImport(): void {
  void refresh();
  void loadSettings();
}

// ── Settings ─────────────────────────────────────────────────────

export async function loadSettings(): Promise<void> {
  try {
    state.settings = await api.getSettings();
  } catch {
    // Silently fall back to defaults; settings UI will still render.
  }
  applyTheme(state.settings.theme);
  applyLocale(state.settings.locale);
  // @ts-expect-error - injected by Go via webview.Bind
  window.applyTopmost?.(state.settings.topmost);
}

export async function saveSettings(
  patch: Partial<UserSettings>,
): Promise<void> {
  const merged = { ...state.settings, ...patch };
  await api.saveSettings(merged);
  state.settings = merged;
  if ("theme" in patch) {
    applyTheme(merged.theme);
  }
  if ("locale" in patch) {
    applyLocale(merged.locale);
  }
  if ("topmost" in patch) {
    // @ts-expect-error - injected by Go via webview.Bind
    window.applyTopmost?.(merged.topmost);
  }
}

function applyLocale(locale: string): void {
  if (locale === "system") {
    setLocale(systemLocale());
  } else {
    setLocale(locale);
  }
}

/**
 * Apply the theme to the document by toggling the `dark` class on
 * `<html>`. When set to "system", we follow the OS preference via
 * matchMedia. A listener is installed once to react to live OS
 * changes (e.g., Windows switching to/from dark mode while the app
 * is running).
 */
let systemDarkMQ: MediaQueryList | null = null;
let mqListener: ((e: MediaQueryListEvent) => void) | null = null;

function applyTheme(mode: string): void {
  // Clean up previous system listener if switching away from "system".
  if (mqListener && systemDarkMQ) {
    systemDarkMQ.removeEventListener("change", mqListener);
    mqListener = null;
  }

  if (mode === "dark") {
    document.documentElement.classList.add("dark");
  } else if (mode === "light") {
    document.documentElement.classList.remove("dark");
  } else {
    // "system" — follow OS preference.
    if (!systemDarkMQ) {
      systemDarkMQ = window.matchMedia("(prefers-color-scheme: dark)");
    }
    setFromMedia(systemDarkMQ.matches);
    mqListener = (e) => setFromMedia(e.matches);
    systemDarkMQ.addEventListener("change", mqListener);
  }
}

function setFromMedia(isDark: boolean): void {
  document.documentElement.classList.toggle("dark", isDark);
}

import type { Entry, ModalState, UpdateInfo, UpdateProgress, UserSettings, ViewMode } from "./types";
import { createTaskQueue } from "./taskQueue";
import { api } from "./api";
import { setLocale, systemLocale } from "./i18n";

// Single source of truth for the UI. All mutations go through the
// action functions below, which call the Go backend and mirror the
// server response into `state.entries`.
/** Result of the last manual "Check for updates" click. Used only by
 * the Settings view to show a transient status line. */
export type UpdateCheckStatus =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "upToDate" }
  | { kind: "available" }
  | { kind: "failed" };

/** Where an in-app update currently is. Failure keeps the running version. */
export type UpdateInstallStatus =
  | { kind: "idle" }
  | { kind: "downloading"; done: number; total: number }
  | { kind: "verifying" }
  | { kind: "applying" }
  | { kind: "restarting" }
  | { kind: "failed"; error: string };

export const state = $state<{
  entries: Entry[];
  query: string;
  modal: ModalState;
  loading: boolean;
  loadError: string | null;
  view: ViewMode;
  settings: UserSettings;
  settingsError: string | null;
  settingsPending: number;
  operationError: string | null;
  updateInfo: UpdateInfo | null;
  updateCheckStatus: UpdateCheckStatus;
  updateInstall: UpdateInstallStatus;
}>({
  entries: [],
  query: "",
  modal: null,
  loading: false,
  loadError: null,
  view: "main",
  settings: {
    autorun: false,
    theme: "system",
    locale: "system",
    topmost: true,
    disableUpdateCheck: false,
    lastSeenUpdateVersion: "",
  },
  settingsError: null,
  settingsPending: 0,
  operationError: null,
  updateInfo: null,
  updateCheckStatus: { kind: "idle" },
  updateInstall: { kind: "idle" },
});

/**
 * True when there is an update available AND the user has not yet
 * acknowledged it (by opening Settings for this version). Drives the
 * orange dot on the gear icon.
 */
export function hasUnseenUpdate(): boolean {
  return (
    state.updateInfo !== null &&
    state.updateInfo.version !== state.settings.lastSeenUpdateVersion
  );
}

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
  if (hasUnseenUpdate() && state.updateInfo) {
    void saveSettings({ lastSeenUpdateVersion: state.updateInfo.version }).catch(() => {});
  }
}

export function closeSettings(): void {
  state.view = "main";
}

// ── Import / Export ──────────────────────────────────────────────

// Import/export and preference writes share a queue so snapshots cannot overwrite each other.
const enqueueSettings = createTaskQueue();

export function exportData(): Promise<boolean> {
  return enqueueSettings(() => api.exportData());
}

export function importData(): Promise<boolean> {
  return enqueueSettings(async () => {
    const imported = await api.importData();
    if (imported) await Promise.all([refresh(), loadSettings()]);
    return imported;
  });
}

// ── Settings ─────────────────────────────────────────────────────

export async function loadSettings(): Promise<void> {
  try {
    state.settings = await api.getSettings();
    state.settingsError = null;
  } catch (error) {
    state.settingsError = String(error);
  }
  applyTheme(state.settings.theme);
  applyLocale(state.settings.locale);
  window.applyTopmost?.(state.settings.topmost);
}

export function saveSettings(patch: Partial<UserSettings>): Promise<void> {
  state.settingsPending++;
  return enqueueSettings(async () => {
    state.settingsError = null;
    // Read the latest successful state when this operation starts, not when queued.
    const merged = { ...state.settings, ...patch };
    try {
      await api.saveSettings(merged);
      state.settings = merged;
      applyTheme(merged.theme);
      applyLocale(merged.locale);
      await window.applyTopmost?.(merged.topmost);
    } catch (error) {
      state.settingsError = String(error);
      throw error;
    }
  }).finally(() => { state.settingsPending--; });
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

// ── Updates ──────────────────────────────────────────────────────

/**
 * Background check triggered at startup. Honors the
 * disableUpdateCheck preference on the Go side. Silently no-ops on
 * any error — update notifications are a nice-to-have, not critical.
 */
let updateRequest = 0;

export async function loadUpdateInfo(): Promise<void> {
  const request = ++updateRequest;
  try {
    const info = await api.checkForUpdates();
    if (request !== updateRequest) return;
    state.updateInfo = info;
    state.updateCheckStatus = state.updateInfo
      ? { kind: "available" }
      : { kind: "idle" };
  } catch {
    // Leave updateInfo as null. The UI renders nothing.
  }
}

/**
 * Manual check triggered by the "Check for updates" button in
 * Settings. Always hits the network, regardless of
 * disableUpdateCheck. Surfaces a per-invocation status so the UI can
 * show "checking / up to date / failed".
 */
export async function forceCheckUpdateInfo(): Promise<void> {
  const request = ++updateRequest;
  state.updateCheckStatus = { kind: "checking" };
  if (state.updateInstall.kind === "failed") state.updateInstall = { kind: "idle" };
  try {
    const info = await api.forceCheckForUpdates();
    if (request !== updateRequest) return;
    state.updateInfo = info;
    state.updateCheckStatus = info
      ? { kind: "available" }
      : { kind: "upToDate" };
  } catch {
    if (request !== updateRequest) return;
    state.updateCheckStatus = { kind: "failed" };
  }
}

// ── Self-update ──────────────────────────────────────────────────

export function isUpdateInstalling(): boolean {
  const kind = state.updateInstall.kind;
  return kind === "downloading" || kind === "verifying" || kind === "applying" || kind === "restarting";
}

/** How often download progress is polled from Go while installUpdate runs. */
const PROGRESS_POLL_MS = 250;

/**
 * Download, verify and swap in the release shown in `state.updateInfo`,
 * then restart the application. Progress is polled because the bridge
 * only reports completion. On failure the running version is untouched,
 * so the user can simply retry or download manually.
 */
export async function installUpdate(): Promise<void> {
  const info = state.updateInfo;
  if (!info?.selfUpdate || isUpdateInstalling()) return;
  state.updateInstall = { kind: "downloading", done: 0, total: info.size };

  let active = true;
  let timer: ReturnType<typeof setTimeout> | null = null;
  const poll = async () => {
    timer = null;
    if (!active) return;
    try {
      const progress = await api.updateProgress();
      if (active) applyInstallProgress(progress);
    } catch {
      // Progress is cosmetic; the install promise carries the result.
    }
    if (active) timer = setTimeout(() => void poll(), PROGRESS_POLL_MS);
  };
  timer = setTimeout(() => void poll(), PROGRESS_POLL_MS);
  const stop = () => {
    active = false;
    if (timer !== null) clearTimeout(timer);
  };

  try {
    await api.installUpdate();
    stop();
    state.updateInstall = { kind: "restarting" };
    await api.restartApp();
  } catch (error) {
    stop();
    state.updateInstall = { kind: "failed", error: String(error).replace(/^Error:\s*/, "") };
  }
}

function applyInstallProgress(progress: UpdateProgress): void {
  switch (progress.stage) {
    case "download":
      state.updateInstall = { kind: "downloading", done: progress.done, total: progress.total };
      break;
    case "verify":
      state.updateInstall = { kind: "verifying" };
      break;
    case "apply":
      state.updateInstall = { kind: "applying" };
      break;
    default:
      // "" means Go has finished; the install promise decides the outcome.
      break;
  }
}

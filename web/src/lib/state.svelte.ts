import type { Entry, ImportResult, InstallLocation, ModalState, UpdateInfo, UpdateProgress, UserSettings, ViewMode } from "./types";
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

/** Where the "move me somewhere permanent" action is. A move ends with
 * the process restarting, so "moving" has no success state to return to. */
export type RelocateStatus =
  | { kind: "idle" }
  | { kind: "moving" }
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
  installLocation: InstallLocation | null;
  relocate: RelocateStatus;
  /** Session-only: "remind me later" was clicked, so the banner is gone
   *  now regardless of whether the write has landed yet. */
  relocateSnoozeClicked: boolean;
  /** Session-only: the user just filled an empty list, so the hint about
   *  clicking a card is worth showing — that is the first moment it can
   *  actually be tried. Deliberately not persisted: no settings field to
   *  migrate, and installations that already have entries never see it. */
  showFirstCopyHint: boolean;
  /** Why Windows refused the global shortcut, if it did. Unlike the other
   *  preferences this one can fail outside the app's control. `taken` is the
   *  ordinary case — another program owns the combination — and gets a plain
   *  sentence; anything else falls back to Windows' own wording in `detail`.
   *  Kept structured rather than as a finished string so a language switch
   *  re-renders it. */
  hotkeyError: { combo: string; taken: boolean; detail: string } | null;
  /** Why the last copy failed, if it did. Another program holding the
   *  clipboard open is the ordinary case and gets a plain sentence; anything
   *  else keeps Go's wording in `detail`. Structured like hotkeyError, so a
   *  language switch re-renders it. */
  copyError: { busy: boolean; detail: string } | null;
  /** Bumps after the native window hides so EntryList can clear its DOM-only
   *  scroll and roving-focus position. */
  listResetToken: number;
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
    disableAutoHide: false,
    relocatePromptDismissed: false,
    relocateRemindAfter: "",
    hotkey: "",
    lastSeenUpdateVersion: "",
  },
  settingsError: null,
  settingsPending: 0,
  operationError: null,
  updateInfo: null,
  updateCheckStatus: { kind: "idle" },
  updateInstall: { kind: "idle" },
  installLocation: null,
  relocate: { kind: "idle" },
  relocateSnoozeClicked: false,
  showFirstCopyHint: false,
  hotkeyError: null,
  copyError: null,
  listResetToken: 0,
});

/** The combination Go falls back to when the preference is empty. */
export const DEFAULT_HOTKEY = "Ctrl+Alt+N";
export const HOTKEY_OFF = "off";

/** What to print for the stored preference. */
export function hotkeyLabel(): string {
  const h = state.settings.hotkey;
  return h === "" ? DEFAULT_HOTKEY : h;
}

/**
 * Registers the combination first and only stores it once Windows accepted
 * it — persisting a shortcut that does not work would leave the user with a
 * setting that lies. Another program owning the combination is the ordinary
 * failure here, not an exception.
 */
/**
 * Turns whatever the bridge threw into something Settings can phrase. The
 * combination comes from what was asked for, never from the error text — an
 * empty preference means the default and must not print as a blank.
 */
export function describeHotkeyError(
  spec: string,
  error: unknown,
): { combo: string; taken: boolean; detail: string } {
  const detail = String(error).replace(/^(Error:\s*)+/, "").replace(/\.\s*$/, "");
  return {
    combo: spec.trim() === "" ? DEFAULT_HOTKEY : spec,
    // ERROR_HOTKEY_ALREADY_REGISTERED is the refusal people actually hit.
    taken: /already registered/i.test(detail),
    detail,
  };
}

export async function applyHotkey(spec: string): Promise<void> {
  state.hotkeyError = null;
  try {
    await api.applyHotkey(spec);
  } catch (error) {
    state.hotkeyError = describeHotkeyError(spec, error);
    return;
  }
  await saveSettings({ hotkey: spec }).catch(() => {});
}

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
export function describeCopyError(error: unknown): { busy: boolean; detail: string } {
  const detail = String(error).replace(/^(Error:\s*)+/, "").replace(/^clipboard:\s*/, "");
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

// Two functions rather than one with a default argument: both call sites
// pass this straight to `onclick`, which would hand it a MouseEvent.
export function openCreate(): void {
  state.modal = { kind: "create", label: "" };
}

/** "Nothing found" offers to save what was typed, so the form starts filled. */
export function openCreateFromSearch(query: string): void {
  state.modal = { kind: "create", label: query.trim() };
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

/** Resolves to what the import did, or null when the file dialog was cancelled. */
export function importData(): Promise<ImportResult | null> {
  return enqueueSettings(async () => {
    const result = await api.importData();
    if (result) await Promise.all([refresh(), loadSettings()]);
    return result;
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
  window.applyAutoHide?.(!state.settings.disableAutoHide);
  // The tray already registered this at startup; re-applying is cheap and is
  // the only way a conflict detected back then reaches the Settings screen,
  // where the user can actually do something about it.
  window.applyHotkey?.(state.settings.hotkey).catch((error) => {
    state.hotkeyError = describeHotkeyError(state.settings.hotkey, error);
  });
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
      await window.applyAutoHide?.(!merged.disableAutoHide);
    } catch (error) {
      state.settingsError = String(error);
      throw error;
    }
  }).finally(() => { state.settingsPending--; });
}

/** Toggle the header pin, which is the quick way to turn auto-hide off/on. */
export function toggleAutoHide(): void {
  state.operationError = null;
  void saveSettings({ disableAutoHide: !state.settings.disableAutoHide }).catch((error) => {
    state.operationError = String(error);
  });
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

// ── Install location ───────────────────────────────────

export async function loadInstallLocation(): Promise<void> {
  try {
    state.installLocation = await api.getInstallLocation();
  } catch {
    // Nothing to offer if we cannot tell where we are running from.
    state.installLocation = null;
  }
}

/** True while the executable sits outside a program folder. Drives both
 *  the banner and the Settings button. */
export function canOfferRelocate(): boolean {
  const loc = state.installLocation;
  return loc !== null && loc.canRelocate && !loc.permanent;
}

/** The folder name alone — the full path is too long for 420 px and the
 *  name is what makes "you are running from Downloads" land. */
export function installFolderName(): string {
  const dir = state.installLocation?.dir ?? "";
  const parts = dir.split(/[\\/]/).filter(Boolean);
  return parts.length > 0 ? parts[parts.length - 1] : dir;
}

/** How many entries the user must have before the banner is worth showing.
 *  Raise it to hold the warning back further. */
const RELOCATE_BANNER_MIN_ENTRIES = 1;

/** True while "remind me later" is in effect — either the click just
 *  happened and the write may still be in flight, or the instant Go stored
 *  has not passed yet. An unparseable value counts as no snooze, so a
 *  corrupted setting shows the banner rather than hiding it forever. */
function relocateSnoozed(): boolean {
  if (state.relocateSnoozeClicked) return true;
  const until = Date.parse(state.settings.relocateRemindAfter);
  return Number.isFinite(until) && Date.now() < until;
}

/** The banner waits for the list to have something in it: the first thing a
 *  new user sees should explain what the app is for, not warn that Windows
 *  might delete it. It also respects both ways of saying no. Settings
 *  applies none of this, so nothing here locks the user out of the action. */
export function shouldShowRelocateBanner(): boolean {
  return (
    canOfferRelocate() &&
    !state.settings.relocatePromptDismissed &&
    !relocateSnoozed() &&
    // The first entry is also when the "click a card to copy" hint appears.
    // Stacking a disk-cleanup warning on top of the one lesson the app ever
    // teaches would drown it; the banner waits for the hint to retire.
    !state.showFirstCopyHint &&
    state.entries.length >= RELOCATE_BANNER_MIN_ENTRIES
  );
}

/** Copies the executable and restarts from the new location. Resolves
 *  only if the move failed — on success this window goes away. */
export async function relocateApp(targetDir = ""): Promise<void> {
  if (state.relocate.kind === "moving") return;
  state.relocate = { kind: "moving" };
  try {
    await api.relocateApp(targetDir);
  } catch (error) {
    state.relocate = {
      kind: "failed",
      error: String(error).replace(/^Error:\s*/, ""),
    };
  }
}

/** Asks for a folder first; a cancelled dialog changes nothing. */
export async function relocateAppTo(pickerTitle: string): Promise<void> {
  if (state.relocate.kind === "moving") return;
  let dir = "";
  try {
    dir = await api.pickInstallFolder(pickerTitle);
  } catch (error) {
    state.relocate = {
      kind: "failed",
      error: String(error).replace(/^Error:\s*/, ""),
    };
    return;
  }
  if (!dir) return;
  await relocateApp(dir);
}

/** "Don't offer again" — permanent, and the only one of the two that the
 *  user can never undo from the banner itself. */
export async function dismissRelocatePrompt(): Promise<void> {
  // Mirror locally first so the banner goes away even if the write fails.
  state.settings = { ...state.settings, relocatePromptDismissed: true };
  try {
    await api.dismissRelocatePrompt();
  } catch (error) {
    state.settingsError = String(error);
  }
}

/** "Remind me later" — Go owns how long "later" is and returns the instant
 *  it persisted. A failed write only means the banner is back next launch. */
export async function snoozeRelocatePrompt(): Promise<void> {
  state.relocateSnoozeClicked = true;
  try {
    const until = await api.snoozeRelocatePrompt();
    state.settings = { ...state.settings, relocateRemindAfter: until };
  } catch (error) {
    state.settingsError = String(error);
  }
}

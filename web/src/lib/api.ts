// Typed wrappers around the global functions injected by Go via webview.Bind.
// Each function returns a promise that rejects with an Error whose message
// matches the Go error returned by the bound method.

import type {
  Entry,
  EntryMenuRequest,
  ImportResult,
  InstallLocation,
  UpdateInfo,
  UpdateProgress,
  UserSettings,
  ViewMode,
} from "./types";

declare global {
  interface Window {
    list: () => Promise<Entry[]>;
    create: (label: string, value: string) => Promise<Entry>;
    update: (id: string, label: string, value: string) => Promise<Entry>;
    remove: (id: string) => Promise<null>;
    reorder: (orderedIds: string[]) => Promise<null>;
    copy: (id: string) => Promise<Entry>;
    hide: () => Promise<void>;
    resizeWindow: (contentHeight: number) => Promise<void>;
    windowPrepared?: (id: number, contentHeight: number) => Promise<void>;
    getSettings: () => Promise<UserSettings>;
    saveSettings: (settings: UserSettings) => Promise<void>;
    exportData: () => Promise<boolean>;
    /** Resolves to null when the user cancels the file dialog. */
    importData: () => Promise<ImportResult | null>;
    openExternal: (url: string) => Promise<void>;
    /** Opens the folder the running copynote.exe sits in, in Explorer. */
    openAppFolder: () => Promise<void>;
    notifyReady: () => Promise<void>;
    getVersion: () => Promise<string>;
    checkForUpdates: () => Promise<UpdateInfo | null>;
    forceCheckForUpdates: () => Promise<UpdateInfo | null>;
    /** Downloads, verifies and swaps in the release from the last check. */
    installUpdate: () => Promise<{ version: string }>;
    /** Snapshot of the running install; polled while installUpdate is pending. */
    updateProgress: () => Promise<UpdateProgress>;
    /** Quits and relaunches the replaced executable; only valid after installUpdate. */
    restartApp: () => Promise<void>;
    applyTopmost: (enabled: boolean) => Promise<void>;
    /** Whether losing focus parks the window off-screen. */
    applyAutoHide: (enabled: boolean) => Promise<void>;
    /** Registers the global shortcut; rejects when Windows refuses it. */
    applyHotkey: (spec: string) => Promise<void>;
    // Moving the executable into a program folder. Go registers these five
    // only while the feature is switched on (relocationEnabled in
    // relocate_windows.go), hence optional.
    /** Where the running executable lives and whether that is permanent. */
    getInstallLocation?: () => Promise<InstallLocation>;
    /** Shell folder browser; resolves to "" when the user cancels. */
    pickInstallFolder?: (title: string) => Promise<string>;
    /** Copies the exe into targetDir ("" = default) and restarts from there. */
    relocateApp?: (targetDir: string) => Promise<string>;
    dismissRelocatePrompt?: () => Promise<void>;
    /** Hides the move banner for a while; resolves to the RFC3339 instant
     *  Go stored, so the duration lives in one place only. */
    snoozeRelocatePrompt?: () => Promise<string>;
    /** Opens the native context menu for an entry and resolves once it is
     *  open; the choice arrives through __entryMenuClosed. */
    showEntryMenu: (request: EntryMenuRequest) => Promise<void>;
    /** Called by Go when that menu closes: the picked id, "" if dismissed. */
    __entryMenuClosed?: (token: number, id: string) => void;
    /** Injected at runtime by Go for tray→settings navigation. */
    __openSettings?: () => void;
    /** Called by Go as the window comes on screen. `view` is the view it was
     *  parked with, or null when it was on screen already. */
    __onShow?: (view: ViewMode | null) => void;
    /** Called by Go at the start/end of a native show or hide animation. */
    __onWindowTransition?: (active: boolean, generation: number) => void;
    /** Called by Go after the window is parked off-screen; the page answers
     *  through windowPrepared with the same id. */
    __onHide?: (id: number, settings: boolean) => void;
  }
}

export const api = {
  list: (): Promise<Entry[]> => window.list(),
  create: (label: string, value: string): Promise<Entry> =>
    window.create(label, value),
  update: (id: string, label: string, value: string): Promise<Entry> =>
    window.update(id, label, value),
  remove: (id: string): Promise<null> => window.remove(id),
  reorder: (orderedIds: string[]): Promise<null> => window.reorder(orderedIds),
  copy: (id: string): Promise<Entry> => window.copy(id),
  getSettings: (): Promise<UserSettings> => window.getSettings(),
  saveSettings: (s: UserSettings): Promise<void> => window.saveSettings(s),
  exportData: (): Promise<boolean> => window.exportData(),
  importData: (): Promise<ImportResult | null> => window.importData(),
  openAppFolder: (): Promise<void> => window.openAppFolder(),
  getVersion: (): Promise<string> => window.getVersion(),
  checkForUpdates: (): Promise<UpdateInfo | null> => window.checkForUpdates(),
  forceCheckForUpdates: (): Promise<UpdateInfo | null> =>
    window.forceCheckForUpdates(),
  installUpdate: (): Promise<{ version: string }> => window.installUpdate(),
  updateProgress: (): Promise<UpdateProgress> => window.updateProgress(),
  restartApp: (): Promise<void> => window.restartApp(),
  /** Resolves to null while moving the executable is switched off in Go. */
  getInstallLocation: async (): Promise<InstallLocation | null> =>
    (await window.getInstallLocation?.()) ?? null,
  pickInstallFolder: (title: string): Promise<string> =>
    window.pickInstallFolder?.(title) ?? relocationOff(),
  relocateApp: (targetDir: string): Promise<string> =>
    window.relocateApp?.(targetDir) ?? relocationOff(),
  dismissRelocatePrompt: (): Promise<void> =>
    window.dismissRelocatePrompt?.() ?? relocationOff(),
  snoozeRelocatePrompt: (): Promise<string> =>
    window.snoozeRelocatePrompt?.() ?? relocationOff(),
  applyHotkey: (spec: string): Promise<void> => window.applyHotkey(spec),
};

/**
 * These four are only reachable from the banner and the Settings card, and
 * those are only drawn for an install location — so with the feature off none
 * of this runs. If it ever does, the caller gets a rejection it already
 * handles rather than "undefined is not a function".
 */
function relocationOff(): Promise<never> {
  return Promise.reject(new Error("moving the program is switched off"));
}

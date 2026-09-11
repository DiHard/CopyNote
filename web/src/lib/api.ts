// Typed wrappers around the global functions injected by Go via webview.Bind.
// Each function returns a promise that rejects with an Error whose message
// matches the Go error returned by the bound method.

import type { Entry, UpdateInfo, UserSettings } from "./types";

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
    getSettings: () => Promise<UserSettings>;
    saveSettings: (settings: UserSettings) => Promise<void>;
    exportData: () => Promise<boolean>;
    importData: () => Promise<boolean>;
    openExternal: (url: string) => Promise<void>;
    notifyReady: () => Promise<void>;
    getVersion: () => Promise<string>;
    checkForUpdates: () => Promise<UpdateInfo | null>;
    forceCheckForUpdates: () => Promise<UpdateInfo | null>;
    applyTopmost: (enabled: boolean) => Promise<void>;
    /** Injected at runtime by Go for tray→settings navigation. */
    __openSettings?: () => void;
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
  importData: (): Promise<boolean> => window.importData(),
  getVersion: (): Promise<string> => window.getVersion(),
  checkForUpdates: (): Promise<UpdateInfo | null> => window.checkForUpdates(),
  forceCheckForUpdates: (): Promise<UpdateInfo | null> =>
    window.forceCheckForUpdates(),
};

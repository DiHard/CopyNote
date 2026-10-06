import type { ImportResult, UserSettings } from "./types";
import { api } from "./api";
import { state } from "./appState.svelte";
import { refresh } from "./entries";
import { createTaskQueue } from "./taskQueue";
import { activeLocale, setLocale, systemLocale } from "./i18n";

/** The combination Go falls back to when the preference is empty. */
export const DEFAULT_HOTKEY = "Ctrl+Alt+N";
export const HOTKEY_OFF = "off";

/** What to print for the stored preference. */
export function hotkeyLabel(): string {
  const h = state.settings.hotkey;
  return h === "" ? DEFAULT_HOTKEY : h;
}

/**
 * Turns whatever the bridge threw into something Settings can phrase. The
 * combination comes from what was asked for, never from the error text — an
 * empty preference means the default and must not print as a blank.
 */
export function describeHotkeyError(
  spec: string,
  error: unknown,
): { combo: string; taken: boolean; detail: string } {
  const detail = String(error)
    .replace(/^(Error:\s*)+/, "")
    .replace(/\.\s*$/, "");
  return {
    combo: spec.trim() === "" ? DEFAULT_HOTKEY : spec,
    // ERROR_HOTKEY_ALREADY_REGISTERED is the refusal people actually hit.
    taken: /already registered/i.test(detail),
    detail,
  };
}

/** Register and persist as one queued operation; restore on a failed write. */
export function applyHotkey(spec: string): Promise<void> {
  return queueSettingsChange(async () => {
    state.hotkeyError = null;
    const previous = state.settings.hotkey;
    try {
      await api.applyHotkey(spec);
    } catch (error) {
      state.hotkeyError = describeHotkeyError(spec, error);
      return;
    }
    try {
      await persistSettings({ hotkey: spec });
    } catch {
      try {
        await api.applyHotkey(previous);
      } catch (error) {
        state.hotkeyError = {
          ...describeHotkeyError(previous, `restore hotkey: ${String(error)}`),
          // Show the rollback context even when another program took the old key.
          taken: false,
        };
      }
      return;
    }
  });
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
  let loaded = false;
  try {
    publishSettings(await api.getSettings());
    state.settingsError = null;
    loaded = true;
  } catch (error) {
    state.settingsError = String(error);
    // Nothing was read: the built-in defaults are what is on screen.
    publishSettings(state.settings);
  }
  try {
    await applyWindowEffects(state.settings);
  } catch (error) {
    state.settingsError = String(error);
  }
  // Without the stored value there is nothing to re-apply: registering the
  // empty default here would replace whatever shortcut the tray registered
  // from the real settings at startup.
  if (!loaded) return;
  // The tray already registered this at startup; re-applying is cheap and is
  // the only way a conflict detected back then reaches the Settings screen,
  // where the user can actually do something about it.
  try {
    await window.applyHotkey?.(state.settings.hotkey);
    state.hotkeyError = null;
  } catch (error) {
    state.hotkeyError = describeHotkeyError(state.settings.hotkey, error);
  }
}

export function saveSettings(patch: Partial<UserSettings>): Promise<void> {
  return queueSettingsChange(async () => {
    await persistSettings(patch);
    try {
      await applyWindowEffects(state.settings);
    } catch (error) {
      state.settingsError = String(error);
      throw error;
    }
  });
}

function queueSettingsChange(operation: () => Promise<void>): Promise<void> {
  state.settingsPending++;
  return enqueueSettings(operation).finally(() => {
    state.settingsPending--;
  });
}

// Publishing and OS effects stay separate: an effect failing after the write
// must never cause us to roll back a shortcut whose preference is already saved.
async function persistSettings(patch: Partial<UserSettings>): Promise<void> {
  state.settingsError = null;
  const merged = { ...state.settings, ...patch };
  try {
    await api.saveSettings(merged);
  } catch (error) {
    state.settingsError = String(error);
    throw error;
  }
  publishSettings(merged);
}

/**
 * Makes `next` the current settings. Theme and language are part of the
 * same step: they are what the settings look like, they cannot fail, and
 * with an `await` between the store and them the UI got re-created for a new
 * language while the old one was still in effect.
 */
function publishSettings(next: UserSettings): void {
  state.settings = next;
  applyTheme(next.theme);
  applyLocale(next.locale);
}

/** What the native window does with the settings; each call can fail. */
async function applyWindowEffects(settings: UserSettings): Promise<void> {
  await window.applyTopmost?.(settings.topmost);
  await window.applyAutoHide?.(!settings.disableAutoHide);
}

/** Toggle the header pin, which is the quick way to turn auto-hide off/on. */
export function toggleAutoHide(): void {
  state.operationError = null;
  void saveSettings({ disableAutoHide: !state.settings.disableAutoHide }).catch(
    (error) => {
      state.operationError = String(error);
    },
  );
}

function applyLocale(locale: string): void {
  if (locale === "system") {
    setLocale(systemLocale());
  } else {
    setLocale(locale);
  }
  // t() reads a plain variable, so this is what tells the UI to redraw.
  state.appliedLocale = activeLocale();
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

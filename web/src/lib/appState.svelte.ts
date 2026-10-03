import type { Entry, InstallLocation, ModalState, UpdateInfo, UserSettings, ViewMode, UpdateCheckStatus, UpdateInstallStatus, RelocateStatus } from "./types";

// Shared reactive state; feature actions own persistence and OS effects.
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

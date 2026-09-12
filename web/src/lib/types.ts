export interface Entry {
  id: string;
  label: string;
  value: string;
  order: number;
  createdAt: string; // ISO 8601 UTC
  updatedAt: string; // ISO 8601 UTC
}

export type ModalState =
  | null
  /** `label` prefills the form — the empty-search action offers to save
   *  whatever was typed. Empty for a plain "new entry" click. */
  | { kind: "create"; label: string }
  | { kind: "edit"; entry: Entry }
  | { kind: "delete"; entry: Entry };

/** Matches Go model.Settings JSON shape. */
export interface UserSettings {
  autorun: boolean;
  theme: "light" | "dark" | "system";
  locale: string; // "en" | "ru" | "system"
  topmost: boolean;
  /** Inverted: true disables the update check. Default is false (enabled). */
  disableUpdateCheck: boolean;
  /** Inverted: true keeps the window on screen when another program takes
   *  focus. Default is false — the window hides. */
  disableAutoHide: boolean;
  /** Hides the banner offering to move the exe into a program folder for
   *  good. The action itself stays available in Settings. */
  relocatePromptDismissed: boolean;
  /** RFC3339 instant before which that banner stays hidden; "" when no
   *  snooze is running. Unlike the flag above, this one expires. */
  relocateRemindAfter: string;
  /** Global shortcut as the user sees it ("Ctrl+Alt+N"). "" means the
   *  built-in default, "off" disables it. */
  hotkey: string;
  /** Last release version acknowledged by the user. */
  lastSeenUpdateVersion: string;
}

/** Matches Go updater.ReleaseInfo JSON shape plus the bridge's selfUpdate flag. */
export interface UpdateInfo {
  version: string;     // "1.0.2" (no leading v)
  name: string;        // release title
  url: string;         // release page URL
  publishedAt: string; // RFC3339
  /** Byte size of the release binary; 0 when the release has no binary asset. */
  size: number;
  /** True when this installation can download, verify and swap the binary itself. */
  selfUpdate: boolean;
}

/** Matches the Go installLocation JSON shape. */
export interface InstallLocation {
  path: string;       // the running executable
  dir: string;        // the folder holding it
  /** True when dir is already a program folder, which hides the offer. */
  permanent: boolean;
  /** Where the one-click move puts it; "" when LOCALAPPDATA is unset. */
  defaultDir: string;
  /** False when the executable path could not be resolved at startup. */
  canRelocate: boolean;
}

/** Matches the Go updateProgress JSON shape polled during installUpdate. */
export interface UpdateProgress {
  stage: "" | "download" | "verify" | "apply";
  done: number;
  total: number;
}

/** Which top-level view is active. */
export type ViewMode = "main" | "settings";

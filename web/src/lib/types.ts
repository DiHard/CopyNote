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
  | { kind: "create" }
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
  /** Last release version acknowledged by the user. */
  lastSeenUpdateVersion: string;
}

/** Matches Go updater.ReleaseInfo JSON shape. */
export interface UpdateInfo {
  version: string;     // "1.0.2" (no leading v)
  name: string;        // release title
  url: string;         // release page URL
  publishedAt: string; // RFC3339
}

/** Which top-level view is active. */
export type ViewMode = "main" | "settings";

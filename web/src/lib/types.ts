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
}

/** Which top-level view is active. */
export type ViewMode = "main" | "settings";

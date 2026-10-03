import type { Entry } from "./types";
import { state } from "./appState.svelte";
import { saveSettings } from "./settings";
import { hasUnseenUpdate } from "./updates";

// Stable entry point for components; implementations live with their feature.
export { state } from "./appState.svelte";
export * from "./entries";
export * from "./settings";
export * from "./updates";
export * from "./relocation";
export type { UpdateCheckStatus, UpdateInstallStatus, RelocateStatus } from "./types";

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

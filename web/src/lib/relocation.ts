import { api } from "./api";
import { state } from "./appState.svelte";

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

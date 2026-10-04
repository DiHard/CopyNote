import type { UpdateProgress } from "./types";
import { api } from "./api";
import { state } from "./appState.svelte";

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
  if (state.updateInstall.kind === "failed")
    state.updateInstall = { kind: "idle" };
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
  return (
    kind === "downloading" ||
    kind === "verifying" ||
    kind === "applying" ||
    kind === "restarting"
  );
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
    state.updateInstall = {
      kind: "failed",
      error: String(error).replace(/^Error:\s*/, ""),
    };
  }
}

function applyInstallProgress(progress: UpdateProgress): void {
  switch (progress.stage) {
    case "download":
      state.updateInstall = {
        kind: "downloading",
        done: progress.done,
        total: progress.total,
      };
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

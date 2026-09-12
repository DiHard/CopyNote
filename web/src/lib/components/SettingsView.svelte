<script lang="ts">
  import { onMount } from "svelte";
  import {
    state as appState,
    closeSettings,
    saveSettings,
    exportData,
    importData,
    forceCheckUpdateInfo,
    installUpdate,
    isUpdateInstalling,
    canOfferRelocate,
    relocateApp,
    relocateAppTo,
  } from "../state.svelte";
  import { t, availableLocales } from "../i18n";
  import type { UserSettings } from "../types";
  import { api } from "../api";
  import Spinner from "./Spinner.svelte";

  let appVersion = $state("");

  onMount(async () => {
    try {
      appVersion = await api.getVersion();
    } catch {
      appVersion = "";
    }
  });

  const themeOptions = () => [
    { value: "system", label: t("settings.theme.system") },
    { value: "light", label: t("settings.theme.light") },
    { value: "dark", label: t("settings.theme.dark") },
  ];

  let dataStatus = $state<string | null>(null);
  let dataBusy = $state(false);

  function savePreference(patch: Partial<UserSettings>, event: Event, inverted = false) {
    const control = event.currentTarget as HTMLInputElement | HTMLSelectElement;
    void saveSettings(patch).catch(() => {
      const key = Object.keys(patch)[0] as keyof UserSettings;
      const value = appState.settings[key];
      if (control instanceof HTMLInputElement) control.checked = inverted ? !value : Boolean(value);
      else control.value = String(value);
    });
  }

  function onAutorunChange(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    savePreference({ autorun: checked }, e);
  }

  function onTopmostChange(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    savePreference({ topmost: checked }, e);
  }

  // Inverted against the persisted `disableAutoHide` so the label can be
  // positive, same as the update-check toggle below.
  function onAutoHideChange(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    savePreference({ disableAutoHide: !checked }, e, true);
  }

  function onThemeChange(e: Event) {
    const value = (e.target as HTMLSelectElement).value as
      | "light"
      | "dark"
      | "system";
    savePreference({ theme: value }, e);
  }

  function onLocaleChange(e: Event) {
    const value = (e.target as HTMLSelectElement).value;
    savePreference({ locale: value }, e);
  }

  // Auto-check toggle uses inverted semantics against the persisted
  // `disableUpdateCheck` field so the UX label is positive.
  function onAutoCheckChange(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    savePreference({ disableUpdateCheck: !checked }, e, true);
  }

  async function onCheckUpdates() {
    await forceCheckUpdateInfo();
  }

  function openReleasePage() {
    if (appState.updateInfo) {
      window.openExternal?.(appState.updateInfo.url);
    }
  }

  function onInstallUpdate() {
    void installUpdate();
  }

  /** Download percentage, or null when no download with a known size is running. */
  function downloadPercent(): number | null {
    const status = appState.updateInstall;
    if (status.kind !== "downloading" || status.total <= 0) return null;
    return Math.min(100, Math.floor((status.done / status.total) * 100));
  }

  /** The update button doubles as the status line while an install runs. */
  function installLabel(): string {
    switch (appState.updateInstall.kind) {
      case "downloading": {
        const percent = downloadPercent();
        return percent === null
          ? t("settings.updates.downloading")
          : t("settings.updates.downloadingPercent", { percent: String(percent) });
      }
      case "verifying":
        return t("settings.updates.verifying");
      case "applying":
        return t("settings.updates.applying");
      case "restarting":
        return t("settings.updates.restarting");
      default:
        return t("settings.updates.install");
    }
  }

  async function onExport() {
    if (dataBusy) return;
    dataBusy = true;
    dataStatus = null;
    try {
      if (await exportData()) dataStatus = t("settings.exportOk");
    } catch (e) {
      dataStatus = String(e);
    } finally { dataBusy = false; }
  }

  async function onImport() {
	if (dataBusy) return;
	dataBusy = true;
    dataStatus = null;
    try {
      if (await importData()) dataStatus = t("settings.importOk");
    } catch (e) {
      dataStatus = t("settings.importError", { error: String(e).replace(/^Error:\s*/, "") });
    } finally { dataBusy = false; }
  }

  let folderError = $state<string | null>(null);

  const relocating = $derived(appState.relocate.kind === "moving");
  const relocateError = $derived(
    appState.relocate.kind === "failed" ? appState.relocate.error : null,
  );

  async function onOpenAppFolder() {
    folderError = null;
    try {
      await api.openAppFolder();
    } catch (e) {
      folderError = t("settings.openFolderError", {
        error: String(e).replace(/^Error:\s*/, ""),
      });
    }
  }
</script>

<!-- Same shape as the main view: one viewport tall, only the body scrolls.
     h-full here resolved against a parent with no height, so the settings
     list used to push its own header off the top of the window. -->
<div data-shell class="flex h-screen flex-col bg-surface text-on-surface">
  <!-- Header -->
  <div class="flex shrink-0 items-center gap-1.5 border-b border-outline bg-surface-alt px-2.5 py-2">
    <button
      type="button"
      onclick={closeSettings}
      class="rounded-md p-1 text-on-surface-dim transition hover:bg-surface-hover hover:text-on-surface"
      title={t("settings.back")}
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="18"
        height="18"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="m15 18-6-6 6-6" />
      </svg>
    </button>
    <h1 class="text-sm font-semibold tracking-tight">{t("settings.title")}</h1>
  </div>

  <!-- Scrollable content -->
  <div data-scroller class="min-h-0 flex-1 overflow-y-auto">
   <div data-scroll-content class="space-y-5 px-3 py-3">
    {#if appState.settingsError}
      <p role="alert" class="text-xs text-danger">{t("settings.saveError", {error: appState.settingsError})}</p>
    {/if}
    <!-- General -->
    <section>
      <h2 class="mb-1.5 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.general")}
      </h2>
      <label
        class="flex cursor-pointer items-center justify-between rounded-lg border border-outline bg-card px-2.5 py-2"
      >
        <span class="text-sm">{t("settings.autorun")}</span>
        <input
          type="checkbox"
          disabled={appState.settingsPending > 0 || dataBusy}
          checked={appState.settings.autorun}
          onchange={onAutorunChange}
          class="h-4 w-4 cursor-pointer rounded border-input-border bg-input text-accent focus:ring-accent focus:ring-offset-0"
        />
      </label>
      <label
        class="mt-1.5 flex cursor-pointer items-start justify-between gap-3 rounded-lg border border-outline bg-card px-2.5 py-2"
      >
        <span class="min-w-0">
          <span class="block text-sm">{t("settings.autohide")}</span>
          <span class="mt-0.5 block text-[11px] leading-snug text-on-surface-dim"
            >{t("settings.autohide.hint")}</span
          >
        </span>
        <input
          type="checkbox"
          disabled={appState.settingsPending > 0 || dataBusy}
          checked={!appState.settings.disableAutoHide}
          onchange={onAutoHideChange}
          class="mt-0.5 h-4 w-4 shrink-0 cursor-pointer rounded border-input-border bg-input text-accent focus:ring-accent focus:ring-offset-0"
        />
      </label>
      <!-- Reads as a pair with the toggle above: turn hiding off and this is
           what keeps the window in front of the app being filled in. Both
           carry a real trade-off, so both name it — here a heavier shadow. -->
      <label
        class="mt-1.5 flex cursor-pointer items-start justify-between gap-3 rounded-lg border border-outline bg-card px-2.5 py-2"
      >
        <span class="min-w-0">
          <span class="block text-sm">{t("settings.topmost")}</span>
          <span class="mt-0.5 block text-[11px] leading-snug text-on-surface-dim"
            >{t("settings.topmost.hint")}</span
          >
        </span>
        <input
          type="checkbox"
          disabled={appState.settingsPending > 0 || dataBusy}
          checked={appState.settings.topmost}
          onchange={onTopmostChange}
          class="mt-0.5 h-4 w-4 shrink-0 cursor-pointer rounded border-input-border bg-input text-accent focus:ring-accent focus:ring-offset-0"
        />
      </label>
    </section>

    <!-- Appearance -->
    <section>
      <h2 class="mb-1.5 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.appearance")}
      </h2>
      <div class="space-y-1.5">
        <div
          class="flex items-center justify-between rounded-lg border border-outline bg-card px-2.5 py-2"
        >
          <span class="text-sm">{t("settings.theme")}</span>
          <select
            aria-label={t("settings.theme")}
            disabled={appState.settingsPending > 0 || dataBusy}
            value={appState.settings.theme}
            onchange={onThemeChange}
            class="cursor-pointer rounded-md border border-input-border bg-input px-2 py-1 text-sm text-on-surface focus:border-input-focus focus:outline-none"
          >
            {#each themeOptions() as opt}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
        </div>

        <div
          class="flex items-center justify-between rounded-lg border border-outline bg-card px-2.5 py-2"
        >
          <span class="text-sm">{t("settings.language")}</span>
          <select
            aria-label={t("settings.language")}
            disabled={appState.settingsPending > 0 || dataBusy}
            value={appState.settings.locale}
            onchange={onLocaleChange}
            class="cursor-pointer rounded-md border border-input-border bg-input px-2 py-1 text-sm text-on-surface focus:border-input-focus focus:outline-none"
          >
            {#each availableLocales as loc}
              <option value={loc.code}>{loc.label}</option>
            {/each}
          </select>
        </div>
      </div>
    </section>

    <!-- Data -->
    <section>
      <h2 class="mb-1.5 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.data")}
      </h2>
      <div class="flex gap-1.5">
        <button
          type="button"
          onclick={onImport}
          disabled={dataBusy || appState.settingsPending > 0 || isUpdateInstalling()}
          class="flex-1 rounded-lg border border-outline bg-card px-2.5 py-1.5 text-sm text-on-surface-dim transition hover:bg-card-hover hover:text-on-surface"
        >
          {t("settings.import")}
        </button>
        <button
          type="button"
          onclick={onExport}
          disabled={dataBusy || appState.settingsPending > 0 || isUpdateInstalling()}
          class="flex-1 rounded-lg border border-outline bg-card px-2.5 py-1.5 text-sm text-on-surface-dim transition hover:bg-card-hover hover:text-on-surface"
        >
          {t("settings.export")}
        </button>
      </div>
      {#if dataStatus}
        <p class="mt-1 text-[11px] text-on-surface-dim">{dataStatus}</p>
      {/if}
    </section>

    <!-- Updates -->
    <section>
      <h2 class="mb-1.5 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.updates.title")}
      </h2>
      <div class="space-y-1.5">
        {#if appState.updateInfo}
          <div class="rounded-lg border border-outline bg-card px-2.5 py-2">
            <div class="flex items-center justify-between gap-1.5">
              <span class="text-sm">
                {t("settings.updates.available", { version: appState.updateInfo.version })}
              </span>
              {#if appState.updateInfo.selfUpdate}
                <!-- One click downloads, verifies and swaps the binary, then
                     restarts. The label doubles as the status line. -->
                <button
                  type="button"
                  onclick={onInstallUpdate}
                  disabled={isUpdateInstalling() || dataBusy}
                  class="shrink-0 rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-accent-text transition hover:bg-accent-hover disabled:opacity-60"
                >
                  {installLabel()}
                </button>
              {:else}
                <!-- No signed binary in the release, or the exe folder is not
                     writable: fall back to the release page. -->
                <button
                  type="button"
                  onclick={openReleasePage}
                  class="shrink-0 rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-accent-text transition hover:bg-accent-hover"
                >
                  {t("settings.updates.download")}
                </button>
              {/if}
            </div>
            {#if downloadPercent() !== null}
              <div
                class="mt-1.5 h-1 overflow-hidden rounded-full bg-surface-hover"
                role="progressbar"
                aria-valuemin="0"
                aria-valuemax="100"
                aria-valuenow={downloadPercent()}
              >
                <div class="h-full rounded-full bg-accent transition-[width] duration-200" style:width="{downloadPercent()}%"></div>
              </div>
            {/if}
            {#if appState.updateInstall.kind === "failed"}
              <p role="alert" class="mt-1 text-[11px] text-danger">
                {t("settings.updates.installFailed", { error: appState.updateInstall.error })}
              </p>
              <button type="button" onclick={openReleasePage} class="mt-0.5 text-[11px] text-accent hover:underline">
                {t("settings.updates.manualDownload")}
              </button>
            {/if}
          </div>
        {/if}

        <!-- The button doubles as the status line. Text changes with
             updateCheckStatus to avoid layout shift from a separate
             status card. Clicking always triggers a fresh check. -->
        <button
          type="button"
          onclick={onCheckUpdates}
          disabled={appState.updateCheckStatus.kind === "checking"}
          class="w-full rounded-lg border border-outline bg-card px-2.5 py-1.5 text-sm text-on-surface-dim transition hover:bg-card-hover hover:text-on-surface disabled:opacity-60"
        >
          {#if appState.updateCheckStatus.kind === "checking"}
            {t("settings.updates.checking")}
          {:else if appState.updateCheckStatus.kind === "upToDate"}
            {t("settings.updates.upToDate")}
          {:else if appState.updateCheckStatus.kind === "failed"}
            {t("settings.updates.checkFailed")}
          {:else}
            {t("settings.updates.check")}
          {/if}
        </button>

        <label
          class="flex cursor-pointer items-center justify-between rounded-lg border border-outline bg-card px-2.5 py-2"
        >
          <span class="text-sm">{t("settings.updates.autoCheck")}</span>
          <input
            type="checkbox"
          disabled={appState.settingsPending > 0 || dataBusy}
            checked={!appState.settings.disableUpdateCheck}
            onchange={onAutoCheckChange}
            class="h-4 w-4 cursor-pointer rounded border-input-border bg-input text-accent focus:ring-accent focus:ring-offset-0"
          />
        </label>
      </div>
    </section>

    <!-- About -->
    <section>
      <h2 class="mb-1.5 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.about")}
      </h2>
      <div class="rounded-lg border border-outline bg-card px-2.5 py-1.5">
        <div class="flex items-baseline justify-between">
          <span class="text-sm font-medium">{t("app.title")}</span>
          <span class="text-[11px] text-on-surface-faint">{appVersion ? `v${appVersion} · ` : ""}MIT</span>
        </div>
        <div class="text-xs text-on-surface-faint">
          <button
            type="button"
            onclick={() => window.openExternal?.("https://github.com/DiHard/CopyNote")}
            class="cursor-pointer text-accent hover:underline"
          >github.com/DiHard/CopyNote</button>
        </div>
      </div>
      {#if canOfferRelocate()}
        <!-- Running from a download folder: offer to put it somewhere that
             survives a disk cleanup. Unlike the banner this ignores the
             dismissal, so dismissing never hides the action for good. -->
        <button
          type="button"
          onclick={() => void relocateApp()}
          disabled={relocating || dataBusy || isUpdateInstalling()}
          class="mt-1.5 flex w-full items-center justify-center gap-1.5 rounded-lg border border-outline bg-card px-2.5 py-1.5 text-sm text-on-surface-dim transition hover:bg-card-hover hover:text-on-surface disabled:opacity-60"
        >
          {#if relocating}<Spinner class="h-3.5 w-3.5" />{/if}
          {relocating ? t("relocate.moving") : t("relocate.settings")}
        </button>
        <div class="mt-1 flex items-baseline justify-between gap-2">
          <!-- While the move runs, where it is now matters less than the fact
               that the window is about to close itself. -->
          <p class="min-w-0 truncate text-[11px] text-on-surface-faint" title={appState.installLocation?.dir ?? ""}>
            {relocating
              ? t("relocate.willRestart")
              : t("relocate.currently", { dir: appState.installLocation?.dir ?? "" })}
          </p>
          <button
            type="button"
            onclick={() => void relocateAppTo(t("relocate.pickTitle"))}
            disabled={relocating}
            class="shrink-0 text-[11px] text-accent transition hover:underline disabled:opacity-60"
          >
            {t("relocate.choose")}
          </button>
        </div>
        {#if relocateError}
          <p role="alert" class="mt-1 text-[11px] text-danger">
            {t("relocate.failed", { error: relocateError })}
          </p>
        {/if}
      {/if}
      <!-- The app is portable: this folder holds the exe, and a self-update
           leaves its .old fallback here too. -->
      <button
        type="button"
        onclick={onOpenAppFolder}
        class="mt-1.5 w-full rounded-lg border border-outline bg-card px-2.5 py-1.5 text-sm text-on-surface-dim transition hover:bg-card-hover hover:text-on-surface"
      >
        {t("settings.openFolder")}
      </button>
      {#if folderError}
        <p role="alert" class="mt-1 text-[11px] text-danger">{folderError}</p>
      {/if}
    </section>
   </div>
  </div>
</div>

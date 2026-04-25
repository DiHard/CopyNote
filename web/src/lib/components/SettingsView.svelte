<script lang="ts">
  import { onMount } from "svelte";
  import {
    state as appState,
    closeSettings,
    saveSettings,
    exportData,
    importData,
    forceCheckUpdateInfo,
  } from "../state.svelte";
  import { t, availableLocales } from "../i18n";
  import { api } from "../api";

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

  function onAutorunChange(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    void saveSettings({ autorun: checked });
  }

  function onTopmostChange(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    void saveSettings({ topmost: checked });
  }

  function onThemeChange(e: Event) {
    const value = (e.target as HTMLSelectElement).value as
      | "light"
      | "dark"
      | "system";
    void saveSettings({ theme: value });
  }

  function onLocaleChange(e: Event) {
    const value = (e.target as HTMLSelectElement).value;
    void saveSettings({ locale: value });
  }

  // Auto-check toggle uses inverted semantics against the persisted
  // `disableUpdateCheck` field so the UX label is positive.
  function onAutoCheckChange(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    void saveSettings({ disableUpdateCheck: !checked });
  }

  async function onCheckUpdates() {
    await forceCheckUpdateInfo();
  }

  function openReleasePage() {
    if (appState.updateInfo) {
      window.openExternal?.(appState.updateInfo.url);
    }
  }

  async function onExport() {
    dataStatus = null;
    try {
      await exportData();
      dataStatus = t("settings.exportOk");
    } catch (e) {
      dataStatus = String(e);
    }
  }

  async function onImport() {
    dataStatus = null;
    try {
      await importData();
      dataStatus = t("settings.importOk");
    } catch (e) {
      dataStatus = t("settings.importError", { error: String(e).replace(/^Error:\s*/, "") });
    }
  }
</script>

<div class="flex h-full flex-col bg-surface text-on-surface">
  <!-- Header -->
  <div class="flex items-center gap-1.5 border-b border-outline bg-surface-alt px-2.5 py-2">
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
  <div class="flex-1 space-y-5 overflow-y-auto px-3 py-3">
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
          checked={appState.settings.autorun}
          onchange={onAutorunChange}
          class="h-4 w-4 cursor-pointer rounded border-input-border bg-input text-accent focus:ring-accent focus:ring-offset-0"
        />
      </label>
      <label
        class="mt-1.5 flex cursor-pointer items-center justify-between rounded-lg border border-outline bg-card px-2.5 py-2"
      >
        <span class="text-sm">{t("settings.topmost")}</span>
        <input
          type="checkbox"
          checked={appState.settings.topmost}
          onchange={onTopmostChange}
          class="h-4 w-4 cursor-pointer rounded border-input-border bg-input text-accent focus:ring-accent focus:ring-offset-0"
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
          class="flex-1 rounded-lg border border-outline bg-card px-2.5 py-1.5 text-sm text-on-surface-dim transition hover:bg-card-hover hover:text-on-surface"
        >
          {t("settings.import")}
        </button>
        <button
          type="button"
          onclick={onExport}
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
              <button
                type="button"
                onclick={openReleasePage}
                class="shrink-0 rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-accent-text transition hover:bg-accent-hover"
              >
                {t("settings.updates.download")}
              </button>
            </div>
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
          <!-- svelte-ignore a11y_missing_attribute a11y_no_static_element_interactions a11y_click_events_have_key_events -->
          <span
            role="link"
            onclick={() => window.openExternal?.("https://github.com/DiHard/CopyNote")}
            class="cursor-pointer text-accent hover:underline"
          >github.com/DiHard/CopyNote</span>
        </div>
      </div>
    </section>
  </div>
</div>

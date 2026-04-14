<script lang="ts">
  import { state, closeSettings, saveSettings } from "../state.svelte";
  import { t, availableLocales } from "../i18n";

  const themeOptions = () => [
    { value: "system", label: t("settings.theme.system") },
    { value: "light", label: t("settings.theme.light") },
    { value: "dark", label: t("settings.theme.dark") },
  ];

  function onAutorunChange(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    void saveSettings({ autorun: checked });
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
</script>

<div class="flex h-full flex-col bg-surface text-on-surface">
  <!-- Header -->
  <div class="flex items-center gap-2 border-b border-outline bg-surface-alt px-3 py-2.5">
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
  <div class="flex-1 space-y-6 overflow-y-auto px-4 py-4">
    <!-- General -->
    <section>
      <h2 class="mb-2 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.general")}
      </h2>
      <label
        class="flex cursor-pointer items-center justify-between rounded-lg border border-outline bg-card px-3 py-2.5"
      >
        <span class="text-sm">{t("settings.autorun")}</span>
        <input
          type="checkbox"
          checked={state.settings.autorun}
          onchange={onAutorunChange}
          class="h-4 w-4 cursor-pointer rounded border-input-border bg-input text-accent focus:ring-accent focus:ring-offset-0"
        />
      </label>
    </section>

    <!-- Appearance -->
    <section>
      <h2 class="mb-2 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.appearance")}
      </h2>
      <div class="space-y-2">
        <div
          class="flex items-center justify-between rounded-lg border border-outline bg-card px-3 py-2.5"
        >
          <span class="text-sm">{t("settings.theme")}</span>
          <select
            value={state.settings.theme}
            onchange={onThemeChange}
            class="cursor-pointer rounded-md border border-input-border bg-input px-2 py-1 text-sm text-on-surface focus:border-input-focus focus:outline-none"
          >
            {#each themeOptions() as opt}
              <option value={opt.value}>{opt.label}</option>
            {/each}
          </select>
        </div>

        <div
          class="flex items-center justify-between rounded-lg border border-outline bg-card px-3 py-2.5"
        >
          <span class="text-sm">{t("settings.language")}</span>
          <select
            value={state.settings.locale}
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
      <h2 class="mb-2 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.data")}
      </h2>
      <div class="flex gap-2">
        <button
          type="button"
          disabled
          class="flex-1 rounded-lg border border-outline bg-card px-3 py-2 text-sm text-on-surface-dim transition hover:bg-card-hover hover:text-on-surface disabled:cursor-not-allowed disabled:opacity-50"
        >
          {t("settings.import")}
        </button>
        <button
          type="button"
          disabled
          class="flex-1 rounded-lg border border-outline bg-card px-3 py-2 text-sm text-on-surface-dim transition hover:bg-card-hover hover:text-on-surface disabled:cursor-not-allowed disabled:opacity-50"
        >
          {t("settings.export")}
        </button>
      </div>
      <p class="mt-1.5 text-[11px] text-on-surface-faint">{t("settings.comingSoon")}</p>
    </section>

    <!-- About -->
    <section>
      <h2 class="mb-2 text-[11px] font-semibold uppercase tracking-widest text-on-surface-faint">
        {t("settings.about")}
      </h2>
      <div class="rounded-lg border border-outline bg-card px-3 py-2">
        <div class="flex items-baseline justify-between">
          <span class="text-sm font-medium">{t("app.title")}</span>
          <span class="text-[11px] text-on-surface-faint">v1.0.0 · MIT</span>
        </div>
        <div class="text-xs text-on-surface-faint">
          <!-- svelte-ignore a11y_missing_attribute -->
          <a
            class="text-accent hover:underline"
            onclick={(e) => e.preventDefault()}
          >github.com/DiHard/CopyNote</a>
        </div>
      </div>
    </section>
  </div>
</div>

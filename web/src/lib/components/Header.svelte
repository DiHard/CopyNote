<script lang="ts">
  import { state, openCreate, openSettings } from "../state.svelte";
  import { t } from "../i18n";

  function hideWindow() {
    window.hide();
  }
</script>

<header
  class="flex items-center gap-1.5 border-b border-outline bg-surface-alt px-2.5 py-1.5"
  style="-webkit-app-region: drag"
>
  <span class="shrink-0 text-xs font-semibold text-on-surface">{t("app.title")}</span>

  <div class="relative flex-1" style="-webkit-app-region: no-drag">
    <svg
      xmlns="http://www.w3.org/2000/svg"
      class="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-on-surface-faint"
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <circle cx="11" cy="11" r="8"></circle>
      <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
    </svg>
    <input
      type="text"
      bind:value={state.query}
      placeholder={t("search.placeholder")}
      class="w-full rounded border border-input-border bg-input py-1 pl-7 pr-2 text-xs text-on-surface placeholder:text-on-surface-faint focus:border-input-focus focus:outline-none"
    />
  </div>

  <div class="flex shrink-0 items-center gap-0.5" style="-webkit-app-region: no-drag">
    <button
      type="button"
      onclick={openCreate}
      title={t("header.new")}
      class="rounded p-1 text-on-surface-dim transition hover:bg-surface-hover hover:text-on-surface"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <line x1="12" y1="5" x2="12" y2="19"></line>
        <line x1="5" y1="12" x2="19" y2="12"></line>
      </svg>
    </button>

    <button
      type="button"
      onclick={openSettings}
      title={t("header.settings")}
      class="rounded p-1 text-on-surface-dim transition hover:bg-surface-hover hover:text-on-surface"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
        <circle cx="12" cy="12" r="3" />
      </svg>
    </button>

    <button
      type="button"
      onclick={hideWindow}
      title={t("header.hide")}
      class="rounded p-1 text-on-surface-dim transition hover:bg-surface-hover hover:text-on-surface"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <line x1="18" y1="6" x2="6" y2="18"></line>
        <line x1="6" y1="6" x2="18" y2="18"></line>
      </svg>
    </button>
  </div>
</header>

<script lang="ts">
  import { state, openCreate, openSettings, hasUnseenUpdate, copyTopMatch, toggleAutoHide } from "../state.svelte";
  import { SEARCH_ID, focusCardAt } from "../focus";
  import { t } from "../i18n";

  function hideWindow() {
    window.hide();
  }

  /**
   * Closes the keyboard loop the app is built around: open, type a few
   * letters, press Enter, paste. Arrow-down hands off to the list for the
   * cases where the top match is not the one wanted.
   */
  async function onSearchKeydown(e: KeyboardEvent) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      focusCardAt(0);
      return;
    }
    if (e.key === "Escape" && state.query !== "") {
      // Clear the filter first; a second Escape falls through to App's
      // global handler and hides the window.
      e.preventDefault();
      state.query = "";
      return;
    }
    if (e.key === "Enter") {
      e.preventDefault();
      // A failed or empty copy leaves the window up, with the error shown.
      if (await copyTopMatch()) window.hide();
    }
  }
</script>

<header
  class="flex shrink-0 items-center gap-1.5 border-b border-outline bg-surface-alt px-2.5 py-1.5"
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
      id={SEARCH_ID}
      type="text"
      bind:value={state.query}
      onkeydown={onSearchKeydown}
      aria-label={t("search.placeholder")}
      placeholder={t("search.placeholder")}
      class="w-full rounded border border-input-border bg-input py-1 pl-7 pr-2 text-xs text-on-surface placeholder:text-on-surface-faint"
    />
  </div>

  <div class="flex shrink-0 items-center gap-0.5" style="-webkit-app-region: no-drag">
    <button
      type="button"
      onclick={toggleAutoHide}
      disabled={state.settingsPending > 0}
      data-tooltip={state.settings.disableAutoHide ? t("header.unpin") : t("header.pin")}
      aria-label={state.settings.disableAutoHide ? t("header.unpin") : t("header.pin")}
      aria-pressed={state.settings.disableAutoHide}
      class="rounded p-1.5 transition disabled:cursor-wait disabled:opacity-60 {state.settings.disableAutoHide
        ? 'bg-accent-soft text-accent hover:bg-accent-soft'
        : 'text-on-surface-dim hover:bg-surface-hover hover:text-on-surface'}"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 17v5"></path>
        <path d="M5 17h14"></path>
        <path d="M17 3H7l2 6-4 4v2h14v-2l-4-4 2-6Z"></path>
      </svg>
    </button>

    <button
      type="button"
      onclick={openCreate}
      data-tooltip={t("header.new")}
      aria-label={t("header.new")}
      class="rounded p-1.5 text-on-surface-dim transition hover:bg-surface-hover hover:text-on-surface"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <line x1="12" y1="5" x2="12" y2="19"></line>
        <line x1="5" y1="12" x2="19" y2="12"></line>
      </svg>
    </button>

    <button
      type="button"
      onclick={openSettings}
      data-tooltip={t("header.settings")}
      aria-label={t("header.settings")}
      class="relative rounded p-1.5 text-on-surface-dim transition hover:bg-surface-hover hover:text-on-surface"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
        <circle cx="12" cy="12" r="3" />
      </svg>
      {#if hasUnseenUpdate()}
        <span
          class="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-update-dot"
          aria-label={t("header.updateAvailable")}
        ></span>
      {/if}
    </button>

    <button
      type="button"
      onclick={hideWindow}
      data-tooltip={t("header.hide")}
      aria-label={t("header.hide")}
      class="rounded p-1.5 text-on-surface-dim transition hover:bg-surface-hover hover:text-on-surface"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <line x1="18" y1="6" x2="6" y2="18"></line>
        <line x1="6" y1="6" x2="18" y2="18"></line>
      </svg>
    </button>
  </div>
</header>

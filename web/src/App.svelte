<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import {
    state as appState,
    refresh,
    loadSettings,
    openSettings,
    closeSettings,
    refreshAfterImport,
    filterEntries,
  } from "./lib/state.svelte";
  import Header from "./lib/components/Header.svelte";
  import EntryList from "./lib/components/EntryList.svelte";
  import EntryModal from "./lib/components/EntryModal.svelte";
  import ConfirmModal from "./lib/components/ConfirmModal.svelte";
  import SettingsView from "./lib/components/SettingsView.svelte";

  onMount(async () => {
    window.__openSettings = openSettings;
    window.__refreshAfterImport = refreshAfterImport;
    await Promise.all([refresh(), loadSettings()]);
    // Signal Go that the UI is ready — stops tray icon pulse
    // and enables LMB click.
    window.notifyReady?.();
  });

  onDestroy(() => {
    delete window.__openSettings;
    delete window.__refreshAfterImport;
  });

  // ── Auto-resize window to fit content ──────────────────────────
  // Measure actual DOM scrollHeight after Svelte flushes. Fixed/
  // absolute elements (modals) don't contribute to scrollHeight, so
  // we enforce a minimum when a modal is open.

  const MIN_H = 80;
  const MODAL_MIN_H = 420;
  const SETTINGS_MIN_H = 480;
  const EASE = 0.35; // move 35% of remaining distance per frame (fast but smooth)

  let currentH = 0;
  let targetH = 0;
  let rafId: number | null = null;

  function animateStep() {
    const diff = targetH - currentH;
    if (Math.abs(diff) < 1) {
      currentH = targetH;
      window.resizeWindow?.(Math.round(currentH));
      rafId = null;
      return;
    }
    currentH += diff * EASE;
    window.resizeWindow?.(Math.round(currentH));
    rafId = requestAnimationFrame(animateStep);
  }

  function smoothResize(h: number) {
    targetH = h;
    if (currentH === 0) {
      // Very first call — jump instantly, no animation.
      currentH = h;
      window.resizeWindow?.(Math.round(h));
      return;
    }
    // Both expanding and shrinking use smooth animation.
    // The window background brush (#f3f3f3) fills any briefly
    // exposed area during expansion, preventing black gaps.
    if (!rafId) {
      rafId = requestAnimationFrame(animateStep);
    }
  }

  onDestroy(() => {
    if (rafId) cancelAnimationFrame(rafId);
  });

  $effect(() => {
    // Touch reactive deps so the effect re-runs when they change.
    void appState.view;
    void appState.entries;
    void appState.query;
    void appState.loading;
    const modal = appState.modal;

    const view = appState.view;
    void tick().then(() => {
      // Wait one extra frame so the browser finishes layout after
      // Svelte's DOM update — prevents measuring stale scrollHeight
      // when switching views (e.g., opening settings from tray menu).
      requestAnimationFrame(() => {
        const root = document.getElementById("app");
        if (!root) return;
        let h = Math.max(MIN_H, root.scrollHeight);
        if (modal) {
          h = Math.max(h, MODAL_MIN_H);
        }
        if (view === "settings") {
          h = Math.max(h, SETTINGS_MIN_H);
        }
        smoothResize(h);
      });
    });
  });

  // Delay modal rendering so the window resize animation completes
  // before the overlay + modal card appear. Without this, the user
  // briefly sees the overlay on an undersized window.
  let showModal = $state(false);
  let modalTimer: number | null = null;

  $effect(() => {
    const modal = appState.modal;
    if (modal) {
      // Schedule modal show after resize has time to finish.
      modalTimer = window.setTimeout(() => { showModal = true; }, 180);
    } else {
      if (modalTimer) { clearTimeout(modalTimer); modalTimer = null; }
      showModal = false;
    }
  });

  /** Global keyboard shortcuts. */
  function onGlobalKeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && !appState.modal) {
      e.preventDefault();
      if (appState.view === "settings") {
        closeSettings();
      } else {
        window.hide();
      }
    }
  }
</script>

<svelte:window onkeydown={onGlobalKeydown} />

<!-- Re-key the entire UI when locale changes so every t() call
     re-evaluates. Slightly heavy but simple and correct. -->
{#key appState.settings.locale}
  {#if appState.view === "settings"}
    <SettingsView />
  {:else}
    <main class="flex flex-col bg-surface text-on-surface">
      <Header />
      <EntryList />
    </main>

    {#if showModal && appState.modal?.kind === "create"}
      <EntryModal />
    {:else if showModal && appState.modal?.kind === "edit"}
      <EntryModal entry={appState.modal.entry} />
    {:else if showModal && appState.modal?.kind === "delete"}
      <ConfirmModal entry={appState.modal.entry} />
    {/if}
  {/if}
{/key}

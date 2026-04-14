<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import {
    state,
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
  const EASE = 0.25; // move 25% of remaining distance per frame

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
    if (currentH === 0 || h > currentH) {
      // First call or EXPANDING — jump instantly to prevent black
      // gap at the bottom (unpainted pixels before WebView2 redraws).
      if (rafId) { cancelAnimationFrame(rafId); rafId = null; }
      currentH = h;
      window.resizeWindow?.(Math.round(h));
      return;
    }
    // SHRINKING — animate for smooth visual.
    if (!rafId) {
      rafId = requestAnimationFrame(animateStep);
    }
  }

  onDestroy(() => {
    if (rafId) cancelAnimationFrame(rafId);
  });

  $effect(() => {
    // Touch reactive deps so the effect re-runs when they change.
    void state.view;
    void state.entries;
    void state.query;
    void state.loading;
    const modal = state.modal;

    void tick().then(() => {
      const root = document.getElementById("app");
      if (!root) return;
      let h = Math.max(MIN_H, root.scrollHeight);
      // Fixed/absolute modals don't affect scrollHeight — enforce min.
      if (modal) {
        h = Math.max(h, MODAL_MIN_H);
      }
      smoothResize(h);
    });
  });

  /** Global keyboard shortcuts. */
  function onGlobalKeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && !state.modal) {
      e.preventDefault();
      if (state.view === "settings") {
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
{#key state.settings.locale}
  {#if state.view === "settings"}
    <SettingsView />
  {:else}
    <main class="flex flex-col bg-surface text-on-surface">
      <Header />
      <EntryList />
    </main>

    {#if state.modal?.kind === "create"}
      <EntryModal />
    {:else if state.modal?.kind === "edit"}
      <EntryModal entry={state.modal.entry} />
    {:else if state.modal?.kind === "delete"}
      <ConfirmModal entry={state.modal.entry} />
    {/if}
  {/if}
{/key}

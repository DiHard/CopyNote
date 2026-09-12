<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import {
    state as appState,
    refresh,
    loadSettings,
    openSettings,
    closeSettings,
    loadUpdateInfo,
    loadInstallLocation,
    resetForShow,
  } from "./lib/state.svelte";
  import { focusSearch } from "./lib/focus";
  import { t } from "./lib/i18n";
  import Header from "./lib/components/Header.svelte";
  import EntryList from "./lib/components/EntryList.svelte";
  import EntryModal from "./lib/components/EntryModal.svelte";
  import ConfirmModal from "./lib/components/ConfirmModal.svelte";
  import SettingsView from "./lib/components/SettingsView.svelte";

  /**
   * Go calls this every time the window comes back on screen. A modal is
   * left completely alone: it may hold an edit the user was pulled away
   * from mid-sentence, and discarding that to tidy the view would be worse
   * than a stale search box.
   */
  async function onWindowShown() {
    if (appState.modal) return;
    resetForShow();
    // The settings view may have just been swapped out; the search box does
    // not exist until Svelte has flushed.
    await tick();
    focusSearch();
  }

  onMount(async () => {
    window.__openSettings = openSettings;
    window.__onShow = onWindowShown;
    await Promise.all([refresh(), loadSettings()]);
    // Signal Go that the UI is ready — stops tray icon pulse
    // and enables LMB click.
    window.notifyReady?.();
    // Background update check — fire-and-forget, runs after the UI is
    // already interactive so it never blocks startup.
    void loadUpdateInfo();
    // Where the exe lives decides whether the relocate banner shows.
    void loadInstallLocation();
  });

  onDestroy(() => {
    delete window.__openSettings;
    delete window.__onShow;
  });

  // ── Auto-resize window to fit content ──────────────────────────
  // The shell is exactly one viewport tall and the entry list is the only
  // thing that scrolls, so the height we ask Go for cannot be read off the
  // shell — it would just report the size we already have. Instead we add
  // up the fixed rows and the list's *content*, which is the height the
  // window would need to show everything. Go clamps that to the work area;
  // past the clamp the list simply scrolls inside a full-height window.
  //
  // Fixed/absolute elements (modals) are outside the shell, so a minimum is
  // enforced for them separately.

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

  /** Flex children never collapse their margins, so a plain sum is exact. */
  function outerHeight(el: HTMLElement): number {
    const cs = getComputedStyle(el);
    return el.offsetHeight + parseFloat(cs.marginTop) + parseFloat(cs.marginBottom);
  }

  /**
   * The window height that would show everything: every fixed row at its own
   * height, plus the scroller's content rather than the slot it was given.
   * Measuring the scroller itself would pin the window at whatever size it
   * already has and it could never shrink again.
   */
  function desiredHeight(): number {
    const shell = document.querySelector<HTMLElement>("[data-shell]");
    const scroller = shell?.querySelector<HTMLElement>("[data-scroller]");
    const content = scroller?.querySelector<HTMLElement>("[data-scroll-content]");
    if (!shell || !scroller || !content) {
      // No shell yet (or a view that has no scroller): fall back to the
      // document, which is what this used to measure all the time.
      return document.getElementById("app")?.scrollHeight ?? MIN_H;
    }
    let h = 0;
    for (const child of Array.from(shell.children) as HTMLElement[]) {
      h += child === scroller ? outerHeight(content) : outerHeight(child);
    }
    return h;
  }

  $effect(() => {
    // Touch reactive deps so the effect re-runs when they change.
    void appState.view;
    void appState.entries;
    void appState.query;
    void appState.loading;
    void appState.installLocation;
    void appState.settings.relocatePromptDismissed;
    void appState.relocateSnoozeClicked;
    void appState.relocate;
    const modal = appState.modal;

    const view = appState.view;
    void tick().then(() => {
      // Wait one extra frame so the browser finishes layout after
      // Svelte's DOM update — prevents measuring stale scrollHeight
      // when switching views (e.g., opening settings from tray menu).
      requestAnimationFrame(() => {
        let h = Math.max(MIN_H, desiredHeight());
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
    return () => { if (modalTimer) clearTimeout(modalTimer); };
  });

  /** Global keyboard shortcuts. */
  function onGlobalKeydown(e: KeyboardEvent) {
	if (e.defaultPrevented) return;
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
    <!-- Exactly one viewport tall: the search box stays put and everything
         else scrolls inside the list. Before this the shell grew past the
         window, the document scrolled instead, and the search box went with
         it — see desiredHeight() for how the window size is derived now. -->
    <main data-shell class="flex h-screen flex-col bg-surface text-on-surface">
      <Header />
      {#if appState.operationError}
        <p role="alert" class="shrink-0 px-3 text-xs text-danger">{t("operation.error", {error: appState.operationError})}</p>
      {/if}
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

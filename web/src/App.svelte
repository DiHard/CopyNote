<script lang="ts">
  import { onMount, onDestroy, tick } from "svelte";
  import {
    state as appState,
    refresh,
    loadSettings,
    openSettings,
    closeSettings,
    loadUpdateInfo,
    // loadInstallLocation, // temporarily disabled with the relocate feature
    resetForShow,
  } from "./lib/state.svelte";
  import { focusSearch, nextTabStop } from "./lib/focus";
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
    // Temporarily disabled while investigating antivirus detections related
    // to copying the executable. Keep the loader so the feature can be
    // restored without reworking the startup flow.
    // void loadInstallLocation();
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
  // Fixed/absolute elements (modals) are outside the shell and are measured
  // separately; see modalHeight().

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

  /**
   * A modal is position: fixed, so desiredHeight() never sees it. MODAL_MIN_H
   * keeps room for the usual form until it renders; after that the card is
   * measured, because the value field can be dragged taller than that room
   * and the buttons would end up below the window.
   */
  function modalHeight(): number {
    const overlay = document.querySelector<HTMLElement>("[data-modal]");
    const card = overlay?.querySelector<HTMLElement>("[data-modal-card]");
    if (!overlay || !card) return 0;
    const cs = getComputedStyle(overlay);
    return card.offsetHeight + parseFloat(cs.paddingTop) + parseFloat(cs.paddingBottom);
  }

  /** Asks Go for the height the current view and modal need. */
  function fitWindow(): void {
    let h = Math.max(MIN_H, desiredHeight());
    if (appState.modal) h = Math.max(h, MODAL_MIN_H, modalHeight());
    if (appState.view === "settings") h = Math.max(h, SETTINGS_MIN_H);
    smoothResize(h);
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
    void appState.operationError;
    void appState.copyError;
    void appState.modal;

    void tick().then(() => {
      // Wait one extra frame so the browser finishes layout after
      // Svelte's DOM update — prevents measuring stale scrollHeight
      // when switching views (e.g., opening settings from tray menu).
      requestAnimationFrame(fitWindow);
    });
  });

  // A modal waits until the window can hold it: on an undersized window the
  // overlay shows a dark bar at the bottom while the resize animation catches
  // up. A window that is already tall enough — any list of a few entries —
  // shows it at once; a fixed wait there only made every add, edit and delete
  // feel slow.
  const MODAL_GROW_MS = 180;
  let showModal = $state(false);

  $effect(() => {
    if (!appState.modal) {
      showModal = false;
      return;
    }
    if (window.innerHeight >= MODAL_MIN_H) {
      showModal = true;
      return;
    }
    const timer = window.setTimeout(() => { showModal = true; }, MODAL_GROW_MS);
    return () => clearTimeout(timer);
  });

  // Once the modal is on screen the window follows its card: a dragged value
  // field or a new error line changes the height without touching any state
  // the resize effect above tracks.
  $effect(() => {
    if (!showModal) return;
    void appState.modal;
    const card = document.querySelector<HTMLElement>("[data-modal-card]");
    if (!card) return;
    const observer = new ResizeObserver(() => fitWindow());
    observer.observe(card);
    return () => observer.disconnect();
  });

  /** Global keyboard shortcuts. */
  function onGlobalKeydown(e: KeyboardEvent) {
	if (e.defaultPrevented) return;
    // Tab from the search box reaches the list before the header buttons; see
    // nextTabStop. Modals trap Tab themselves, and Settings keeps the default.
    if (e.key === "Tab" && !e.altKey && !e.ctrlKey && !e.metaKey && !appState.modal && appState.view === "main") {
      const shell = document.querySelector<HTMLElement>("main[data-shell]");
      const next = shell ? nextTabStop(shell, document.activeElement, e.shiftKey) : null;
      if (next) {
        e.preventDefault();
        next.focus();
      }
      return;
    }
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
      {#if appState.copyError}
        <!-- The badge on the card says that a copy failed; this says why. -->
        <p role="alert" class="shrink-0 px-3 text-xs text-danger">
          {appState.copyError.busy
            ? t("copyError.busy")
            : t("copyError.failed", { error: appState.copyError.detail })}
        </p>
      {/if}
      {#if appState.operationError}
        <p role="alert" class="shrink-0 px-3 text-xs text-danger">{t("operation.error", {error: appState.operationError})}</p>
      {/if}
      <EntryList />
    </main>

    {#if showModal && appState.modal?.kind === "create"}
      <EntryModal initialLabel={appState.modal.label} />
    {:else if showModal && appState.modal?.kind === "edit"}
      <EntryModal entry={appState.modal.entry} />
    {:else if showModal && appState.modal?.kind === "delete"}
      <ConfirmModal entry={appState.modal.entry} />
    {/if}
  {/if}
{/key}

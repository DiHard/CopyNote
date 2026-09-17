<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { flip } from "svelte/animate";
  import { cubicOut } from "svelte/easing";
  import {
    state as appState,
    filterEntries,
    openCreate,
    openCreateFromSearch,
    reorderEntries,
    dismissFirstCopyHint,
    // shouldShowRelocateBanner, // temporarily disabled with the relocate UI
  } from "../state.svelte";
  import { t } from "../i18n";
  import { SEARCH_ID } from "../focus";
  import type { Entry } from "../types";
  import EntryCard from "./EntryCard.svelte";
  // import RelocateBanner from "./RelocateBanner.svelte"; // temporarily disabled

  let { interactionDisabled = false }: { interactionDisabled?: boolean } = $props();

  const filtered = $derived(filterEntries(appState.entries, appState.query));
  const canDrag = $derived(appState.query.trim() === "");

  /** A long query would blow the "create ..." button out of a 420 px window. */
  function shortQuery(): string {
    const q = appState.query.trim();
    return q.length > 24 ? q.slice(0, 24) + "…" : q;
  }

  // ── Drag state ───────────────────────────────────────────────────
  // Two-phase: a pointerdown records `pendingDrag`; only after the
  // pointer travels past DRAG_THRESHOLD pixels do we promote it to an
  // active drag (`draggingId` set, `dragOrder` snapshotted).
  let draggingId = $state<string | null>(null);
  let dragOrder = $state<Entry[] | null>(null);
  let pendingDrag: { id: string; x: number; y: number } | null = null;
  let suppressNextClick = false;
  let listEl: HTMLDivElement | null = null;
  let scrollTimer: number | null = null;
  let scrollDir = 0;

  const DRAG_THRESHOLD = 5;
  const SCROLL_EDGE = 28;
  const SCROLL_STEP = 6;

  // While dragging we render `dragOrder`; otherwise `filtered`.
  const renderList = $derived<Entry[]>(dragOrder ?? filtered);

  // ── Roving tabindex ──────────────────────────────────────────────
  // The list is one Tab stop, not three per entry: only the card that last
  // had focus is tabbable, and the arrows move between them. Tabbing out and
  // back therefore returns to where the user was, and a filter that removes
  // that entry falls back to the top of the list.
  let activeId = $state<string | null>(null);
  const tabbableId = $derived(
    renderList.some((e) => e.id === activeId) ? activeId : (renderList[0]?.id ?? null),
  );

  // The native window stays mounted while it is parked off-screen. Reset the
  // DOM-only position whenever a hide completes, so a tray reopen starts at
  // the top and Tab enters at the first card.
  $effect(() => {
    void appState.listResetToken;
    activeId = null;
    if (listEl) listEl.scrollTop = 0;
  });

  // ── Handlers ─────────────────────────────────────────────────────
  function onCardPointerDown(e: PointerEvent, id: string): void {
    if (interactionDisabled) return;
    if (!canDrag) return;
    if (e.button !== 0) return;
    const target = e.target as HTMLElement | null;
    // Pointerdown on edit/delete buttons must not arm a drag; those
    // buttons mark themselves with data-no-drag.
    if (target?.closest("[data-no-drag]")) return;
    pendingDrag = { id, x: e.clientX, y: e.clientY };
  }

  function cancelDrag(): void {
    pendingDrag = null;
    draggingId = null;
    dragOrder = null;
    stopAutoScroll();
  }

  function onWindowPointerMove(e: PointerEvent): void {
    if (interactionDisabled) {
      // A pointerdown may have happened just before the native slide started.
      // Do not let a later move turn that stale press into a drag.
      if (pendingDrag || draggingId) suppressNextClick = true;
      cancelDrag();
      return;
    }
    if (pendingDrag && !draggingId) {
      if ((e.buttons & 1) === 0) {
        pendingDrag = null;
        return;
      }
      const dx = e.clientX - pendingDrag.x;
      const dy = e.clientY - pendingDrag.y;
      if (Math.hypot(dx, dy) > DRAG_THRESHOLD) {
        draggingId = pendingDrag.id;
        dragOrder = [...filtered];
      }
    }
    if (draggingId) {
      if ((e.buttons & 1) === 0) {
        suppressNextClick = true;
        cancelDrag();
        return;
      }
      e.preventDefault();
      updateDragOrder(e.clientY);
      updateAutoScroll(e.clientY);
    }
  }

  function onWindowPointerUp(): void {
    if (interactionDisabled) {
      if (pendingDrag || draggingId) suppressNextClick = true;
      cancelDrag();
      return;
    }
    if (draggingId && dragOrder) {
      const same =
        dragOrder.length === filtered.length &&
        dragOrder.every((e, i) => e.id === filtered[i].id);
      if (!same) {
        suppressNextClick = true;
        // The dragged card may now be lower down; the new visual top should
        // be the list's Tab entry point.
        activeId = null;
        void reorderEntries(dragOrder.map((e) => e.id));
      }
    }
    cancelDrag();
  }

  function onWindowPointerCancel(): void {
    cancelDrag();
  }

  function onWindowBlur(): void {
    // The native window can slide away before WebView2 receives pointerup.
    // A blur means the in-progress gesture is no longer safe to complete.
    if (pendingDrag || draggingId) suppressNextClick = true;
    cancelDrag();
  }

  function onWindowKeyDown(e: KeyboardEvent): void {
    if (e.key === "Escape" && draggingId) {
      // Cancel drag — restore original order.
      e.preventDefault();
      e.stopPropagation();
      cancelDrag();
    }
  }

  function updateDragOrder(pointerY: number): void {
    if (!listEl || !draggingId || !dragOrder) return;
    const cards = listEl.querySelectorAll<HTMLElement>("[data-entry-id]");
    const others: { id: string; midY: number }[] = [];
    cards.forEach((el) => {
      const id = el.dataset.entryId;
      if (!id || id === draggingId) return;
      const r = el.getBoundingClientRect();
      others.push({ id, midY: r.top + r.height / 2 });
    });
    let insertAt = others.findIndex((c) => pointerY < c.midY);
    if (insertAt === -1) insertAt = others.length;

    const dragged = dragOrder.find((e) => e.id === draggingId);
    if (!dragged) return;
    const rest = dragOrder.filter((e) => e.id !== draggingId);
    const next = [
      ...rest.slice(0, insertAt),
      dragged,
      ...rest.slice(insertAt),
    ];
    // Avoid a reactivity churn when the order is unchanged.
    let changed = false;
    for (let i = 0; i < next.length; i++) {
      if (next[i].id !== dragOrder[i].id) {
        changed = true;
        break;
      }
    }
    if (changed) dragOrder = next;
  }

  function updateAutoScroll(pointerY: number): void {
    if (!listEl) return;
    const r = listEl.getBoundingClientRect();
    let dir = 0;
    if (pointerY < r.top + SCROLL_EDGE) dir = -1;
    else if (pointerY > r.bottom - SCROLL_EDGE) dir = 1;
    if (dir === scrollDir) return;
    scrollDir = dir;
    if (dir === 0) {
      stopAutoScroll();
      return;
    }
    if (scrollTimer === null) {
      scrollTimer = window.setInterval(() => {
        if (listEl && scrollDir !== 0) {
          listEl.scrollTop += scrollDir * SCROLL_STEP;
        }
      }, 16);
    }
  }

  function stopAutoScroll(): void {
    if (scrollTimer !== null) {
      clearInterval(scrollTimer);
      scrollTimer = null;
    }
    scrollDir = 0;
  }

  function onClickCapture(e: MouseEvent): void {
    if (suppressNextClick) {
      e.preventDefault();
      e.stopPropagation();
      suppressNextClick = false;
    }
  }

  function onKeyboardMove(id: string, dir: -1 | 1): void {
    if (!canDrag) return;
    const arr = appState.entries;
    const idx = arr.findIndex((e) => e.id === id);
    if (idx < 0) return;
    const newIdx = idx + dir;
    if (newIdx < 0 || newIdx >= arr.length) return;
    const ids = arr.map((e) => e.id);
    [ids[idx], ids[newIdx]] = [ids[newIdx], ids[idx]];
    // Reordering changes the visual top of the list.
    activeId = null;
    void reorderEntries(ids);
  }

  function onWindowFocusIn(e: FocusEvent): void {
    // Search is the stable entry point for a fresh keyboard pass. WebView2
    // can restore a card focus during show, so clear the stale card here too.
    const target = e.target as HTMLElement | null;
    if (target?.id === SEARCH_ID) activeId = null;
  }

  // Suppress the synthetic click that fires after a drag-mouseup, so a
  // drag never accidentally copies the dragged entry. Capture phase so
  // we beat the inner button's click handler.
  onMount(() => {
    window.addEventListener("click", onClickCapture, true);
  });
  onDestroy(() => {
    window.removeEventListener("click", onClickCapture, true);
    stopAutoScroll();
  });

  // Cancel a gesture immediately when the native window starts moving. The
  // transparent shield in App.svelte also prevents a new pointerdown.
  $effect(() => {
    if (interactionDisabled) {
      if (pendingDrag || draggingId) suppressNextClick = true;
      cancelDrag();
    }
  });

  // While dragging, pin the cursor and disable text selection globally.
  $effect(() => {
    if (draggingId) {
      document.body.style.cursor = "grabbing";
      document.body.style.userSelect = "none";
    } else {
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    }
  });
</script>

<svelte:window
  onfocusin={onWindowFocusIn}
  onpointermove={onWindowPointerMove}
  onpointerup={onWindowPointerUp}
  onpointercancel={onWindowPointerCancel}
  onblur={onWindowBlur}
  onkeydown={onWindowKeyDown}
/>

<!-- The only scroller in the app. min-h-0 is what lets a flex child actually
     shrink below its content and scroll; the padding lives on the inner
     wrapper so App can measure the content's true height. -->
<div bind:this={listEl} data-scroller class="min-h-0 flex-1 overflow-y-auto">
  <div data-scroll-content class="px-3 py-3">
    <!-- The relocate banner is temporarily disabled while investigating
         antivirus detections related to copying the executable. The component
         remains in the source for a later, one-line restoration. -->
    {#if appState.loading}
      <div
        class="flex min-h-[7rem] items-center justify-center text-sm text-on-surface-dim"
      >
        {t("list.loading")}
      </div>
    {:else if appState.loadError}
      <div
        class="rounded-md border border-danger/40 bg-danger-dim p-3 text-xs text-danger"
      >
        {t("list.error", { error: appState.loadError })}
      </div>
    {:else if appState.entries.length === 0}
      <!-- The one moment the app can explain itself. It answers only what is
           useful before there is anything to click: what this is for, and
           where the window is about to disappear to. How to copy is taught
           later, under the first entry, where it can be tried. -->
      <div class="flex flex-col items-center gap-3 px-2 py-6 text-center">
        <div class="flex flex-col gap-1">
          <p class="text-sm font-medium text-on-surface">{t("list.empty")}</p>
          <p class="text-xs leading-snug text-on-surface-dim">{t("list.empty.what")}</p>
        </div>
        <button
          type="button"
          data-list-focus
          onclick={openCreate}
          class="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-accent-text shadow-sm transition hover:bg-accent-hover"
        >
          {t("list.empty.add")}
        </button>
        <!-- The single most useful sentence here: in Windows 11 a new tray
             icon is hidden in the overflow, so without this the window is
             gone for good the first time it auto-hides. -->
        <p class="mt-3 text-[11px] leading-snug text-on-surface-dim">
          {t("list.empty.tray")}
        </p>
      </div>
    {:else if filtered.length === 0}
      <div
        class="flex min-h-[7rem] flex-col items-center justify-center gap-3 text-center"
      >
        <p class="text-sm text-on-surface-dim">{t("list.noMatch")}</p>
        <button
          type="button"
          data-list-focus
          onclick={() => openCreateFromSearch(appState.query)}
          class="max-w-full truncate rounded-md border border-outline bg-card px-3 py-1.5 text-sm text-on-surface transition hover:bg-card-hover"
        >
          {t("list.noMatch.create", { query: shortQuery() })}
        </button>
      </div>
    {:else}
      {#snippet card(entry: Entry, i: number)}
        <EntryCard
          {entry}
          isDragging={entry.id === draggingId}
          dragInProgress={draggingId !== null}
          dragDisabled={!canDrag || interactionDisabled}
          tabbable={entry.id === tabbableId}
          canMoveUp={canDrag && i > 0}
          canMoveDown={canDrag && i < renderList.length - 1}
          onDragPointerDown={(e) => onCardPointerDown(e, entry.id)}
          onMoveByKey={(dir) => onKeyboardMove(entry.id, dir)}
          onFocused={() => (activeId = entry.id)}
        />
      {/snippet}
      <!-- The list's direct child carries the role. On the card inside, one
           wrapper further down, the items fell outside the list and a screen
           reader never announced it.

           Only an unfiltered list animates. animate:flip is there for moves —
           a drag, Ctrl+↑↓, an entry added or deleted — but every row a keyed
           update removes, Svelte first measures and pins in place with a
           forced layout of its own, whether or not anything animates after.
           A search that hid most of a thousand entries spent seconds on that,
           and a filtered list cannot be reordered anyway. -->
      <div role="list" class="flex flex-col gap-2">
        {#if canDrag}
          {#each renderList as entry, i (entry.id)}
            <div role="listitem" animate:flip={{ duration: 180, easing: cubicOut }}>
              {@render card(entry, i)}
            </div>
          {/each}
        {:else}
          {#each renderList as entry, i (entry.id)}
            <div role="listitem">{@render card(entry, i)}</div>
          {/each}
        {/if}
      </div>
      {#if appState.showFirstCopyHint}
        <!-- Shown once, right after the list stops being empty: this is the
             first moment "click a card" can actually be tried. Session-only,
             and any successful copy retires it. -->
        <div
          class="mt-2 flex items-start gap-2 rounded-lg border border-outline bg-card px-2.5 py-2"
        >
          <p class="min-w-0 flex-1 text-[11px] leading-snug text-on-surface-dim">
            {t("list.firstCopyHint")}
          </p>
          <button
            type="button"
            onclick={dismissFirstCopyHint}
            data-tooltip={t("list.firstCopyHint.dismiss")}
            aria-label={t("list.firstCopyHint.dismiss")}
            class="-mr-1 shrink-0 rounded p-1.5 text-on-surface-dim transition hover:bg-surface-hover hover:text-on-surface"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="13"
              height="13"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <line x1="18" y1="6" x2="6" y2="18"></line>
              <line x1="6" y1="6" x2="18" y2="18"></line>
            </svg>
          </button>
        </div>
      {/if}
    {/if}
  </div>
</div>

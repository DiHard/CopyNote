<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { flip } from "svelte/animate";
  import { cubicOut } from "svelte/easing";
  import {
    state as appState,
    filterEntries,
    openCreate,
    reorderEntries,
  } from "../state.svelte";
  import { t } from "../i18n";
  import type { Entry } from "../types";
  import EntryCard from "./EntryCard.svelte";

  const filtered = $derived(filterEntries(appState.entries, appState.query));
  const canDrag = $derived(appState.query.trim() === "");

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

  // ── Handlers ─────────────────────────────────────────────────────
  function onCardPointerDown(e: PointerEvent, id: string): void {
    if (!canDrag) return;
    if (e.button !== 0) return;
    const target = e.target as HTMLElement | null;
    // Pointerdown on edit/delete buttons must not arm a drag; those
    // buttons mark themselves with data-no-drag.
    if (target?.closest("[data-no-drag]")) return;
    pendingDrag = { id, x: e.clientX, y: e.clientY };
  }

  function onWindowPointerMove(e: PointerEvent): void {
    if (pendingDrag && !draggingId) {
      const dx = e.clientX - pendingDrag.x;
      const dy = e.clientY - pendingDrag.y;
      if (Math.hypot(dx, dy) > DRAG_THRESHOLD) {
        draggingId = pendingDrag.id;
        dragOrder = [...filtered];
      }
    }
    if (draggingId) {
      e.preventDefault();
      updateDragOrder(e.clientY);
      updateAutoScroll(e.clientY);
    }
  }

  function onWindowPointerUp(): void {
    if (draggingId && dragOrder) {
      const same =
        dragOrder.length === filtered.length &&
        dragOrder.every((e, i) => e.id === filtered[i].id);
      if (!same) {
        suppressNextClick = true;
        void reorderEntries(dragOrder.map((e) => e.id));
      }
    }
    pendingDrag = null;
    draggingId = null;
    dragOrder = null;
    stopAutoScroll();
  }

  function onWindowKeyDown(e: KeyboardEvent): void {
    if (e.key === "Escape" && draggingId) {
      // Cancel drag — restore original order.
      e.preventDefault();
      e.stopPropagation();
      pendingDrag = null;
      draggingId = null;
      dragOrder = null;
      stopAutoScroll();
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
    void reorderEntries(ids);
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
  onpointermove={onWindowPointerMove}
  onpointerup={onWindowPointerUp}
  onkeydown={onWindowKeyDown}
/>

<div bind:this={listEl} class="flex-1 overflow-y-auto px-3 py-3">
  {#if appState.loading}
    <div
      class="flex h-full items-center justify-center text-sm text-on-surface-dim"
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
    <div
      class="flex h-full flex-col items-center justify-center gap-3 text-center"
    >
      <p class="text-sm text-on-surface-dim">{t("list.empty")}</p>
      <button
        type="button"
        onclick={openCreate}
        class="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-accent-text shadow-sm transition hover:bg-accent-hover"
      >
        {t("list.empty.add")}
      </button>
    </div>
  {:else if filtered.length === 0}
    <div
      class="flex h-full items-center justify-center text-center text-sm text-on-surface-dim"
    >
      {t("list.noMatch")}
    </div>
  {:else}
    <div role="list" class="flex flex-col gap-2">
      {#each renderList as entry (entry.id)}
        <div animate:flip={{ duration: 180, easing: cubicOut }}>
          <EntryCard
            {entry}
            isDragging={entry.id === draggingId}
            dragInProgress={draggingId !== null}
            dragDisabled={!canDrag}
            onDragPointerDown={(e) => onCardPointerDown(e, entry.id)}
            onMoveByKey={(dir) => onKeyboardMove(entry.id, dir)}
          />
        </div>
      {/each}
    </div>
  {/if}
</div>

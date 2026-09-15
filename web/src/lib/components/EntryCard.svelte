<script lang="ts">
  import type { Entry } from "../types";
  import { openEdit, openDelete, copyEntry } from "../state.svelte";
  import { moveCardFocus, focusCardAt, focusLastCard } from "../focus";
  import { t } from "../i18n";
  import { onDestroy } from "svelte";

  let {
    entry,
    isDragging = false,
    dragInProgress = false,
    dragDisabled = false,
    tabbable = false,
    onDragPointerDown,
    onMoveByKey,
    onFocused,
  }: {
    entry: Entry;
    isDragging?: boolean;
    dragInProgress?: boolean;
    dragDisabled?: boolean;
    /** Roving tabindex: the whole list is one Tab stop, and this is it. */
    tabbable?: boolean;
    onDragPointerDown?: (e: PointerEvent) => void;
    onMoveByKey?: (dir: -1 | 1) => void;
    onFocused?: () => void;
  } = $props();

  type CopyState = "idle" | "copied" | "failed";
  let copyState = $state<CopyState>("idle");
  let timer: number | null = null;

  function flash(state: CopyState) {
    if (timer !== null) clearTimeout(timer);
    copyState = state;
    timer = window.setTimeout(() => {
      copyState = "idle";
      timer = null;
    }, 900);
  }

  async function onCopy() {
    try {
      await copyEntry(entry.id);
      flash("copied");
    } catch {
      flash("failed");
    }
  }

  function onKeyDown(e: KeyboardEvent) {
    // The row's own actions are out of the Tab order, so they get the keys a
    // Windows user already expects. Announced via aria-keyshortcuts below.
    if (e.key === "F2") {
      e.preventDefault();
      openEdit(entry);
      return;
    }
    if (e.key === "Delete") {
      e.preventDefault();
      openDelete(entry);
      return;
    }
    if (e.key === "Home") {
      e.preventDefault();
      focusCardAt(0);
      return;
    }
    if (e.key === "End") {
      e.preventDefault();
      focusLastCard();
      return;
    }
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
    const dir = e.key === "ArrowDown" ? 1 : -1;
    // Ctrl moves the entry, a plain arrow moves the focus.
    if (e.ctrlKey) {
      if (dragDisabled) return;
      e.preventDefault();
      onMoveByKey?.(dir);
      return;
    }
    e.preventDefault();
    moveCardFocus(dir);
  }

  onDestroy(() => {
    if (timer !== null) clearTimeout(timer);
  });
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  role="listitem"
  data-entry-id={entry.id}
  onpointerdown={(e) => onDragPointerDown?.(e)}
  class="group relative flex items-stretch gap-1 rounded-lg border border-outline bg-surface-alt transition hover:border-outline-strong hover:bg-card-hover {isDragging
    ? 'opacity-60 shadow-lg ring-2 ring-accent/40'
    : ''}"
>
  <!-- data-card-focus: the element arrow keys walk between; see lib/focus.ts -->
  <button
    type="button"
    data-card-focus
    tabindex={tabbable ? 0 : -1}
    onfocus={() => onFocused?.()}
    onclick={onCopy}
    onkeydown={onKeyDown}
    title={t("card.copy")}
    class="flex min-w-0 flex-1 items-start px-3 py-2.5 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-accent rounded-l-lg {dragInProgress
      ? 'cursor-grabbing'
      : 'cursor-pointer'}"
  >
    <div class="min-w-0 flex-1">
      <div class="truncate text-sm font-semibold text-on-surface">
        {entry.label}
      </div>
      {#if entry.value && entry.value !== entry.label}
        <div class="truncate text-xs text-on-surface-dim">{entry.value}</div>
      {/if}
    </div>
  </button>

  <div
    data-no-drag
    class="flex shrink-0 items-center gap-1 px-1.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 {copyState !==
    'idle'
      ? 'pointer-events-none opacity-0'
      : ''}"
  >
    <!-- Out of the Tab order so the list costs one stop, not three; the key
         is announced instead. -->
    <button
      type="button"
      tabindex="-1"
      aria-keyshortcuts="F2"
      title={t("card.edit")}
      aria-label={t("card.edit")}
      onclick={() => openEdit(entry)}
      class="rounded p-1.5 text-on-surface-dim hover:bg-surface-hover hover:text-on-surface focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path
          d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"
        ></path>
        <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"
        ></path>
      </svg>
    </button>
    <button
      type="button"
      tabindex="-1"
      aria-keyshortcuts="Delete"
      title={t("card.delete")}
      aria-label={t("card.delete")}
      onclick={() => openDelete(entry)}
      class="rounded p-1.5 text-on-surface-dim hover:bg-danger-dim hover:text-danger focus:outline-none focus-visible:ring-2 focus-visible:ring-danger"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <polyline points="3 6 5 6 21 6"></polyline>
        <path
          d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"
        ></path>
      </svg>
    </button>
  </div>

  <div
    class="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 transition-all duration-200 {copyState ===
    'idle'
      ? 'translate-x-1 opacity-0'
      : 'opacity-100'}"
    aria-live="polite"
  >
    {#if copyState === "copied"}
      <span
        class="rounded-md bg-success px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-on-success shadow-sm"
      >
        {t("card.copied")}
      </span>
    {:else if copyState === "failed"}
      <span
        class="rounded-md bg-danger px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-on-danger shadow-sm"
      >
        {t("card.failed")}
      </span>
    {/if}
  </div>
</div>

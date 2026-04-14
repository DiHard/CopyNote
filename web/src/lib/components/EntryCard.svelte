<script lang="ts">
  import type { Entry } from "../types";
  import { openEdit, openDelete, copyEntry } from "../state.svelte";
  import { onDestroy } from "svelte";

  let { entry }: { entry: Entry } = $props();

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

  onDestroy(() => {
    if (timer !== null) clearTimeout(timer);
  });
</script>

<div
  class="group relative flex items-stretch gap-1 rounded-lg border border-outline bg-surface-alt transition hover:border-outline-strong hover:bg-card-hover"
>
  <button
    type="button"
    onclick={onCopy}
    title="Click to copy"
    class="flex min-w-0 flex-1 cursor-pointer items-start px-3 py-2.5 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-accent rounded-l-lg"
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
    class="flex shrink-0 items-center gap-1 px-2 opacity-0 transition-opacity group-hover:opacity-100 {copyState !==
    'idle'
      ? 'pointer-events-none opacity-0'
      : ''}"
  >
    <button
      type="button"
      title="Edit"
      aria-label="Edit"
      tabindex={-1}
      onclick={() => openEdit(entry)}
      class="rounded p-1 text-on-surface-dim hover:bg-surface-hover hover:text-on-surface"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="14"
        height="14"
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
      title="Delete"
      aria-label="Delete"
      tabindex={-1}
      onclick={() => openDelete(entry)}
      class="rounded p-1 text-on-surface-dim hover:bg-danger-dim hover:text-danger"
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="14"
        height="14"
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
        class="rounded-md bg-success px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-white shadow-sm"
      >
        Copied
      </span>
    {:else if copyState === "failed"}
      <span
        class="rounded-md bg-danger px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-white shadow-sm"
      >
        Failed
      </span>
    {/if}
  </div>
</div>

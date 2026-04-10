<script lang="ts">
  import type { Entry } from "../types";
  import { closeModal, deleteEntry } from "../state.svelte";

  let { entry }: { entry: Entry } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

  async function confirm() {
    busy = true;
    error = null;
    try {
      await deleteEntry(entry.id);
      closeModal();
    } catch (e) {
      error = String(e).replace(/^Error:\s*/, "");
    } finally {
      busy = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.preventDefault();
      closeModal();
    }
    if (e.key === "Enter" && !busy) {
      e.preventDefault();
      confirm();
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
  class="fixed inset-0 z-40 flex items-center justify-center bg-overlay p-4"
  role="dialog"
  aria-modal="true"
  tabindex="-1"
  onclick={(e) => {
    if (e.target === e.currentTarget) closeModal();
  }}
>
  <div
    class="w-full max-w-sm rounded-xl border border-outline bg-surface-alt p-4 shadow-2xl"
  >
    <h2 class="mb-1 text-base font-semibold text-on-surface">Delete entry?</h2>
    <p class="mb-4 text-sm text-on-surface-dim">
      "{entry.label}" will be permanently removed.
    </p>

    {#if error}
      <div class="mb-3 text-xs text-danger">{error}</div>
    {/if}

    <div class="flex justify-end gap-2">
      <button
        type="button"
        onclick={closeModal}
        class="rounded-md border border-outline bg-surface px-3 py-1.5 text-sm text-on-surface hover:bg-surface-hover"
      >
        Cancel
      </button>
      <button
        type="button"
        disabled={busy}
        onclick={confirm}
        class="rounded-md bg-danger px-3 py-1.5 text-sm font-medium text-white shadow-sm transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Delete
      </button>
    </div>
  </div>
</div>

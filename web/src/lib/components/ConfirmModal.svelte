<script lang="ts">
  import { modalFocus } from "../modalFocus";
  import { fade } from "svelte/transition";
  import type { Entry } from "../types";
  import { closeModal, deleteEntry } from "../state.svelte";
  import { t } from "../i18n";

  let { entry }: { entry: Entry } = $props();

  let busy = $state(false);
  let error = $state<string | null>(null);

  async function confirm() {
    if (busy) return;
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
    if (e.key === "Escape" && !busy) {
      e.preventDefault();
      closeModal();
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- Scrolls rather than centres, like EntryModal: a long label can make the
     card taller than the window. -->
<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
  data-modal
  class="fixed inset-0 z-40 flex overflow-y-auto bg-overlay p-4 outline-none"
  use:modalFocus
  role="dialog"
  aria-labelledby="modal-title"
  aria-modal="true"
  tabindex="-1"
  onclick={(e) => {
    if (e.target === e.currentTarget && !busy) closeModal();
  }}
>
  <div
    data-modal-card
    class="m-auto w-full max-w-sm rounded-xl border border-outline bg-surface-alt p-4 shadow-2xl"
    transition:fade={{ duration: 150 }}
  >
    <h2 id="modal-title" class="mb-1 text-base font-semibold text-on-surface">{t("confirm.delete.title")}</h2>
    <p class="mb-4 text-sm text-on-surface-dim">
      "{entry.label}" {t("confirm.delete.body")}
    </p>

    {#if error}
      <div class="mb-3 text-xs text-danger">{error}</div>
    {/if}

    <div class="flex justify-end gap-2">
      <button
        type="button"
        onclick={closeModal}
        disabled={busy}
        data-initial-focus
        class="rounded-md border border-outline bg-surface px-3 py-1.5 text-sm text-on-surface hover:bg-surface-hover"
      >
        {t("confirm.cancel")}
      </button>
      <button
        type="button"
        disabled={busy}
        onclick={confirm}
        class="rounded-md bg-danger px-3 py-1.5 text-sm font-medium text-on-danger shadow-sm transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {t("confirm.delete")}
      </button>
    </div>
  </div>
</div>

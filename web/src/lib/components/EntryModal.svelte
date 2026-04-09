<script lang="ts">
  import type { Entry } from "../types";
  import { closeModal, createEntry, updateEntry } from "../state.svelte";

  let { entry }: { entry?: Entry } = $props();

  const isEdit = $derived(!!entry);

  // Initial form values come from `entry` once at mount; we deliberately
  // do NOT track future prop changes (the modal is recreated per open).
  /* svelte-ignore state_referenced_locally */
  let label = $state(entry?.label ?? "");
  /* svelte-ignore state_referenced_locally */
  let value = $state(entry?.value ?? "");
  let error = $state<string | null>(null);
  let saving = $state(false);

  let labelInput = $state<HTMLInputElement | null>(null);
  $effect(() => {
    labelInput?.focus();
  });

  const canSave = $derived(label.trim().length > 0 && !saving);

  async function save() {
    if (!canSave) return;
    saving = true;
    error = null;
    try {
      if (entry) {
        await updateEntry(entry.id, label, value);
      } else {
        await createEntry(label, value);
      }
      closeModal();
    } catch (e) {
      error = String(e).replace(/^Error:\s*/, "");
    } finally {
      saving = false;
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.preventDefault();
      closeModal();
    }
  }
</script>

<svelte:window onkeydown={onKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div
  class="fixed inset-0 z-40 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
  role="dialog"
  aria-modal="true"
  tabindex="-1"
  onclick={(e) => {
    if (e.target === e.currentTarget) closeModal();
  }}
>
  <div
    class="w-full max-w-sm rounded-xl border border-slate-700/60 bg-slate-900 p-4 shadow-2xl"
  >
    <h2 class="mb-3 text-base font-semibold text-slate-100">
      {isEdit ? "Edit entry" : "New entry"}
    </h2>

    <form
      onsubmit={(e) => {
        e.preventDefault();
        save();
      }}
      class="flex flex-col gap-3"
    >
      <label class="flex flex-col gap-1">
        <span class="text-xs font-medium uppercase tracking-wide text-slate-400"
          >Label</span
        >
        <input
          bind:this={labelInput}
          bind:value={label}
          type="text"
          placeholder="e.g. Personal email"
          class="rounded-md border border-slate-700 bg-slate-800 px-3 py-1.5 text-sm text-slate-100 placeholder:text-slate-500 focus:border-indigo-400 focus:outline-none"
        />
      </label>

      <label class="flex flex-col gap-1">
        <span class="text-xs font-medium uppercase tracking-wide text-slate-400"
          >Value</span
        >
        <textarea
          bind:value
          rows="3"
          placeholder="e.g. me@example.com"
          class="resize-y rounded-md border border-slate-700 bg-slate-800 px-3 py-1.5 text-sm text-slate-100 placeholder:text-slate-500 focus:border-indigo-400 focus:outline-none"
        ></textarea>
      </label>

      {#if error}
        <div class="text-xs text-rose-400">{error}</div>
      {/if}

      <div class="mt-2 flex justify-end gap-2">
        <button
          type="button"
          onclick={closeModal}
          class="rounded-md border border-slate-700 bg-slate-800 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-700"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={!canSave}
          class="rounded-md bg-indigo-500 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition hover:bg-indigo-400 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {isEdit ? "Save" : "Create"}
        </button>
      </div>
    </form>
  </div>
</div>

<script lang="ts">
  import { fade } from "svelte/transition";
  import type { Entry } from "../types";
  import { closeModal, createEntry, updateEntry } from "../state.svelte";
  import { t } from "../i18n";

  let { entry }: { entry?: Entry } = $props();

  const isEdit = $derived(!!entry);

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
  class="fixed inset-0 z-40 flex items-center justify-center bg-overlay p-4"
  transition:fade={{ duration: 150 }}
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
    <h2 class="mb-3 text-base font-semibold text-on-surface">
      {isEdit ? t("modal.edit.title") : t("modal.create.title")}
    </h2>

    <form
      onsubmit={(e) => {
        e.preventDefault();
        save();
      }}
      class="flex flex-col gap-3"
    >
      <label class="flex flex-col gap-1">
        <span class="text-xs font-medium uppercase tracking-wide text-on-surface-dim"
          >{t("modal.label")}</span
        >
        <input
          bind:this={labelInput}
          bind:value={label}
          type="text"
          placeholder={t("modal.label.placeholder")}
          class="rounded-md border border-input-border bg-input px-3 py-1.5 text-sm text-on-surface placeholder:text-on-surface-faint focus:border-input-focus focus:outline-none"
        />
      </label>

      <label class="flex flex-col gap-1">
        <span class="text-xs font-medium uppercase tracking-wide text-on-surface-dim"
          >{t("modal.value")}</span
        >
        <textarea
          bind:value
          rows="3"
          placeholder={t("modal.value.placeholder")}
          class="resize-y rounded-md border border-input-border bg-input px-3 py-1.5 text-sm text-on-surface placeholder:text-on-surface-faint focus:border-input-focus focus:outline-none"
        ></textarea>
      </label>

      {#if error}
        <div class="text-xs text-danger">{error}</div>
      {/if}

      <div class="mt-2 flex justify-end gap-2">
        <button
          type="button"
          onclick={closeModal}
          class="rounded-md border border-outline bg-surface px-3 py-1.5 text-sm text-on-surface hover:bg-surface-hover"
        >
          {t("modal.cancel")}
        </button>
        <button
          type="submit"
          disabled={!canSave}
          class="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-accent-text shadow-sm transition hover:bg-accent-hover disabled:cursor-not-allowed disabled:opacity-50"
        >
          {isEdit ? t("modal.save") : t("modal.create")}
        </button>
      </div>
    </form>
  </div>
</div>

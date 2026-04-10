<script lang="ts">
  import { state, filterEntries, openCreate } from "../state.svelte";
  import EntryCard from "./EntryCard.svelte";

  const filtered = $derived(filterEntries(state.entries, state.query));
</script>

<div class="flex-1 overflow-y-auto px-3 py-3">
  {#if state.loading}
    <div class="flex h-full items-center justify-center text-sm text-on-surface-dim">
      Loading…
    </div>
  {:else if state.loadError}
    <div class="rounded-md border border-danger/40 bg-danger-dim p-3 text-xs text-danger">
      Failed to load: {state.loadError}
    </div>
  {:else if state.entries.length === 0}
    <div class="flex h-full flex-col items-center justify-center gap-3 text-center">
      <p class="text-sm text-on-surface-dim">No entries yet</p>
      <button
        type="button"
        onclick={openCreate}
        class="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-accent-text shadow-sm transition hover:bg-accent-hover"
      >
        Add your first entry
      </button>
    </div>
  {:else if filtered.length === 0}
    <div class="flex h-full items-center justify-center text-center text-sm text-on-surface-dim">
      Nothing matches your search
    </div>
  {:else}
    <div class="flex flex-col gap-2">
      {#each filtered as entry (entry.id)}
        <EntryCard {entry} />
      {/each}
    </div>
  {/if}
</div>

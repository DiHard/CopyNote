<script lang="ts">
  import { state, filterEntries, openCreate } from "../state.svelte";
  import EntryCard from "./EntryCard.svelte";

  const filtered = $derived(filterEntries(state.entries, state.query));
</script>

<div class="flex-1 overflow-y-auto px-3 py-3">
  {#if state.loading}
    <div class="flex h-full items-center justify-center text-sm text-slate-500">
      Loading…
    </div>
  {:else if state.loadError}
    <div class="rounded-md border border-rose-700/40 bg-rose-900/20 p-3 text-xs text-rose-300">
      Failed to load: {state.loadError}
    </div>
  {:else if state.entries.length === 0}
    <div class="flex h-full flex-col items-center justify-center gap-3 text-center">
      <p class="text-sm text-slate-400">No entries yet</p>
      <button
        type="button"
        onclick={openCreate}
        class="rounded-md bg-indigo-500 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition hover:bg-indigo-400"
      >
        Add your first entry
      </button>
    </div>
  {:else if filtered.length === 0}
    <div class="flex h-full items-center justify-center text-center text-sm text-slate-500">
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

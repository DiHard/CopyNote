<script lang="ts">
  import { onMount } from "svelte";
  import { state, refresh } from "./lib/state.svelte";
  import Header from "./lib/components/Header.svelte";
  import SearchBar from "./lib/components/SearchBar.svelte";
  import EntryList from "./lib/components/EntryList.svelte";
  import EntryModal from "./lib/components/EntryModal.svelte";
  import ConfirmModal from "./lib/components/ConfirmModal.svelte";

  onMount(() => {
    void refresh();
  });

  /** Escape when no modal is open → hide the window (same as close button). */
  function onGlobalKeydown(e: KeyboardEvent) {
    if (e.key === "Escape" && !state.modal) {
      e.preventDefault();
      // @ts-expect-error - injected by Go via webview.Bind
      window.hide();
    }
  }
</script>

<svelte:window onkeydown={onGlobalKeydown} />

<main
  class="flex h-full flex-col bg-gradient-to-br from-slate-900 to-slate-800 text-slate-100"
>
  <Header />
  <SearchBar />
  <EntryList />
</main>

{#if state.modal?.kind === "create"}
  <EntryModal />
{:else if state.modal?.kind === "edit"}
  <EntryModal entry={state.modal.entry} />
{:else if state.modal?.kind === "delete"}
  <ConfirmModal entry={state.modal.entry} />
{/if}

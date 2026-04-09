<script lang="ts">
  import { openCreate } from "../state.svelte";
</script>

<header
  class="flex items-center justify-between border-b border-slate-700/60 bg-slate-900/60 px-4 py-3 backdrop-blur"
>
  <div class="flex items-center gap-2">
    <div class="h-6 w-6 rounded bg-indigo-500"></div>
    <h1 class="text-base font-semibold text-slate-100">CopyNote</h1>
  </div>
  <button
    type="button"
    onclick={openCreate}
    class="flex items-center gap-1 rounded-md bg-indigo-500 px-3 py-1.5 text-sm font-medium text-white shadow-sm transition hover:bg-indigo-400"
  >
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2.5"
      stroke-linecap="round"
      stroke-linejoin="round"
      ><line x1="12" y1="5" x2="12" y2="19"></line><line
        x1="5"
        y1="12"
        x2="19"
        y2="12"
      ></line></svg
    >
    New
  </button>
</header>

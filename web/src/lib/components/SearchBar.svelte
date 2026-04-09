<script lang="ts">
  import { state } from "../state.svelte";
</script>

<div class="border-b border-slate-700/40 bg-slate-900/40 px-4 py-2">
  <div class="relative">
    <svg
      xmlns="http://www.w3.org/2000/svg"
      class="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-400"
      width="14"
      height="14"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <circle cx="11" cy="11" r="8"></circle>
      <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
    </svg>
    <input
      type="text"
      bind:value={state.query}
      placeholder="Search…"
      class="w-full rounded-md border border-slate-700/70 bg-slate-800/70 py-1.5 pl-8 pr-3 text-sm text-slate-100 placeholder:text-slate-500 focus:border-indigo-400 focus:outline-none"
    />
  </div>
</div>

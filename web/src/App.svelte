<script lang="ts">
  let pingResult = $state<string>("(not called yet)");
  let loading = $state(false);

  async function callPing() {
    loading = true;
    try {
      // @ts-expect-error - injected by Go via webview.Bind
      const res: string = await window.ping("from svelte");
      pingResult = res;
    } catch (e) {
      pingResult = `error: ${e}`;
    } finally {
      loading = false;
    }
  }
</script>

<main class="flex h-full flex-col items-center justify-center gap-6 bg-gradient-to-br from-slate-900 to-slate-800 text-white">
  <h1 class="text-3xl font-bold tracking-tight">CopyNote</h1>
  <p class="text-sm text-slate-400">Scaffold step 0 — bridge test</p>

  <button
    type="button"
    onclick={callPing}
    disabled={loading}
    class="rounded-lg bg-indigo-500 px-4 py-2 text-sm font-medium text-white shadow transition hover:bg-indigo-400 disabled:opacity-50"
  >
    {loading ? "calling..." : "Call Go ping()"}
  </button>

  <div class="rounded-md bg-slate-700/50 px-3 py-2 font-mono text-xs text-slate-200">
    {pingResult}
  </div>
</main>

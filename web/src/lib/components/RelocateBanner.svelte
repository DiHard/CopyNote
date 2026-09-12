<script lang="ts">
  import {
    state as appState,
    relocateApp,
    relocateAppTo,
    dismissRelocatePrompt,
  } from "../state.svelte";
  import { t } from "../i18n";
  import Spinner from "./Spinner.svelte";

  const moving = $derived(appState.relocate.kind === "moving");
  const failure = $derived(
    appState.relocate.kind === "failed" ? appState.relocate.error : null,
  );

  /** The folder name alone — the full path is too long for 420 px and the
   *  name is what makes "you are running from Downloads" land. */
  const folder = $derived.by(() => {
    const dir = appState.installLocation?.dir ?? "";
    const parts = dir.split(/[\\/]/).filter(Boolean);
    return parts.length > 0 ? parts[parts.length - 1] : dir;
  });
</script>

<!-- mt-3 matches the list's own px-3 py-3, and the gap below comes from that
     same padding — so the banner sits on the same 12 px rhythm as the cards. -->
<div class="mx-3 mt-3 rounded-lg border border-outline bg-card px-2.5 py-2">
  <div class="flex items-start gap-2">
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      class="mt-px shrink-0 text-update-dot"
      aria-hidden="true"
    >
      <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z" />
      <path d="M12 9v4" />
      <path d="M12 17h.01" />
    </svg>

    <div class="min-w-0 flex-1">
      <p class="text-xs font-medium">{t("relocate.title", { folder })}</p>
      <!-- Once the move starts, why to move is moot; what matters is that the
           window is about to vanish on purpose. Swapping the text in place
           keeps the banner's height steady. -->
      <p class="mt-0.5 text-[11px] leading-snug text-on-surface-dim">
        {moving ? t("relocate.willRestart") : t("relocate.body")}
      </p>

      {#if failure}
        <p role="alert" class="mt-1 text-[11px] leading-snug text-danger">
          {t("relocate.failed", { error: failure })}
        </p>
      {/if}

      <div class="mt-1.5 flex items-center gap-2">
        <button
          type="button"
          onclick={() => void relocateApp()}
          disabled={moving}
          class="inline-flex items-center gap-1.5 rounded-md bg-accent px-2.5 py-1 text-xs font-medium text-accent-text transition hover:bg-accent-hover disabled:opacity-60"
        >
          {#if moving}<Spinner />{/if}
          {moving ? t("relocate.moving") : t("relocate.move")}
        </button>
        <button
          type="button"
          onclick={() => void relocateAppTo(t("relocate.pickTitle"))}
          disabled={moving}
          class="text-[11px] text-accent transition hover:underline disabled:opacity-60"
        >
          {t("relocate.choose")}
        </button>
        <button
          type="button"
          onclick={() => void dismissRelocatePrompt()}
          disabled={moving}
          class="ml-auto text-[11px] text-on-surface-faint transition hover:text-on-surface-dim disabled:opacity-60"
        >
          {t("relocate.later")}
        </button>
      </div>
    </div>
  </div>
</div>

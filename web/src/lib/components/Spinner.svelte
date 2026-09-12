<script lang="ts">
  /** Inline loading ring. Strokes with currentColor, so it reads correctly
   *  both on the accent button and on the quieter Settings one. */
  let { class: className = "h-3 w-3" }: { class?: string } = $props();
</script>

<svg
  class="{className} shrink-0 animate-spin"
  viewBox="0 0 24 24"
  fill="none"
  aria-hidden="true"
>
  <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="3" opacity="0.25" />
  <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
</svg>

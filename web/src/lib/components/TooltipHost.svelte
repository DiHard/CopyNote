<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";

  type TooltipPlacement = "top" | "bottom";

  let host: HTMLDivElement | null = null;
  let bubble = $state<HTMLDivElement | null>(null);
  let active = $state<{ anchor: HTMLElement; text: string } | null>(null);
  let ready = $state(false);
  let layout = $state({
    left: 12,
    top: 12,
    arrowLeft: 12,
    placement: "bottom" as TooltipPlacement,
  });

  let showTimer: number | null = null;
  let frame: number | null = null;

  function triggerFrom(target: EventTarget | null): HTMLElement | null {
    if (!(target instanceof Element)) return null;
    const trigger = target.closest<HTMLElement>("[data-tooltip]");
    return trigger?.dataset.tooltip ? trigger : null;
  }

  function clearShowTimer(): void {
    if (showTimer !== null) {
      window.clearTimeout(showTimer);
      showTimer = null;
    }
  }

  function hide(trigger?: HTMLElement): void {
    clearShowTimer();
    if (!trigger || active?.anchor === trigger) {
      active = null;
      ready = false;
    }
  }

  function scheduleShow(trigger: HTMLElement): void {
    clearShowTimer();
    if (active?.anchor === trigger) return;

    showTimer = window.setTimeout(() => {
      showTimer = null;
      const text = trigger.dataset.tooltip?.trim();
      if (!text || !trigger.isConnected) return;
      ready = false;
      active = { anchor: trigger, text };
    }, 260);
  }

  function viewportSize(): { width: number; height: number } {
    return {
      width: document.documentElement.clientWidth || window.innerWidth,
      height: document.documentElement.clientHeight || window.innerHeight,
    };
  }

  function positionBubble(): void {
    if (!active || !bubble || !active.anchor.isConnected) {
      if (active && !active.anchor.isConnected) hide();
      return;
    }

    const anchorRect = active.anchor.getBoundingClientRect();
    const scroller = active.anchor.closest<HTMLElement>("[data-scroller]");
    if (scroller) {
      const scrollerRect = scroller.getBoundingClientRect();
      const visible =
        anchorRect.bottom > scrollerRect.top &&
        anchorRect.top < scrollerRect.bottom;
      if (!visible) {
        hide(active.anchor);
        return;
      }
    }

    const bubbleRect = bubble.getBoundingClientRect();
    const viewport = viewportSize();
    const margin = 12;
    const gap = 8;
    const spaceAbove = anchorRect.top - margin;
    const spaceBelow = viewport.height - anchorRect.bottom - margin;

    let placement: TooltipPlacement;
    if (spaceBelow >= bubbleRect.height + gap) {
      placement = "bottom";
    } else if (spaceAbove >= bubbleRect.height + gap) {
      placement = "top";
    } else {
      // If neither side has a full bubble's height (for example, one card
      // filling a shrink-wrapped window), choose the roomier side and clamp
      // the bubble into the viewport below.
      placement = spaceAbove >= spaceBelow ? "top" : "bottom";
    }

    const desiredTop =
      placement === "bottom"
        ? anchorRect.bottom + gap
        : anchorRect.top - bubbleRect.height - gap;
    const maxTop = Math.max(margin, viewport.height - bubbleRect.height - margin);
    const top = Math.max(margin, Math.min(desiredTop, maxTop));

    const desiredLeft = anchorRect.left + anchorRect.width / 2 - bubbleRect.width / 2;
    const maxLeft = Math.max(margin, viewport.width - bubbleRect.width - margin);
    const left = Math.max(margin, Math.min(desiredLeft, maxLeft));
    const anchorCenter = anchorRect.left + anchorRect.width / 2;
    const arrowLeft = Math.max(10, Math.min(bubbleRect.width - 10, anchorCenter - left));

    layout = { left, top, arrowLeft, placement };
    ready = true;
  }

  function refreshPosition(): void {
    if (frame !== null) return;
    frame = window.requestAnimationFrame(() => {
      frame = null;
      positionBubble();
    });
  }

  function onPointerOver(event: PointerEvent): void {
    if (event.pointerType === "touch") return;
    const trigger = triggerFrom(event.target);
    const previous = triggerFrom(event.relatedTarget);
    if (trigger && trigger !== previous) scheduleShow(trigger);
  }

  function onPointerOut(event: PointerEvent): void {
    if (event.pointerType === "touch") return;
    const trigger = triggerFrom(event.target);
    const next = triggerFrom(event.relatedTarget);
    if (trigger && trigger !== next) hide(trigger);
  }

  function onFocusIn(event: FocusEvent): void {
    const trigger = triggerFrom(event.target);
    if (trigger) scheduleShow(trigger);
  }

  function onFocusOut(event: FocusEvent): void {
    const trigger = triggerFrom(event.target);
    const next = triggerFrom(event.relatedTarget);
    if (trigger && trigger !== next) hide(trigger);
  }

  $effect(() => {
    const current = active;
    if (!current) return;
    void current.anchor;
    void current.text;

    void tick().then(() => {
      if (active !== current) return;
      window.requestAnimationFrame(positionBubble);
    });
  });

  onMount(() => {
    // The host lives outside the scroll container, so the bubble can never
    // be clipped by the list's overflow.
    if (host) document.body.appendChild(host);
    window.addEventListener("pointerover", onPointerOver);
    window.addEventListener("pointerout", onPointerOut);
    window.addEventListener("focusin", onFocusIn);
    window.addEventListener("focusout", onFocusOut);
    window.addEventListener("resize", refreshPosition);
    window.addEventListener("scroll", refreshPosition, true);
  });

  onDestroy(() => {
    clearShowTimer();
    if (frame !== null) window.cancelAnimationFrame(frame);
    window.removeEventListener("pointerover", onPointerOver);
    window.removeEventListener("pointerout", onPointerOut);
    window.removeEventListener("focusin", onFocusIn);
    window.removeEventListener("focusout", onFocusOut);
    window.removeEventListener("resize", refreshPosition);
    window.removeEventListener("scroll", refreshPosition, true);
    host?.remove();
  });
</script>

<div bind:this={host} data-tooltip-host>
  {#if active}
    <div
      bind:this={bubble}
      data-app-tooltip
      data-placement={layout.placement}
      data-ready={ready}
      aria-hidden="true"
      role="tooltip"
      style={"left: " + layout.left + "px; top: " + layout.top + "px; --tooltip-arrow-left: " + layout.arrowLeft + "px;"}
    >
      {active.text}
    </div>
  {/if}
</div>

<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";

  /**
   * One bubble for every element that carries `data-tooltip`.
   *
   * It appears when the pointer rests on such an element, and when the
   * element takes keyboard focus — `:focus-visible`, not any focus: focus put
   * back by script after a click (a closed dialog returning to its button)
   * would otherwise pop a hint next to something the user is not looking at.
   * `data-tooltip-mode="hover"` opts an element out of the focus part and
   * gives it a longer delay; it is for large targets like an entry card, where
   * the hint is about the mouse and would sit on top of the next card while
   * the user walks the list with the arrow keys.
   *
   * It goes away as soon as the user acts — a press, a key — and when the
   * window is put away (`suspended`).
   */
  let { suspended = false }: { suspended?: boolean } = $props();

  type TooltipPlacement = "top" | "bottom";

  const SHOW_DELAY_MS = 260;
  /** About what Windows waits before a native tooltip. */
  const HOVER_ONLY_DELAY_MS = 700;

  let host: HTMLDivElement | null = null;
  let bubble = $state<HTMLDivElement | null>(null);
  let active = $state<{ anchor: HTMLElement; text: string } | null>(null);
  let ready = $state(false);
  let layout = $state({
    left: 12,
    top: 12,
    arrowLeft: 12,
    placement: "bottom",
  });

  let showTimer: number | null = null;
  let frame: number | null = null;

  function triggerFrom(target: EventTarget | null): HTMLElement | null {
    if (!(target instanceof Element)) return null;
    const trigger = target.closest<HTMLElement>("[data-tooltip]");
    return trigger?.dataset.tooltip ? trigger : null;
  }

  function hoverOnly(trigger: HTMLElement): boolean {
    return trigger.dataset.tooltipMode === "hover";
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
    if (suspended || active?.anchor === trigger) return;

    const delay = hoverOnly(trigger) ? HOVER_ONLY_DELAY_MS : SHOW_DELAY_MS;
    showTimer = window.setTimeout(() => {
      showTimer = null;
      const text = trigger.dataset.tooltip?.trim();
      if (suspended || !text || !trigger.isConnected) return;
      ready = false;
      active = { anchor: trigger, text };
    }, delay);
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
    const maxTop = Math.max(
      margin,
      viewport.height - bubbleRect.height - margin,
    );
    const top = Math.max(margin, Math.min(desiredTop, maxTop));

    const desiredLeft =
      anchorRect.left + anchorRect.width / 2 - bubbleRect.width / 2;
    const maxLeft = Math.max(
      margin,
      viewport.width - bubbleRect.width - margin,
    );
    const left = Math.max(margin, Math.min(desiredLeft, maxLeft));
    const anchorCenter = anchorRect.left + anchorRect.width / 2;
    const arrowLeft = Math.max(
      10,
      Math.min(bubbleRect.width - 10, anchorCenter - left),
    );

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
    if (!trigger || hoverOnly(trigger)) return;
    // :focus-visible is the browser's own answer to "did the keyboard put
    // the focus here". It is on the element that took focus, which may be
    // inside the trigger.
    if (!(event.target as Element).matches(":focus-visible")) return;
    scheduleShow(trigger);
  }

  function onFocusOut(event: FocusEvent): void {
    const trigger = triggerFrom(event.target);
    const next = triggerFrom(event.relatedTarget);
    if (trigger && trigger !== next) hide(trigger);
  }

  /** The hint has done its job once the user acts on what it describes. */
  function onUserAction(): void {
    hide();
  }

  // The bubble shows what its element said when it appeared, and the pointer
  // can stay put while the element changes: the pin swaps "Pin" for "Unpin"
  // on a click, a filter removes a card, Escape leaves Settings. None of
  // that fires pointerout, so the bubble watches for it while it is up.
  $effect(() => {
    const current = active;
    if (!current) return;
    const observer = new MutationObserver(() => {
      if (active !== current) return;
      const { anchor } = current;
      const text = anchor.dataset.tooltip?.trim();
      if (!anchor.isConnected || !text || anchor.matches(":disabled")) {
        hide(anchor);
      } else if (text !== current.text) {
        ready = false;
        active = { anchor, text };
      }
    });
    observer.observe(document.body, {
      subtree: true,
      childList: true,
      attributes: true,
      attributeFilter: ["data-tooltip", "disabled"],
    });
    return () => observer.disconnect();
  });

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

  // A window being put away takes its hints with it: the pointer does not
  // leave anything when the window slides out from under it.
  $effect(() => {
    if (suspended) hide();
  });

  onMount(() => {
    // The host lives outside the scroll container, so the bubble can never
    // be clipped by the list's overflow.
    if (host) document.body.appendChild(host);
    window.addEventListener("pointerover", onPointerOver);
    window.addEventListener("pointerout", onPointerOut);
    window.addEventListener("focusin", onFocusIn);
    window.addEventListener("focusout", onFocusOut);
    window.addEventListener("pointerdown", onUserAction, true);
    window.addEventListener("keydown", onUserAction, true);
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
    window.removeEventListener("pointerdown", onUserAction, true);
    window.removeEventListener("keydown", onUserAction, true);
    window.removeEventListener("resize", refreshPosition);
    window.removeEventListener("scroll", refreshPosition, true);
    // The portal host was created by this component, outside Svelte's DOM tree.
    (host as HTMLDivElement | null)?.remove();
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
      style={"left: " +
        layout.left +
        "px; top: " +
        layout.top +
        "px; --tooltip-arrow-left: " +
        layout.arrowLeft +
        "px;"}
    >
      {active.text}
    </div>
  {/if}
</div>

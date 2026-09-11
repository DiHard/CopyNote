/** Keep keyboard focus inside a modal and restore the triggering control. */
export function modalFocus(node: HTMLElement) {
  const previous = document.activeElement as HTMLElement | null;
  const background = document.querySelector("main");
  const wasInert = background?.inert ?? false;
  if (background) background.inert = true;
  const controls = () => Array.from(node.querySelectorAll<HTMLElement>(
    'button:not(:disabled), input:not(:disabled), textarea:not(:disabled), select:not(:disabled), a[href], [tabindex="0"]',
  )).filter((el) => el.getClientRects().length > 0);
  const focusFirst = () => (node.querySelector<HTMLElement>("[data-initial-focus]") ?? controls()[0] ?? node).focus();
  focusFirst();
  function onKeydown(e: KeyboardEvent) {
    if (e.key !== "Tab") return;
    const items = controls();
    const first = items[0];
    const last = items[items.length - 1];
    if (!first) { e.preventDefault(); node.focus(); return; }
    if (e.shiftKey && (document.activeElement === first || document.activeElement === node)) {
      e.preventDefault(); last.focus();
    } else if (!e.shiftKey && (document.activeElement === last || document.activeElement === node)) {
      e.preventDefault(); first.focus();
    }
  }
  function onFocus(e: FocusEvent) {
    if (!node.contains(e.target as Node)) focusFirst();
  }
  node.addEventListener("keydown", onKeydown);
  document.addEventListener("focusin", onFocus);
  return { destroy() {
    node.removeEventListener("keydown", onKeydown);
    document.removeEventListener("focusin", onFocus);
    if (background) background.inert = wasInert;
    if (previous?.isConnected) previous.focus();
  } };
}

# UX harness

Opens the **real** frontend bundle in a browser with a fake Go bridge, so UI
work does not require launching `copynote.exe`.

```bash
cd web && npm run build && cd ..
node tools/uxharness/serve.mjs        # http://127.0.0.1:18099/
```

Both the bundle and the stub are re-read on every request: rebuild the
frontend, refresh the page.

## When to run the real app instead

`tools/testinstance` builds a test exe that runs beside the daily CopyNote
without touching it, and checks it for real. That is the tool for anything Go
decides: which global shortcut Windows actually accepted, what the tray does
during a cold start, the window's real position and focus.

This harness stays the faster loop for layout and frontend behaviour: no exe
build, no WebView2 start-up, and scenarios (`?empty`, `?many`, …) that would
otherwise need prepared data files.

## Why it works

`vite-plugin-singlefile` emits one self-contained `web/dist/index.html`, and
every Go binding is reached through `window.*`. Defining those globals in a
plain `<script>` in `<head>` — which runs before the bundle's deferred module
script — boots the genuine UI against fake data.

## Scenarios

Appended to the URL:

| Flag | State |
|---|---|
| *(none)* | eight sample entries, exe in a program folder |
| `?empty` | no entries — the onboarding empty screen |
| `?many` | 30 entries — list scrolling and the window-height clamp |
| `?downloads` | exe in Downloads — the relocate banner |
| `?update` | a signed release is available — the in-app update flow |
| `?hotkeytaken` | Windows refuses every global shortcut — the Settings error path |

`window.__harness.calls` records what the UI asked the bridge to do
(`resizeWindow` heights, `hide` count, copied ids, topmost/auto-hide values).
The window height the UI requests is also mirrored into `document.title`.

## Keeping the stub honest

`stub.js` must cover every entry of the `declare global { interface Window }`
block in `web/src/lib/api.ts`. That block is the authoritative list. A binding
the UI calls but the stub forgets fails as `undefined is not a function`,
often with no visible symptom.

## Harness artifacts that are not app bugs

Confirmed the hard way, in the Claude Code browser pane:

- **`document.hasFocus()` is false.** A programmatic `.focus()` moves
  `document.activeElement` but fires no `focus` event, so focus-driven logic
  looks broken. Test it with `el.dispatchEvent(new FocusEvent("focus"))`.
- **`requestAnimationFrame` starves** whenever the pane is not painting.
  Anything behind it — the auto-resize effect, Svelte-rendered modals — then
  appears frozen. A modal that would not close on Escape was this.
- **CSS transitions do not advance** either, for the same reason: after a
  class or theme change `getComputedStyle` keeps reporting the value the
  transition started from. Before measuring colours, inject
  `*, *::before, *::after { transition: none !important; animation: none !important }`.
- **Screenshots time out** with "did not finish rendering". Retry once, then
  fall back to reading the DOM, which keeps working.
- **Viewport width below 768 switches the pane to mobile emulation**: Android
  user agent and overlay scrollbars with a 0 px gutter. Measure scrollbar
  width at ≥ 768 instead. The app's real width is 420.

Before calling anything a regression, serve the previous bundle
(`git show HEAD:web/dist/index.html > /tmp/old.html`) through the same stub
and compare.

**What the stub cannot show.** It fakes the bridge's answers, not the Go
state behind them. Anything decided in Go — which global shortcut is really
registered after Windows refuses a new one, what the tray does during a cold
start, the window's actual size and position — needs code review or a real
run in `tools/testinstance`. The hotkey's restore-on-refusal path was found by review for exactly
this reason: here, a refused combination looks perfectly fine.

Layout measurements — `getBoundingClientRect`, `scrollHeight`, computed
styles, tab order, contrast ratios — stay trustworthy throughout. Those are
what this harness is for.

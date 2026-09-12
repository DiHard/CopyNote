# UX harness

Opens the **real** frontend bundle in a browser with a fake Go bridge, so UI
work does not require launching `copynote.exe`.

```bash
cd web && npm run build && cd ..
node tools/uxharness/serve.mjs        # http://127.0.0.1:18099/
```

Both the bundle and the stub are re-read on every request: rebuild the
frontend, refresh the page.

## Why not just run the app

A second CopyNote cannot be run beside the real one for UI work:

- `autorunValueName` (`"CopyNote"` in `internal/service/settings.go`) is a
  `const`, not `-X`-overridable like the mutex, tray class and SHOW message.
  A test instance whose isolated settings say `autorun: false` therefore
  **deletes the real installation's autorun entry**, and one that says `true`
  repoints it at the test binary.
- The singleton mutex means a second launch just surfaces the running window.

Fixing the first point is worth doing; until then, this.

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
- **Screenshots time out** with "did not finish rendering". Retry once, then
  fall back to reading the DOM, which keeps working.
- **Viewport width below 768 switches the pane to mobile emulation**: Android
  user agent and overlay scrollbars with a 0 px gutter. Measure scrollbar
  width at ≥ 768 instead. The app's real width is 420.

Before calling anything a regression, serve the previous bundle
(`git show HEAD:web/dist/index.html > /tmp/old.html`) through the same stub
and compare.

Layout measurements — `getBoundingClientRect`, `scrollHeight`, computed
styles, tab order, contrast ratios — stay trustworthy throughout. Those are
what this harness is for.

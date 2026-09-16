# Test instance

Builds CopyNote from the current sources under names of its own and checks,
in the real exe, what the browser harness in `tools/uxharness` cannot show:
what Go and Windows decide. It runs beside the daily CopyNote and leaves it
alone — why that takes four `-X` overrides is in CLAUDE.md, "Running a test
instance beside the real app".

Requires Windows, PowerShell 7 (`pwsh`), Go and Node 22 or newer.

```bash
pwsh tools/testinstance/all.ps1       # build, then every check below
pwsh tools/testinstance/build.ps1     # rebuild after changing Go code
pwsh tools/testinstance/hotkey.ps1    # one check at a time
```

The frontend goes in as committed in `web/dist`: after changing it, run
`npm run build` in `web/` first.

**While a check runs** a CopyNote window slides in and out in the corner of the
screen, a second icon comes and goes in the tray, and `hotkey.ps1` types
Ctrl+Alt+N and Ctrl+Alt+M. Don't type meanwhile.

| Script | Checks |
|---|---|
| `launch.ps1` | A launch by hand brings the window up with the caret in the search box; an `--autostart` launch stays out of sight until the tray icon is clicked |
| `hotkey.ps1` | Real keystrokes toggle the window and reopening clears the search; switching to another combination, `off` and back to the default; Windows refusing Win+V leaves Ctrl+Alt+N working; quitting releases it |
| `coldstart.ps1` | Tray clicks and the hotkey that arrive before the page has loaded bring the window up once the UI is ready, and only then; a second launch reaches the test build, not the daily CopyNote |
| `autorun.ps1` | Autorun writes and deletes `Run\CopyNoteTest` and never touches `Run\CopyNote` |
| `menu.ps1` | The entry context menu opens at the pointer, or under the card for the menu key; the window stays up while it is open; its keys skip the separator and disabled items; activation and focus return to the page; switching windows or the hotkey closes it; the tray icon's menu still works |
| `slide.ps1` | The window shrinks and grows with its bottom edge in the corner; it is put away when hidden mid-resize — Escape twice quickly, or right after typing — and after the hotkey twice in quick succession; reopened after a search it comes back at full height; Settings from the tray menu ends in the corner. Prints the window's positions while it reopens |
| `preparation.ps1` | With a deliberately delayed layout acknowledgement, quick reopening from search and Settings waits off-screen, starts with cleared state, and keeps a constant height during slide-in. Uses a fresh temporary profile per run |
| `trace.ps1` | Not a check: every change of the window's position and focus after a launch, for a window that appears and vanishes. `-FreshProfile` for a first run, `-WithForegroundRights` for a launch the way Explorer does it |

Each check prints `[ok  ]` or `[FAIL]` lines and exits with its number of
failures. The exe, data, log and WebView2 profile live in
`%TEMP%\copynote-testinstance`; delete it whenever you like. Every check starts
from fresh settings, the profile stays warm.

`hotkey.ps1` stops at once if another program already holds Ctrl+Alt+N or
Ctrl+Alt+M — a daily CopyNote that has the hotkey does.

## How it works

- `build.ps1` passes the overrides, then looks for each name in the binary:
  `-X` ignores a constant or a mistyped name without a word.
- `common.ps1` starts the exe with scratch `APPDATA`/`LOCALAPPDATA` and
  DevTools on port 9223, set around that one launch only — a `go build` from the
  same shell would otherwise lose the real module cache and Go settings.
- `probe.cs`, compiled by `Add-Type`, is the Win32 side: it finds the tray
  window (`FindWindowEx(HWND_MESSAGE, …)`) and the main window, reads where the
  window is (parked means a left edge at −30000), probes a combination with
  `RegisterHotKey` (error 1409: someone holds it), types it with `keybd_event`
  only once the probe says it is held, so keys never land in another window,
  and posts tray and hotkey messages. It also finds the popup menu window
  (`CopyNotePopupMenu`) and its owner, and forces the foreground onto a window
  (`ForceForeground`) for checks that need one active: Windows does not count
  DevTools input as input.
- `cdp.mjs` evaluates an expression in the page over DevTools. That reaches the
  Go bridge too: `window.applyHotkey(…)`, `window.saveSettings(…)`. With
  `--send` (`Send-PageCommand`) it sends one raw DevTools command instead:
  `Input.dispatchMouseEvent` and `Input.dispatchKeyEvent` reach the page as
  trusted input without moving the pointer or typing into another window.
- The test build quits the way its tray does, with `WM_QUIT` on the tray
  window; the next launch waits for its `msedgewebview2.exe` processes to
  release the DevTools port.
- Cold start: the request is posted the moment the tray window exists, which is
  before the page's `DOMContentLoaded`. `performance.timeOrigin` puts that
  event on the same wall clock as the post.

Not covered: the tray icon's hover text. Reading it means opening the hidden
icons panel.

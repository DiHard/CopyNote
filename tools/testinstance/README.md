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

| Script            | Checks                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `launch.ps1`      | A launch by hand brings the window up with the caret in the search box; an `--autostart` launch stays out of sight until the tray icon is clicked                                                                                                                                                                                                                                                                                                                                                    |
| `hotkey.ps1`      | Real keystrokes toggle the window, and text that reached the parked window is gone when it opens; switching to another combination, `off` and back to the default; Windows refusing Win+V leaves Ctrl+Alt+N working; quitting releases it                                                                                                                                                                                                                                                            |
| `coldstart.ps1`   | Tray clicks and the hotkey that arrive before the page has loaded bring the window up once the UI is ready, and only then; a second launch reaches the test build, not the daily CopyNote                                                                                                                                                                                                                                                                                                            |
| `autorun.ps1`     | Autorun writes and deletes `Run\CopyNoteTest` and never touches `Run\CopyNote`                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `menu.ps1`        | The entry context menu opens at the pointer, or under the card for the menu key; the window stays up while it is open; its keys skip the separator and disabled items; activation and focus return to the page; switching windows closes it; the hotkey with the menu open puts the window away, and hiding the window by any means takes the menu along; the tray icon's menu still works                                                                                                           |
| `slide.ps1`       | The window shrinks and grows with its bottom edge in the corner; it is put away when hidden mid-resize — Escape twice quickly, or right after typing — and comes back when asked for while still sliding away, or after the hotkey twice in quick succession; reopened after a search it comes back at full height; a parked window takes no keystrokes, and what did reach it is gone when it opens; Settings from the tray menu ends in the corner. Prints the window's positions while it reopens |
| `preparation.ps1` | With the page's answer held back, a window asked for right after hiding waits off-screen for it, opens on the list with an empty search — from a search and from Settings — and keeps one height through the slide-in. With the answer never sent it opens all the same once Go stops waiting (500 ms), and goes on opening afterwards                                                                                                                                                               |
| `trace.ps1`       | Not a check: every change of the window's position and focus after a launch, for a window that appears and vanishes. `-FreshProfile` for a first run, `-WithForegroundRights` for a launch the way Explorer does it                                                                                                                                                                                                                                                                                  |

Each check prints `[ok  ]`, `[FAIL]` or `[skip]` lines and exits with its number
of failures; `all.ps1` adds them up and says how many checks were skipped. The
exe, data, log and WebView2 profile live in `%TEMP%\copynote-testinstance`;
delete it whenever you like. Every check starts from fresh settings, the profile
stays warm.

A skipped check is one that could not mean anything in this session, and both a
pass and a failure would be wrong:

- **The screen is locked** (someone is following from another device). No
  window can become the active one and typed keys go nowhere, so the checks
  about activation, focus and real keystrokes are skipped. The app is right to
  treat its window as not in use then — which is also why these checks used to
  fail in a locked session. Run the suite unlocked for a full answer.
- **Another program holds Ctrl+Alt+N or Ctrl+Alt+M** — a daily CopyNote that
  has the hotkey does. `hotkey.ps1` then checks nothing: it is not its place to
  take the combination away. Quit the daily CopyNote and run it again.

`all.ps1` also reads the daily CopyNote's `Run` entry before and after the run
and fails if it changed.

## How it works

- `build.ps1` passes the overrides, then looks for each name in the binary:
  `-X` ignores a constant or a mistyped name without a word. It builds with
  `-trimpath`, because otherwise Go records the whole `-ldflags` string in the
  binary and every name is found there whether or not its override took.
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
- The test build is quit by ending its tray's message loop, with `WM_QUIT` on
  the tray window; the rest is the app's ordinary shutdown. The next launch
  waits for its `msedgewebview2.exe` processes to release the DevTools port.
- Cold start: the request is posted the moment the tray window exists, which is
  before the page's `DOMContentLoaded`. `performance.timeOrigin` puts that
  event on the same wall clock as the post.

Not covered: the tray icon's hover text. Reading it means opening the hidden
icons panel.

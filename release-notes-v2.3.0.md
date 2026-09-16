## CopyNote v2.3.0

### New features

- Open or hide CopyNote from any app with **Ctrl+Alt+N**. Choose your own
  global shortcut or turn it off in Settings; CopyNote reports when another
  program has already claimed the combination.
- Right-click an entry, press the menu key or **Shift+F10**, or long-press a
  card to open its context menu: copy, edit, delete, move up or move down.
- Navigate the list with fewer keystrokes: **Tab** from search goes straight
  to the entries, and the whole list uses one Tab stop. **F2** edits the
  focused entry, **Delete** opens its delete confirmation, **Home/End** jump
  to the first or last entry, and **Ctrl+↑/↓** reorder entries when search is
  empty. Settings now includes a keyboard shortcut reference.
- A new empty screen explains what to save and where to find CopyNote in the
  tray. After the first entry, a dismissible hint explains how to copy it.
  When a search has no matches, create an entry with the search text already
  filled in as its label.
- Import now reports how many entries were added and how many duplicates
  were skipped. Settings also explains how import, export, startup, theme,
  language and update checks work, with an explicit **System** language option.

### Bug fixes

- Fixed the window getting stuck on screen when resizing interrupted its
  closing animation, or when it was quickly reopened. Escape, the close
  button and clicking outside no longer leave it stranded in that state.
- Fixed relaunching after an update or a move to another folder: the new
  process waits for the previous one to exit before initializing WebView2.
- Search filters without the previous input delay. Entry dialogs open
  immediately when the window already has enough room for them.
- Entry forms grow with their content and scroll when needed, keeping fields
  and buttons reachable even after enlarging the text area.
- Improved text contrast, icon button hit areas, focus visibility and screen
  reader labels. Settings toggles now expose their checked state to assistive
  technology.
- Failed copying now explains when another program is using the clipboard.
  The copy-status badge no longer covers an entry's edit and delete buttons.

### Changes

- The offer to move CopyNote out of Downloads, including the move controls
  in Settings, is temporarily disabled while antivirus detections related to
  copying the executable are investigated. **Open app folder** remains
  available.

### Internal

- Added an isolated test-instance workflow and a browser UI harness for
  checking startup, hotkeys, context menus and window animations without
  interfering with the installed CopyNote.
- Added regression coverage for hotkey parsing, keyboard focus, import
  results, clipboard errors, popup menus and window behavior.

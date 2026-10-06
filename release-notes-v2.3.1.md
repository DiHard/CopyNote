## CopyNote v2.3.1

### Bug fixes

- Fixed the window refusing to open again until CopyNote was restarted. After
  hiding, the window waited for the page to confirm it was ready and could
  wait forever if that confirmation was lost; it now opens after half a
  second regardless.
- Text typed or pasted right after closing the window no longer ends up in
  CopyNote's search box. The hidden window keeps keyboard focus until you
  click elsewhere; it now ignores those keystrokes, and the search is always
  empty when the window opens.
- The global shortcut now closes the window even while an entry's context
  menu is open. Before, the first press only closed the menu.
- With the window pinned, clicking the tray icon closes it also when the icon
  sits in the hidden icons panel. Before, the click only brought the window
  forward.
- After editing or deleting an entry with the mouse, keyboard focus returns
  to the entry's card instead of an invisible button, so the arrow keys,
  **F2** and **Delete** keep working.
- A drag or long press that was interrupted no longer swallows the next click
  on a card.
- Restarting after an update no longer pauses for about 15 seconds before the
  new version appears. The pause still happens once when updating from
  v2.3.0, because the old version performs that restart.
- Screen readers now announce an available update on the Settings button, and
  the pin button keeps one name while its pressed state changes.

### Changes

- Hints on buttons and entries are now CopyNote's own tooltips instead of the
  system ones. They stay inside the window, appear on hover or keyboard
  focus, and disappear as soon as you click, press a key or the window hides.

### Internal

- The window's show/hide logic is now a single state machine with unit tests.
  The test-instance scripts cover the new cases and report checks they cannot
  run (locked screen, shortcut held by another program) as skipped.
- Moving CopyNote out of Downloads stays disabled, now behind one switch
  instead of commented-out code.
- Quality checks cover only tracked files and verify downloaded tools against
  pinned SHA-256 hashes; CI caches Go, npm and the tools between runs.
- The single-file frontend build escapes `<!--` inside the inlined script.

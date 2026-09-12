## CopyNote v2.2.0

### New features
- The window now opens on its own when you start CopyNote yourself. A Windows
  sign-in still starts it quietly into the tray.
- Keyboard flow: the search box takes focus when the window opens, **Enter**
  copies the top match and hides the window, **↑/↓** move through entries, and
  **Escape** clears the search before it hides anything.
- New setting **Hide when clicking outside**. Turn it off and the window stays
  in front of the app you are working in — together with **Always on top** you
  can paste into several fields in a row without going back to the tray.
- Clicking the tray icon while CopyNote is still starting no longer does
  nothing: the window opens as soon as the interface is ready. The icon's
  tooltip now says whether it is loading or ready.
- The "move me out of Downloads" banner can be postponed with **Remind me
  later** instead of only being dismissed forever, and it waits until you have
  at least one entry.
- The new-entry form now says that leaving the value empty copies the label.

### Bug fixes
- Long lists are no longer cut off. The entry list scrolls on its own and the
  search box stays put; previously, past roughly the eleventh entry on a scaled
  laptop, the header scrolled out of reach and no scrollbar was shown. The
  Settings screen had the same problem.
- Dragging an entry to the top or bottom edge of a long list now scrolls it.
- Reopening the window no longer shows the previous session's search text, and
  closing it from Settings no longer reopens into Settings.
- **Always on top** and **Hide when clicking outside** now explain what they do
  and what they cost.
- Better contrast on the relocate banner's buttons.

### Internal
- The autorun registry entry now carries an `--autostart` flag so the app can
  tell a Windows sign-in from a launch you performed yourself. Existing
  installations rewrite the entry themselves on the next start.

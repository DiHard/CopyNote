## CopyNote v2.4.0 — macOS

### New features

- **CopyNote now runs on macOS 13 and later**, on Apple silicon and Intel
  Macs. It lives in the menu bar and has no Dock icon. Your entries,
  settings, themes, languages, import and export work the same as on Windows,
  and a backup moves between the two platforms unchanged.
- The Mac version uses Mac conventions. The global shortcut is **⌃⌥N** by
  default. **⌘E** edits the focused entry, **⌘⌫** deletes it, and **⌘↑/↓**
  reorders entries. The entry menu shows these keys, and texts in Settings
  mention the menu bar and Finder instead of the tray and Explorer.
- Updates on macOS install from Settings, as on Windows. CopyNote downloads
  the new version, checks its signature, replaces itself and restarts.
- If CopyNote is opened straight from Downloads, it asks you to move it to
  Applications first. Otherwise updates cannot replace it and starting
  CopyNote at login may stop working. **Show in Finder** shows the app so you
  can drag it to Applications, and **Not now** opens CopyNote as usual.
  CopyNote never moves itself.

### Installing on macOS

1. Download `CopyNote-macos.zip` and open it.
2. Drag **CopyNote** to **Applications** and open it.
3. CopyNote is not notarized by Apple, so macOS says it cannot verify the
   developer. Open **System Settings → Privacy & Security**, click
   **Open Anyway** next to the CopyNote message, and confirm.

You only need to do this once. Updates installed from inside CopyNote do not
ask again.

### Internal

- Windows and macOS now share one update flow. Each platform has its own
  signed release asset: `copynote.exe` for Windows and `CopyNote-macos.zip`
  for macOS.
- CI checks the macOS build: it lints, vets and tests the code, builds the
  app bundle, and verifies the committed frontend build.
- Updated the Go toolchain to 1.26.10 and the frontend build tools to fix
  reported vulnerabilities.

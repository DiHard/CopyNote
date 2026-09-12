## CopyNote v2.1.0

### New features
- One-click updates: Settings installs a new release in place — it downloads the signed build, verifies its signature and restarts. No installer, no administrator rights.
- CopyNote offers to move itself out of the Downloads folder into a permanent location, so a Windows disk cleanup cannot delete it and startup keeps working. Accept the default per-user program folder or pick your own.
- A new "Open app folder" button in Settings shows where the running executable lives.

### Bug fixes
- The window is sharp on displays scaled above 100%, and follows the scaling of the monitor it appears on.
- Launching CopyNote a second time reliably brings the existing window back.

### Internal
- Release binaries are signed with an ed25519 key; the in-app updater refuses anything that does not verify.
- Added coverage for the update install flow, the move-to-a-permanent-folder flow and per-monitor DPI handling.

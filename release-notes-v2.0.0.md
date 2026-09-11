## CopyNote v2.0.0

### New features
- Settings and snippets are now saved together in a single consistent data snapshot.
- CopyNote keeps the previous valid snapshot and can offer recovery if the data file becomes unreadable.
- Create, edit, and delete dialogs now provide improved keyboard navigation and focus handling.

### Bug fixes
- Prevented failed saves from leaving in-memory data out of sync with the saved file.
- Import now validates backups before making changes and reports cancelled import/export actions correctly.
- Update checks no longer block the application interface on a slow network.
- Fixed settings changes being lost when several preferences are changed quickly.

### Internal
- Added automated checks for Go, frontend state, accessibility diagnostics, and Windows builds.
- Added coverage for failed writes, data recovery, backup imports, settings migration, and update checks.

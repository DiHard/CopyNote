## CopyNote v1.1.0

### New features
- **Automatic update check.** On startup the app quietly checks GitHub Releases for a newer version. If one is found, a notification appears in the Settings → About section with a link to the release page. The check can be disabled in Settings. You can also trigger it manually at any time.

### Bug fixes
- Fixed a rare crash where a dead tray goroutine left a zombie process running silently in the background. The app now terminates cleanly if the tray thread exits unexpectedly.
- Fixed autorun breaking when the executable is moved to a different path. The registry entry is now self-healed on every launch — if autorun is enabled and the stored path no longer matches the current location, it is updated automatically.

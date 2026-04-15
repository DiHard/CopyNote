## CopyNote v1.0.1

### Bug fixes
- Fixed a regression where, on some Windows 11 machines, the window would fail to appear on the first tray click (requiring 2–3 clicks to toggle visible). Caused by DWM returning invalid `DWMWA_EXTENDED_FRAME_BOUNDS` values for windows parked far off-screen, which made the slide-up animation slide down instead. The invisible-border compensation is now clamped to a safe range.

### Internal
- Introduced a single source of truth for the application version (`internal/version`). The About section now reads the version from the Go binary instead of hardcoding it.

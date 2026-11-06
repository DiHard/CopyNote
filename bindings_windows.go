package main

import (
	"errors"
	"path/filepath"

	"github.com/jchv/go-webview2"

	"copynote/internal/bridge"
	"copynote/internal/service"
	"copynote/internal/tray"
	"copynote/internal/winutil"
)

func bindApplication(w webview2.WebView, hwnd uintptr, svc *service.Service, exePath, dataDir string, tr *tray.Tray) *bridge.Async {
	// 7. Entries, settings, version, export and import.
	const fileFilter = "CopyNote Backup (*.json)|*.json|All Files|*.*"
	bindData(w, svc,
		func() (string, bool) { return winutil.SaveFileDialog(hwnd, fileFilter, backupFileName) },
		func() (string, bool) { return winutil.OpenFileDialog(hwnd, fileFilter) })
	mustBind(w, "hide", func() {
		w.Dispatch(func() {
			moveOffScreen(hwnd)
		})
	})
	mustBind(w, "resizeWindow", func(contentHeight int) {
		w.Dispatch(func() {
			resizeToContent(hwnd, contentHeight)
		})
	})

	mustBind(w, "openExternal", func(url string) {
		winutil.OpenURL(url)
	})

	// The app is portable, and a self-update leaves its .old fallback next to
	// the exe, so the folder is worth a way in from the UI.
	mustBind(w, "openAppFolder", func() error {
		if exePath == "" {
			return errors.New("executable path is unavailable")
		}
		return winutil.OpenFolder(filepath.Dir(exePath))
	})

	// Update checks and the self-update flow run on background workers;
	// see update_windows.go.
	updates := bridge.NewAsync(w)
	bindUpdates(w, updates, svc, exePath, func() { w.Dispatch(func() { w.Terminate() }) })

	// Moving the executable out of a download folder; see relocate_windows.go,
	// which also holds the switch.
	if relocationEnabled {
		bindRelocate(w, svc, hwnd, exePath, dataDir)
	}

	// The entry list's context menu; see entrymenu_windows.go.
	bindEntryMenu(w, hwnd)

	// Read on every focus loss, so a plain atomic store is all it takes.
	mustBind(w, "applyAutoHide", func(enabled bool) {
		autoHideDisabled.Store(!enabled)
	})

	// Unlike the other preferences this one can be refused by Windows — the
	// combination may already belong to another program — so the error goes
	// back to the UI instead of into the log.
	mustBind(w, "applyHotkey", func(setting string) error {
		return tr.SetHotkey(setting)
	})

	mustBind(w, "applyTopmost", func(enabled bool) {
		topmostEnabled.Store(enabled)
		w.Dispatch(func() {
			zOrder := winutil.HWND_NOTOPMOST
			if enabled {
				zOrder = winutil.HWND_TOPMOST
			}
			winutil.SetWindowPos(hwnd, zOrder, 0, 0, 0, 0,
				winutil.SWP_NOMOVE|winutil.SWP_NOSIZE|winutil.SWP_NOACTIVATE)
		})
	})

	return updates
}

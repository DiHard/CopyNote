package main

import (
	"copynote/internal/bridge"
	"copynote/internal/service"
	"copynote/internal/storage"
	"copynote/internal/version"
	"copynote/internal/winutil"
	"fmt"
	"github.com/jchv/go-webview2"
	"log"
	"os"
)

func bindApplication(w webview2.WebView, hwnd uintptr, svc *service.Service, exePath string) *bridge.Async {
	// 7. Bind CRUD bridge methods.
	mustBind := func(name string, fn any) {
		if err := w.Bind(name, fn); err != nil {
			log.Fatalf("bind %s: %v", name, err)
		}
	}
	mustBind("list", svc.List)
	mustBind("create", svc.Create)
	mustBind("update", svc.Update)
	mustBind("remove", svc.Delete) // "delete" is a JS operator, use "remove"
	mustBind("reorder", svc.Reorder)
	mustBind("copy", svc.Copy)
	mustBind("hide", func() {
		w.Dispatch(func() {
			moveOffScreen(hwnd)
		})
	})
	mustBind("getSettings", svc.GetSettings)
	mustBind("saveSettings", svc.SaveSettings)
	mustBind("resizeWindow", func(contentHeight int) {
		w.Dispatch(func() {
			resizeToContent(hwnd, contentHeight)
		})
	})

	mustBind("openExternal", func(url string) {
		winutil.OpenURL(url)
	})

	mustBind("getVersion", func() string {
		return version.Version
	})

	// Update checks and the self-update flow run on background workers;
	// see update_windows.go.
	updates := bridge.NewAsync(w)
	bindUpdates(w, updates, svc, exePath)

	mustBind("applyTopmost", func(enabled bool) {
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

	const fileFilter = "CopyNote Backup (*.json)|*.json|All Files|*.*"

	mustBind("exportData", func() (bool, error) {
		data, err := svc.ExportData()
		if err != nil {
			return false, err
		}
		path, ok := winutil.SaveFileDialog(hwnd, fileFilter, "copynote-backup.json")
		if !ok {
			return false, nil // user cancelled
		}
		err = storage.WriteAtomic(path, data)
		return err == nil, err
	})

	mustBind("importData", func() (bool, error) {
		path, ok := winutil.OpenFileDialog(hwnd, fileFilter)
		if !ok {
			return false, nil // user cancelled
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return false, fmt.Errorf("read file: %w", err)
		}
		if err := svc.ImportData(raw); err != nil {
			return false, err
		}
		return true, nil
	})

	return updates
}

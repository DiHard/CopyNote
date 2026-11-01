package main

import (
	"fmt"
	"log"
	"os"

	"copynote/internal/service"
	"copynote/internal/storage"
	"copynote/internal/version"
)

// binder is the one thing the shared bindings need from a platform's web
// view: go-webview2 on Windows, macui on macOS.
type binder interface {
	Bind(name string, fn any) error
}

func mustBind(b binder, name string, fn any) {
	if err := b.Bind(name, fn); err != nil {
		log.Fatalf("bind %s: %v", name, err)
	}
}

// backupFileName is what the export dialog suggests.
const backupFileName = "copynote-backup.json"

// fileDialog asks the user for a file; ok is false when they cancel.
type fileDialog func() (path string, ok bool)

// bindData binds what is the same on every platform: the entries, the
// settings, the version, and export and import around the platform's own
// file dialogs.
func bindData(b binder, svc *service.Service, saveDialog, openDialog fileDialog) {
	mustBind(b, "list", svc.List)
	mustBind(b, "create", svc.Create)
	mustBind(b, "update", svc.Update)
	mustBind(b, "remove", svc.Delete) // "delete" is a JS operator, use "remove"
	mustBind(b, "reorder", svc.Reorder)
	mustBind(b, "copy", svc.Copy)
	mustBind(b, "getSettings", svc.GetSettings)
	mustBind(b, "saveSettings", svc.SaveSettings)
	mustBind(b, "getVersion", func() string {
		return version.Version
	})

	mustBind(b, "exportData", func() (bool, error) {
		data, err := svc.ExportData()
		if err != nil {
			return false, err
		}
		path, ok := saveDialog()
		if !ok {
			return false, nil // user cancelled
		}
		err = storage.WriteAtomic(path, data)
		return err == nil, err
	})

	// Resolves to null when the dialog is cancelled, which the page tells apart
	// from an import that added nothing.
	mustBind(b, "importData", func() (*service.ImportResult, error) {
		path, ok := openDialog()
		if !ok {
			return nil, nil // user cancelled
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		result, err := svc.ImportData(raw)
		if err != nil {
			return nil, err
		}
		return &result, nil
	})
}

package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"copynote/internal/service"
	"copynote/internal/storage"
)

// What every platform's startup shares. Each platform supplies three
// functions of its own: logDir, systemLocale ("ru" or anything else) and
// messageBox, which shows text in a native dialog and, when ask is set,
// reports whether the user agreed.

// Single-file frontend produced by Vite and web/singlefile.ts.
//
//go:embed all:web/dist
var distFS embed.FS

// startedByAutorun reports whether the sign-in autorun entry started this
// process rather than the user. autorun.SetEnabled puts the flag into the
// registered command; anything without it — a double click, a relaunch after
// an update or a move — is the user asking for the app, and the window should
// come up on its own.
func startedByAutorun() bool {
	return slices.Contains(os.Args[1:], service.AutostartFlag)
}

func initializeLogging() func() {
	dir := logDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return func() {}
	}
	path := filepath.Join(dir, "copynote.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		// Keep one previous log; rotation failure must not block startup.
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return func() {}
	}
	log.SetOutput(f)
	return func() { _ = f.Close() }
}

func fatalStartup(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	log.Print(message)
	prefix := "CopyNote could not start. Your data has not been reset.\n\n"
	if systemLocale() == "ru" {
		prefix = "Не удалось запустить CopyNote. Ваши данные не были сброшены.\n\n"
	}
	messageBox(prefix+message, false)
	os.Exit(1)
}

func openService(path string, deps service.Dependencies) (*service.Service, error) {
	svc, err := service.NewWithDependencies(path, deps)
	if err == nil {
		return svc, nil
	}
	log.Printf("load data: %v", err)
	// Load treats a missing file as a new store, so check backup existence first.
	if _, statErr := os.Stat(path + ".bak"); statErr != nil {
		return nil, err
	}
	if _, backupErr := storage.Load(path + ".bak"); backupErr != nil {
		return nil, err
	}
	prompt := "CopyNote could not read its data file. Restore the previous saved version? The original file will be preserved.\n\n"
	if systemLocale() == "ru" {
		prompt = "Не удалось прочитать данные CopyNote. Восстановить предыдущую сохранённую версию? Исходный файл будет сохранён отдельно.\n\n"
	}
	if !messageBox(prompt+path, true) {
		return nil, err
	}
	if restoreErr := storage.RestoreBackup(path); restoreErr != nil {
		return nil, restoreErr
	}
	log.Print("restored previous data snapshot")
	return service.NewWithDependencies(path, deps)
}

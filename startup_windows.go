package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"copynote/internal/service"
	"copynote/internal/storage"
	"copynote/internal/winutil"
)

func initializeLogging() func() {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "CopyNote")
	if os.Getenv("LOCALAPPDATA") == "" {
		dir = filepath.Join(os.TempDir(), "CopyNote")
	}
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
	if winutil.SystemLocale() == "ru" {
		prefix = "Не удалось запустить CopyNote. Ваши данные не были сброшены.\n\n"
	}
	winutil.MessageBox(prefix+message, false)
	os.Exit(1)
}

func openService(path string) (*service.Service, error) {
	svc, err := service.New(path)
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
	if winutil.SystemLocale() == "ru" {
		prompt = "Не удалось прочитать данные CopyNote. Восстановить предыдущую сохранённую версию? Исходный файл будет сохранён отдельно.\n\n"
	}
	if !winutil.MessageBox(prompt+path, true) {
		return nil, err
	}
	if restoreErr := storage.RestoreBackup(path); restoreErr != nil {
		return nil, restoreErr
	}
	log.Print("restored previous data snapshot")
	return service.New(path)
}

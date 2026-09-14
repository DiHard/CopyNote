package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/sys/windows"

	"copynote/internal/service"
	"copynote/internal/storage"
	"copynote/internal/winutil"
)

const waitForPIDFlag = "--wait-for-pid"

// waitForRelaunchParent is used by a relaunched copy before it creates
// WebView2. The old process must exit first: WebView2 serializes access to
// its user-data directory, and starting both processes at once makes the new
// copy fail during controller creation.
func waitForRelaunchParent() {
	for i := 1; i+1 < len(os.Args); i++ {
		if os.Args[i] != waitForPIDFlag {
			continue
		}
		pid, err := strconv.ParseUint(os.Args[i+1], 10, 32)
		if err != nil || pid == 0 {
			return
		}
		h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			// The parent may have exited before this process opened its handle.
			return
		}
		defer windows.CloseHandle(h)
		_, _ = windows.WaitForSingleObject(h, windows.INFINITE)
		return
	}
}

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

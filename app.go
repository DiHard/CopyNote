package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

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

// waitForPIDFlag carries the PID of the copy that relaunched this one after
// an update; the new copy waits for it to exit before it starts.
const waitForPIDFlag = "--wait-for-pid"

// relaunchParentWait bounds how long a relaunched copy waits for the copy
// that started it. That copy has only its deferred cleanup left to do, so
// one still there after this long is stuck — and waiting for it without a
// limit left the user with no CopyNote at all and nothing on screen to say
// why.
const relaunchParentWait = 30 * time.Second

// relaunchParentPID finds the process a relaunched copy was told to wait for.
func relaunchParentPID(args []string) (uint32, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != waitForPIDFlag {
			continue
		}
		pid, err := strconv.ParseUint(args[i+1], 10, 32)
		return uint32(pid), err == nil && pid != 0
	}
	return 0, false
}

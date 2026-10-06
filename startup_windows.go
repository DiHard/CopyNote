package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/windows"

	"copynote/internal/service"
	"copynote/internal/storage"
	"copynote/internal/winutil"
)

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

// waitForRelaunchParent is used by a relaunched copy before it creates
// WebView2. The old process should be gone first: until it is, its WebView2
// still has the user-data directory open, and starting both at once has made
// the new copy fail during controller creation.
func waitForRelaunchParent() {
	pid, ok := relaunchParentPID(os.Args[1:])
	if !ok {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		// The parent may have exited before this process opened its handle.
		return
	}
	defer func() { _ = windows.CloseHandle(h) }() // The wait below has already produced its result.
	if startedAfter(h, windows.CurrentProcess()) {
		// The parent is gone and Windows has given its PID to something new.
		log.Printf("relaunch: process %d is not the copy that started this one; not waiting", pid)
		return
	}
	event, err := windows.WaitForSingleObject(h, uint32(relaunchParentWait/time.Millisecond))
	switch {
	case err != nil:
		log.Printf("relaunch: waiting for process %d: %v", pid, err)
	case event == uint32(windows.WAIT_TIMEOUT):
		log.Printf("relaunch: process %d is still running after %v; starting anyway", pid, relaunchParentWait)
	}
}

// startedAfter reports whether process a was created later than process b.
// False when either creation time cannot be read.
func startedAfter(a, b windows.Handle) bool {
	created := func(h windows.Handle) (int64, bool) {
		var creation, exit, kernel, user windows.Filetime
		if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
			return 0, false
		}
		return creation.Nanoseconds(), true
	}
	ta, okA := created(a)
	tb, okB := created(b)
	return okA && okB && ta > tb
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
	return service.NewWithDependencies(path, deps)
}

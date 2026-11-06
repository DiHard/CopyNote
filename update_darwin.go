package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// appBundle is the .app bundle holding exePath — what an update replaces —
// or "" for a binary started outside one, which then never updates itself.
func appBundle(exePath string) string {
	if bundle, _, found := strings.Cut(exePath, ".app/Contents/MacOS/"); found {
		return bundle + ".app"
	}
	return ""
}

// relaunch starts the executable of the replaced bundle, as on Windows,
// in a session of its own so it outlives this process. It inherits this
// process's environment, which `open` would not pass on. The new copy waits
// for this one to exit before it takes the single-instance lock, then shows
// its window like any launch by hand.
func relaunch(bundle string) {
	exe := filepath.Join(bundle, "Contents", "MacOS", "CopyNote")
	cmd := exec.Command(exe, waitForPIDFlag, strconv.Itoa(os.Getpid()))
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		log.Printf("relaunch after update: %v", err)
		return
	}
	log.Printf("relaunched %s as pid %d", exe, cmd.Process.Pid)
	_ = cmd.Process.Release()
}

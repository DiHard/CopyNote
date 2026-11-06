package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// relaunch starts the replaced executable. ShowOnStart opens its window once
// it is ready. The child waits for this process to
// exit before initializing WebView2, because both processes cannot safely
// use the same WebView2 user-data directory at the same time.
func relaunch(exePath string) {
	cmd := exec.Command(exePath, waitForPIDFlag, strconv.Itoa(os.Getpid()))
	cmd.Dir = filepath.Dir(exePath)
	if err := cmd.Start(); err != nil {
		log.Printf("relaunch after update: %v", err)
		return
	}
	log.Printf("relaunched %s as pid %d", exePath, cmd.Process.Pid)
	_ = cmd.Process.Release()
	// Return immediately so the child can finish waiting for this process.
}

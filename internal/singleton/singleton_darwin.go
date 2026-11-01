// Package singleton makes sure only one CopyNote runs per user, and lets a
// second launch ask the running one to show its window.
//
// On macOS the lock is an flock on ~/Library/Caches/<name>.lock — released by
// the kernel when the process ends, however it ends — and the request travels
// over a Unix socket beside it. Opening CopyNote.app a second time does not
// get here at all: LaunchServices activates the running copy instead. This is
// for the binary started some other way, such as by launchd at sign-in.
package singleton

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// dir holds the lock and the socket. A var so tests can use a scratch folder.
var dir = func() (string, error) { return os.UserCacheDir() }

func paths(name string) (lock, socket string, err error) {
	d, err := dir()
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", "", err
	}
	socket = filepath.Join(d, name+".sock")
	if len(socket) > maxSocketPath {
		// A Unix socket path is limited to 104 bytes; a long home folder
		// does not leave room under Caches. The per-user temporary folder
		// is shorter, and the user ID keeps it apart if that is /tmp.
		socket = filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d.sock", name, os.Getuid()))
	}
	return filepath.Join(d, name+".lock"), socket, nil
}

// maxSocketPath leaves room for the terminating zero of sun_path[104].
const maxSocketPath = 103

// Acquire takes the lock for name. already is true when another process
// holds it; release is always safe to call.
func Acquire(name string) (release func(), already bool, err error) {
	lockPath, _, err := paths(name)
	if err != nil {
		return func() {}, false, err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return func() {}, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return func() {}, true, nil
		}
		return func() {}, false, fmt.Errorf("flock %s: %w", lockPath, err)
	}
	return func() { _ = f.Close() }, false, nil // closing the file drops the lock
}

// Listen calls onShow, on its own goroutine, whenever another launch asks for
// the window. Only the holder of the lock may listen. stop closes the socket.
func Listen(name string, onShow func()) (stop func(), err error) {
	_, socketPath, err := paths(name)
	if err != nil {
		return nil, err
	}
	// Left behind by a copy that did not exit cleanly; the lock says it is gone.
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // closed
			}
			_ = conn.Close() // the connection itself is the request
			onShow()
		}
	}()
	return func() { _ = ln.Close() }, nil
}

// ShowRunning asks the running copy to show its window. It keeps trying for
// up to wait, since that copy may still be starting and not listening yet.
func ShowRunning(name string, wait time.Duration) bool {
	_, socketPath, err := paths(name)
	if err != nil {
		return false
	}
	deadline := time.Now().Add(wait)
	for {
		conn, err := net.DialTimeout("unix", socketPath, time.Second)
		if err == nil {
			_ = conn.Close()
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

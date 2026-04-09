//go:build windows

// Package singleton implements a process-wide named-mutex lock so that
// only one instance of CopyNote runs per user session at a time.
package singleton

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// Acquire tries to claim a named mutex with the given name (e.g.
// `Local\dev.copynote.app.singleton`).
//
//   release: closes the mutex handle. Always returned non-nil; safe to
//            defer regardless of whether already is true or false.
//   already: true if another live process already holds the same name.
//            CreateMutex still returns a valid handle in that case, so
//            we hold a reference until release() is called.
//   err:     non-nil only on a syscall failure that left no handle.
func Acquire(name string) (release func(), already bool, err error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return func() {}, false, fmt.Errorf("utf16 name: %w", err)
	}
	handle, createErr := windows.CreateMutex(nil, false, ptr)
	if handle == 0 {
		return func() {}, false, fmt.Errorf("CreateMutex: %w", createErr)
	}
	release = func() { _ = windows.CloseHandle(handle) }

	// CreateMutex returns the handle even if another process holds the
	// mutex; it surfaces that fact via GetLastError == ERROR_ALREADY_EXISTS.
	if errors.Is(createErr, windows.ERROR_ALREADY_EXISTS) {
		return release, true, nil
	}
	return release, false, nil
}

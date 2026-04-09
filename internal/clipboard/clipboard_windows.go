//go:build windows

// Package clipboard writes UTF-16 text to the Windows system clipboard
// using user32 + kernel32 syscalls. No cgo, no external dependencies
// beyond golang.org/x/sys/windows.
package clipboard

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	moduser32   = windows.NewLazySystemDLL("user32.dll")
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procOpenClipboard    = moduser32.NewProc("OpenClipboard")
	procCloseClipboard   = moduser32.NewProc("CloseClipboard")
	procEmptyClipboard   = moduser32.NewProc("EmptyClipboard")
	procSetClipboardData = moduser32.NewProc("SetClipboardData")
	procGetClipboardData = moduser32.NewProc("GetClipboardData")

	procGlobalAlloc    = modkernel32.NewProc("GlobalAlloc")
	procGlobalLock     = modkernel32.NewProc("GlobalLock")
	procGlobalUnlock   = modkernel32.NewProc("GlobalUnlock")
	procGlobalFree     = modkernel32.NewProc("GlobalFree")
	procGlobalSize     = modkernel32.NewProc("GlobalSize")
	procRtlMoveMemory  = modkernel32.NewProc("RtlMoveMemory")
)

// WriteText replaces the system clipboard with s, encoded as UTF-16 LE
// (CF_UNICODETEXT). The empty string is allowed and clears the clipboard.
//
// Memory ownership: on success, Windows takes ownership of the global
// memory handle and will free it when the next clipboard owner replaces
// it. We MUST NOT call GlobalFree in that path.
func WriteText(s string) error {
	// Clipboard APIs prefer thread affinity. LockOSThread costs nothing
	// and protects us from rare hand-offs across goroutines.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := openClipboardWithRetry(); err != nil {
		return err
	}
	defer closeClipboard()

	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("EmptyClipboard: %w", err)
	}

	utf16, err := windows.UTF16FromString(s)
	if err != nil {
		// UTF16FromString only fails on embedded NUL — refuse it.
		return fmt.Errorf("encode utf16: %w", err)
	}
	byteLen := uintptr(len(utf16) * 2)

	hMem, _, allocErr := procGlobalAlloc.Call(uintptr(gmemMoveable), byteLen)
	if hMem == 0 {
		return fmt.Errorf("GlobalAlloc: %w", allocErr)
	}

	ptr, _, lockErr := procGlobalLock.Call(hMem)
	if ptr == 0 {
		_, _, _ = procGlobalFree.Call(hMem)
		return fmt.Errorf("GlobalLock: %w", lockErr)
	}

	// Copy via RtlMoveMemory(dst, src, n). Going through a syscall lets
	// us avoid converting the locked-memory uintptr back into an
	// unsafe.Pointer (vet flags such conversions even when they're
	// genuinely safe under Win32 GlobalLock semantics).
	_, _, _ = procRtlMoveMemory.Call(
		ptr,
		uintptr(unsafe.Pointer(&utf16[0])),
		byteLen,
	)

	// GlobalUnlock returns 0 when the lock count drops to 0 — that is
	// the success case for our single-locked moveable handle. Any real
	// error here would surface as a SetClipboardData failure below.
	_, _, _ = procGlobalUnlock.Call(hMem)

	r, _, setErr := procSetClipboardData.Call(uintptr(cfUnicodeText), hMem)
	if r == 0 {
		_, _, _ = procGlobalFree.Call(hMem)
		return fmt.Errorf("SetClipboardData: %w", setErr)
	}
	// Ownership transferred to Windows: do NOT call GlobalFree(hMem).
	return nil
}

// openClipboardWithRetry tries up to 5 times with 10ms backoff because
// neighbouring apps (Slack, Office, etc.) briefly hold the clipboard.
func openClipboardWithRetry() error {
	const attempts = 5
	const wait = 10 * time.Millisecond
	var lastErr error
	for i := 0; i < attempts; i++ {
		r, _, err := procOpenClipboard.Call(0)
		if r != 0 {
			return nil
		}
		lastErr = err
		time.Sleep(wait)
	}
	return fmt.Errorf("OpenClipboard busy after %d attempts: %w", attempts, lastErr)
}

func closeClipboard() {
	_, _, _ = procCloseClipboard.Call()
}


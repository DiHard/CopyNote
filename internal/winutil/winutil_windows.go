//go:build windows

// Package winutil exposes the small set of user32/kernel32 syscalls
// shared between the tray and main packages. Each helper wraps one
// LazyProc.Call and converts return values into idiomatic Go types.
package winutil

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ShowWindow nCmdShow constants.
const (
	SW_HIDE    = 0
	SW_SHOW    = 5
	SW_SHOWNA  = 8
	SW_RESTORE = 9
)

// SetWindowLongPtr index for the WindowProc.
const GWLP_WNDPROC = -4

// Window messages we react to or forward.
const (
	WM_DESTROY     = 0x0002
	WM_CLOSE       = 0x0010
	WM_QUIT        = 0x0012
	WM_ACTIVATEAPP = 0x001C
	WM_COMMAND     = 0x0111
	WM_USER        = 0x0400
	WM_LBUTTONUP   = 0x0202
	WM_RBUTTONUP   = 0x0205
	WM_APP         = 0x8000
)

// HWND_BROADCAST sends a PostMessage to every top-level window.
const HWND_BROADCAST = 0xFFFF

var (
	moduser32 = windows.NewLazySystemDLL("user32.dll")

	procShowWindow             = moduser32.NewProc("ShowWindow")
	procSetForegroundWindow    = moduser32.NewProc("SetForegroundWindow")
	procIsIconic               = moduser32.NewProc("IsIconic")
	procIsWindowVisible        = moduser32.NewProc("IsWindowVisible")
	procPostMessageW           = moduser32.NewProc("PostMessageW")
	procRegisterWindowMessageW = moduser32.NewProc("RegisterWindowMessageW")
	procSetWindowLongPtrW      = moduser32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW        = moduser32.NewProc("CallWindowProcW")
)

// ShowWindow → BOOL ShowWindow(HWND, int).
func ShowWindow(hwnd uintptr, cmd int) bool {
	r, _, _ := procShowWindow.Call(hwnd, uintptr(cmd))
	return r != 0
}

// SetForegroundWindow → BOOL SetForegroundWindow(HWND).
func SetForegroundWindow(hwnd uintptr) bool {
	r, _, _ := procSetForegroundWindow.Call(hwnd)
	return r != 0
}

// IsIconic returns true if the window is currently minimized.
func IsIconic(hwnd uintptr) bool {
	r, _, _ := procIsIconic.Call(hwnd)
	return r != 0
}

// IsWindowVisible reports whether the window has the WS_VISIBLE style.
func IsWindowVisible(hwnd uintptr) bool {
	r, _, _ := procIsWindowVisible.Call(hwnd)
	return r != 0
}

// PostMessage → BOOL PostMessageW(HWND, UINT, WPARAM, LPARAM).
// Pass HWND_BROADCAST as hwnd to send to every top-level window.
func PostMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) bool {
	r, _, _ := procPostMessageW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r != 0
}

// RegisterWindowMessage returns a system-wide message ID for the given
// name. The same name yields the same ID across processes — used as a
// lightweight IPC channel for the single-instance show command.
func RegisterWindowMessage(name string) (uint32, error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, fmt.Errorf("utf16: %w", err)
	}
	r, _, callErr := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(ptr)))
	if r == 0 {
		return 0, fmt.Errorf("RegisterWindowMessageW: %w", callErr)
	}
	return uint32(r), nil
}

// SetWindowLongPtr installs a new value at the given index of the
// window's reserved memory and returns the previous value. Used to
// subclass a window by replacing its WndProc.
func SetWindowLongPtr(hwnd uintptr, index int32, newValue uintptr) uintptr {
	r, _, _ := procSetWindowLongPtrW.Call(hwnd, uintptr(int(index)), newValue)
	return r
}

// CallWindowProc forwards a message to a previously installed WndProc
// captured during subclassing.
func CallWindowProc(prevProc, hwnd, msg, wParam, lParam uintptr) uintptr {
	r, _, _ := procCallWindowProcW.Call(prevProc, hwnd, msg, wParam, lParam)
	return r
}

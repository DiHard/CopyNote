//go:build windows

package tray

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"copynote/internal/winutil"
)

// Names shared by every CopyNote process: the class of the tray window,
// which a second launch looks up, and the registered message that asks
// the running instance to show its main window.
const (
	trayClassName   = "CopyNoteTrayWnd"
	showMessageName = "dev.copynote.app.SHOW"
)

var (
	procFindWindowExW            = moduser32.NewProc("FindWindowExW")
	procGetWindowThreadProcessId = moduser32.NewProc("GetWindowThreadProcessId")
	procAllowSetForegroundWindow = moduser32.NewProc("AllowSetForegroundWindow")
)

// ShowRunningInstance asks an already running CopyNote to show its main
// window and reports whether the request reached its tray window.
//
// The tray window is message-only (HWND_MESSAGE), and message-only windows
// never receive HWND_BROADCAST, so the request is posted to it directly.
// wait covers an instance that already holds the single-instance mutex but
// is still in WebView2 start-up and has not created its tray window yet.
// If no tray window turns up, the request is broadcast as a last resort.
func ShowRunningInstance(wait time.Duration) bool {
	if postToRunningTray(trayClassName, showMessageName, wait) {
		return true
	}
	if id, err := winutil.RegisterWindowMessage(showMessageName); err == nil {
		winutil.PostMessage(winutil.HWND_BROADCAST, id, 0, 0)
	}
	return false
}

// postToRunningTray posts the registered message messageName to the first
// message-only window of class className, polling for up to wait until
// such a window exists.
func postToRunningTray(className, messageName string, wait time.Duration) bool {
	msgID, err := winutil.RegisterWindowMessage(messageName)
	if err != nil {
		return false
	}
	cls, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return false
	}
	deadline := time.Now().Add(wait)
	for {
		hwnd, _, _ := procFindWindowExW.Call(hwndMessage, 0, uintptr(unsafe.Pointer(cls)), 0)
		if hwnd != 0 {
			// This process was just started by the user, so it may pass its
			// right to take the foreground on to the running instance —
			// otherwise that instance's SetForegroundWindow is refused and
			// the window slides out without focus.
			var pid uint32
			_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if pid != 0 {
				_, _, _ = procAllowSetForegroundWindow.Call(uintptr(pid))
			}
			r, _, _ := procPostMessageW.Call(hwnd, uintptr(msgID), 0, 0)
			return r != 0
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

//go:build windows

// Package winutil exposes the small set of user32/kernel32 syscalls
// shared between the tray and main packages. Each helper wraps one
// LazyProc.Call and converts return values into idiomatic Go types.
package winutil

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
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
	WM_DESTROY       = 0x0002
	WM_CLOSE         = 0x0010
	WM_QUIT          = 0x0012
	WM_SETTINGCHANGE = 0x001A
	WM_ACTIVATEAPP   = 0x001C
	WM_COMMAND       = 0x0111
	WM_USER          = 0x0400
	WM_LBUTTONUP     = 0x0202
	WM_RBUTTONUP     = 0x0205
	WM_APP           = 0x8000
)

// HWND_BROADCAST sends a PostMessage to every top-level window.
const HWND_BROADCAST = 0xFFFF

var (
	moduser32 = windows.NewLazySystemDLL("user32.dll")
	moddwmapi = windows.NewLazySystemDLL("dwmapi.dll")

	procShowWindow             = moduser32.NewProc("ShowWindow")
	procSetForegroundWindow    = moduser32.NewProc("SetForegroundWindow")
	procIsIconic               = moduser32.NewProc("IsIconic")
	procIsWindowVisible        = moduser32.NewProc("IsWindowVisible")
	procPostMessageW           = moduser32.NewProc("PostMessageW")
	procRegisterWindowMessageW = moduser32.NewProc("RegisterWindowMessageW")
	procSetWindowLongPtrW      = moduser32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW        = moduser32.NewProc("CallWindowProcW")
	procSystemParametersInfoW  = moduser32.NewProc("SystemParametersInfoW")
	procGetWindowRect          = moduser32.NewProc("GetWindowRect")
	procSetWindowPos           = moduser32.NewProc("SetWindowPos")
	procDwmGetWindowAttribute  = moddwmapi.NewProc("DwmGetWindowAttribute")
)

// DWM window attributes.
const (
	DWMWA_EXTENDED_FRAME_BOUNDS = 9
)

// Rect mirrors Windows RECT (LONGs).
type Rect struct {
	Left, Top, Right, Bottom int32
}

// SystemParametersInfo uiAction codes we use.
const (
	SPI_GETWORKAREA = 0x0030
)

// SetWindowPos flags we use.
const (
	SWP_NOSIZE     = 0x0001
	SWP_NOZORDER   = 0x0004
	SWP_NOACTIVATE = 0x0010
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

// GetWorkArea returns the usable area of the primary monitor — the
// full screen minus the taskbar and any docked toolbars. Coordinates
// are in virtual-screen pixels.
func GetWorkArea() (Rect, bool) {
	var rc Rect
	r, _, _ := procSystemParametersInfoW.Call(
		uintptr(SPI_GETWORKAREA),
		0,
		uintptr(unsafe.Pointer(&rc)),
		0,
	)
	return rc, r != 0
}

// GetWindowRect returns the window's outer bounding rectangle in
// screen coordinates (including frame and title bar).
func GetWindowRect(hwnd uintptr) (Rect, bool) {
	var rc Rect
	r, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	return rc, r != 0
}

// SetWindowPos moves and/or resizes a window. Flags control which of
// the position/size/z-order/activation parameters are actually
// applied (see SWP_* constants).
func SetWindowPos(hwnd, hwndInsertAfter uintptr, x, y, cx, cy int32, flags uint32) bool {
	r, _, _ := procSetWindowPos.Call(
		hwnd,
		hwndInsertAfter,
		uintptr(x),
		uintptr(y),
		uintptr(cx),
		uintptr(cy),
		uintptr(flags),
	)
	return r != 0
}

// StringFromLPCWSTR reads a NUL-terminated UTF-16 string from the
// given pointer. Returns the empty string if the pointer is zero.
// Used to decode the lParam of WM_SETTINGCHANGE into its hint name
// (e.g., "ImmersiveColorSet" when the system theme toggles).
func StringFromLPCWSTR(p uintptr) string {
	if p == 0 {
		return ""
	}
	// Walk the memory until we hit the terminating NUL, then copy
	// out a slice of uint16 and decode with the stdlib helper.
	var length int
	for {
		c := *(*uint16)(unsafe.Pointer(p + uintptr(length*2)))
		if c == 0 {
			break
		}
		length++
		if length > 512 { // bounded sanity limit
			break
		}
	}
	if length == 0 {
		return ""
	}
	buf := make([]uint16, length)
	for i := 0; i < length; i++ {
		buf[i] = *(*uint16)(unsafe.Pointer(p + uintptr(i*2)))
	}
	return windows.UTF16ToString(buf)
}

// IsSystemLightTheme reports whether Windows is currently using a
// light taskbar/tray theme. Reads SystemUsesLightTheme under
// HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize.
// Defaults to true (light) if the key is missing — that matches
// pre-Win10 behavior and avoids a dark-on-dark invisible icon.
func IsSystemLightTheme() bool {
	k, err := registry.OpenKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		registry.QUERY_VALUE,
	)
	if err != nil {
		return true
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	if err != nil {
		return true
	}
	return v == 1
}

// GetExtendedFrameBounds returns the window's visible bounding
// rectangle as rendered by DWM, which excludes the invisible
// resize border that GetWindowRect reports on Windows 10/11. The
// border is ~7–9 px per DPI and its compensation matters when
// visually aligning a window with screen edges.
func GetExtendedFrameBounds(hwnd uintptr) (Rect, bool) {
	var rc Rect
	r, _, _ := procDwmGetWindowAttribute.Call(
		hwnd,
		uintptr(DWMWA_EXTENDED_FRAME_BOUNDS),
		uintptr(unsafe.Pointer(&rc)),
		unsafe.Sizeof(rc),
	)
	// DwmGetWindowAttribute returns S_OK (0) on success.
	return rc, r == 0
}

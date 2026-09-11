//go:build windows

package winutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// The process is per-monitor DPI aware (v2): window coordinates are
// physical pixels, and every size designed at 96 DPI — the CSS-pixel
// scale of the frontend — has to be scaled for the DPI of the monitor
// it is shown on.

// BaseDPI is the DPI at which one logical (CSS) pixel is one physical pixel.
const BaseDPI = 96

// WM_DPICHANGED is sent when a window moves to a monitor with another
// DPI or the DPI of its monitor changes. The low word of wParam holds
// the new DPI.
const WM_DPICHANGED = 0x02E0

// MonitorFromRect fallback flags.
const (
	MONITOR_DEFAULTTONULL    = 0
	MONITOR_DEFAULTTOPRIMARY = 1
	MONITOR_DEFAULTTONEAREST = 2
)

var (
	modshcore = windows.NewLazySystemDLL("shcore.dll")

	procSetProcessDpiAwarenessContext = moduser32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForWindow               = moduser32.NewProc("GetDpiForWindow")
	procMonitorFromRect               = moduser32.NewProc("MonitorFromRect")
	procGetMonitorInfoW               = moduser32.NewProc("GetMonitorInfoW")
	procGetDpiForMonitor              = modshcore.NewProc("GetDpiForMonitor")
)

// EnablePerMonitorDPIAwareness switches the process to per-monitor DPI
// awareness v2, so Windows no longer bitmap-stretches (and blurs) its
// windows on scaled displays. It must run before the first window is
// created; later calls fail with ERROR_ACCESS_DENIED.
func EnablePerMonitorDPIAwareness() error {
	if err := procSetProcessDpiAwarenessContext.Find(); err != nil {
		return err // Windows 10 before 1703
	}
	const perMonitorAwareV2 = ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 == (HANDLE)-4
	if r, _, err := procSetProcessDpiAwarenessContext.Call(perMonitorAwareV2); r == 0 {
		return err
	}
	return nil
}

// ScaleForDPI converts a length designed at 96 DPI into physical pixels
// at dpi, rounding to the nearest pixel like MulDiv.
func ScaleForDPI(v int32, dpi uint32) int32 {
	if dpi == 0 {
		dpi = BaseDPI
	}
	n := int64(v) * int64(dpi)
	if n < 0 {
		return int32((n - BaseDPI/2) / BaseDPI)
	}
	return int32((n + BaseDPI/2) / BaseDPI)
}

// DpiForWindow returns the DPI of the monitor the window is on, or
// BaseDPI if it cannot be determined.
func DpiForWindow(hwnd uintptr) uint32 {
	if r, _, _ := procGetDpiForWindow.Call(hwnd); r != 0 {
		return uint32(r)
	}
	return BaseDPI
}

// DpiForMonitor returns the effective DPI of a monitor, or BaseDPI if it
// cannot be determined.
func DpiForMonitor(hmon uintptr) uint32 {
	var dpiX, dpiY uint32
	r, _, _ := procGetDpiForMonitor.Call(hmon, 0, // MDT_EFFECTIVE_DPI
		uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	if r != 0 || dpiX == 0 {
		return BaseDPI
	}
	return dpiX
}

// MonitorFromRect returns the monitor that overlaps rc the most; flags
// decide the result when rc does not touch any monitor.
func MonitorFromRect(rc Rect, flags uint32) uintptr {
	r, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&rc)), uintptr(flags))
	return r
}

// PrimaryMonitor returns the primary monitor, whose tray the window is
// anchored to. It always contains the virtual-screen origin.
func PrimaryMonitor() uintptr {
	return MonitorFromRect(Rect{Right: 1, Bottom: 1}, MONITOR_DEFAULTTOPRIMARY)
}

// MonitorBounds returns the full rectangle of a monitor in virtual-screen
// coordinates.
func MonitorBounds(hmon uintptr) (Rect, bool) {
	var mi struct {
		cbSize    uint32
		rcMonitor Rect
		rcWork    Rect
		dwFlags   uint32
	}
	mi.cbSize = uint32(unsafe.Sizeof(mi))
	r, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
	return mi.rcMonitor, r != 0
}

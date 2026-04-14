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

// SetWindowLongPtr / GetWindowLongPtr index constants.
const (
	GWL_STYLE    = -16
	GWL_EXSTYLE  = -20
	GWLP_WNDPROC = -4
)

// Window style bits.
const (
	WS_CAPTION       = 0x00C00000 // WS_BORDER | WS_DLGFRAME (title bar + border)
	WS_EX_LAYERED    = 0x00080000
	WS_EX_TOOLWINDOW = 0x00000080 // hides from taskbar + Alt+Tab
	LWA_ALPHA        = 0x00000002
)

// Window messages we react to or forward.
const (
	WM_DESTROY       = 0x0002
	WM_CLOSE         = 0x0010
	WM_QUIT          = 0x0012
	WM_NCCALCSIZE    = 0x0083
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
	moduser32   = windows.NewLazySystemDLL("user32.dll")
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modcomdlg32 = windows.NewLazySystemDLL("comdlg32.dll")

	procGetUserDefaultUILanguage = modkernel32.NewProc("GetUserDefaultUILanguage")
	procGetSaveFileNameW = modcomdlg32.NewProc("GetSaveFileNameW")
	procGetOpenFileNameW = modcomdlg32.NewProc("GetOpenFileNameW")
	moddwmapi = windows.NewLazySystemDLL("dwmapi.dll")

	procShowWindow             = moduser32.NewProc("ShowWindow")
	procSetForegroundWindow    = moduser32.NewProc("SetForegroundWindow")
	procIsIconic               = moduser32.NewProc("IsIconic")
	procIsWindowVisible        = moduser32.NewProc("IsWindowVisible")
	procPostMessageW           = moduser32.NewProc("PostMessageW")
	procSendMessageW           = moduser32.NewProc("SendMessageW")
	procLoadImageW             = moduser32.NewProc("LoadImageW")
	procGetModuleHandleW       = modkernel32.NewProc("GetModuleHandleW")
	procSetClassLongPtrW       = moduser32.NewProc("SetClassLongPtrW")
	procCreateSolidBrush       = windows.NewLazySystemDLL("gdi32.dll").NewProc("CreateSolidBrush")
	procRegisterWindowMessageW = moduser32.NewProc("RegisterWindowMessageW")
	procGetWindowLongPtrW           = moduser32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW           = moduser32.NewProc("SetWindowLongPtrW")
	procSetLayeredWindowAttributes  = moduser32.NewProc("SetLayeredWindowAttributes")
	procCallWindowProcW        = moduser32.NewProc("CallWindowProcW")
	procSystemParametersInfoW  = moduser32.NewProc("SystemParametersInfoW")
	procGetWindowRect          = moduser32.NewProc("GetWindowRect")
	procSetWindowPos           = moduser32.NewProc("SetWindowPos")
	procDwmGetWindowAttribute      = moddwmapi.NewProc("DwmGetWindowAttribute")
	procDwmSetWindowAttribute      = moddwmapi.NewProc("DwmSetWindowAttribute")
	procDwmExtendFrameIntoClientArea = moddwmapi.NewProc("DwmExtendFrameIntoClientArea")
)

// DWM window attributes.
const (
	DWMWA_EXTENDED_FRAME_BOUNDS    = 9
	DWMWA_WINDOW_CORNER_PREFERENCE = 33
	DWMWA_SYSTEMBACKDROP_TYPE      = 38
)

// DWM system backdrop types (Win11 22H2+).
const (
	DWMSBT_MAINWINDOW      = 2 // Mica
	DWMSBT_TRANSIENTWINDOW = 3 // Mica Alt (slightly more opaque)
	DWMSBT_TABBEDWINDOW    = 4 // Tabbed Mica
)

// MARGINS for DwmExtendFrameIntoClientArea.
type Margins struct {
	Left, Right, Top, Bottom int32
}

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
	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_NOACTIVATE   = 0x0010
	SWP_FRAMECHANGED = 0x0020
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
// GetWindowLongPtr retrieves window style or extra data.
func GetWindowLongPtr(hwnd uintptr, index int32) uintptr {
	r, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(int(index)))
	return r
}

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

// SetWindowBackgroundColor sets the window class background brush
// to a solid color (COLORREF = 0x00BBGGRR). This ensures any area
// exposed during resize is painted with this color instead of black.
func SetWindowBackgroundColor(hwnd uintptr, r, g, b byte) {
	colorRef := uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16
	brush, _, _ := procCreateSolidBrush.Call(colorRef)
	if brush != 0 {
		// GCLP_HBRBACKGROUND = -10; use twos-complement for uintptr.
		const gclpHbrBackground = ^uintptr(10 - 1) // == -10 as uintptr
		procSetClassLongPtrW.Call(hwnd, gclpHbrBackground, brush)
	}
}

// SetWindowIcon loads an icon from the exe's embedded resources
// and sets it on the window (both small and big icon).
func SetWindowIcon(hwnd uintptr, resourceID int) {
	const (
		imageIcon      = 1
		lrDefaultColor = 0x00000000
		lrSharedIcon   = 0x00008000
		wmSetIcon      = 0x0080
		iconSmall      = 0
		iconBig        = 1
	)
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	hIcon, _, _ := procLoadImageW.Call(
		hInstance,
		uintptr(resourceID),
		uintptr(imageIcon),
		0, 0,
		uintptr(lrDefaultColor|lrSharedIcon),
	)
	if hIcon != 0 {
		procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, hIcon)
		procSendMessageW.Call(hwnd, wmSetIcon, iconBig, hIcon)
	}
}

// SystemLocale returns a two-letter language code based on the
// Windows UI language (e.g. "en", "ru"). Falls back to "en".
func SystemLocale() string {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	primary := uint16(r) & 0x3FF // lower 10 bits = primary language
	switch primary {
	case 0x19: // LANG_RUSSIAN
		return "ru"
	default:
		return "en"
	}
}

// ── File Dialogs ────────────────────────────────────────────────

// OPENFILENAMEW is the Win32 structure for GetOpenFileNameW / GetSaveFileNameW.
type openFileNameW struct {
	structSize      uint32
	hwndOwner       uintptr
	hInstance       uintptr
	filter          *uint16
	customFilter    *uint16
	maxCustomFilter uint32
	filterIndex     uint32
	file            *uint16
	maxFile         uint32
	fileTitle       *uint16
	maxFileTitle    uint32
	initialDir      *uint16
	title           *uint16
	flags           uint32
	fileOffset      uint16
	fileExtension   uint16
	defExt          *uint16
	custData        uintptr
	fnHook          uintptr
	templateName    *uint16
	// v5 extended fields — not needed, zero-initialized by default.
}

const (
	ofnOverwritePrompt = 0x00000002
	ofnFileMustExist   = 0x00001000
	ofnPathMustExist   = 0x00000800
	ofnNoChangeDir     = 0x00000008
)

// buildFilter converts a Go-friendly filter string to a double-NUL
// terminated UTF-16 slice. Pairs are separated by '|', e.g.:
//
//	"CopyNote Backup (*.json)|*.json|All Files|*.*"
func buildFilter(filter string) *uint16 {
	// Replace '|' with NUL, then add a trailing double-NUL.
	var buf []uint16
	for _, r := range filter {
		if r == '|' {
			buf = append(buf, 0)
		} else {
			buf = append(buf, uint16(r))
		}
	}
	buf = append(buf, 0, 0) // double-NUL terminator
	return &buf[0]
}

// SaveFileDialog opens a native Save dialog. Returns the chosen path
// and true, or ("", false) if the user cancelled.
func SaveFileDialog(hwnd uintptr, filter, defaultName string) (string, bool) {
	fileBuf := make([]uint16, 260)
	if defaultName != "" {
		u16, _ := windows.UTF16FromString(defaultName)
		copy(fileBuf, u16)
	}

	ofn := openFileNameW{
		hwndOwner: hwnd,
		filter:    buildFilter(filter),
		file:      &fileBuf[0],
		maxFile:   uint32(len(fileBuf)),
		flags:     ofnOverwritePrompt | ofnPathMustExist | ofnNoChangeDir,
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))

	r, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return windows.UTF16ToString(fileBuf), true
}

// OpenFileDialog opens a native Open dialog. Returns the chosen path
// and true, or ("", false) if the user cancelled.
func OpenFileDialog(hwnd uintptr, filter string) (string, bool) {
	fileBuf := make([]uint16, 260)

	ofn := openFileNameW{
		hwndOwner: hwnd,
		filter:    buildFilter(filter),
		file:      &fileBuf[0],
		maxFile:   uint32(len(fileBuf)),
		flags:     ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir,
	}
	ofn.structSize = uint32(unsafe.Sizeof(ofn))

	r, _, _ := procGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return "", false
	}
	return windows.UTF16ToString(fileBuf), true
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

// SetWindowAlpha makes the window uniformly semi-transparent via
// the WS_EX_LAYERED extended style + SetLayeredWindowAttributes.
// alpha ranges from 0 (invisible) to 255 (fully opaque).
func SetWindowAlpha(hwnd uintptr, alpha byte) {
	exStyle := GetWindowLongPtr(hwnd, GWL_EXSTYLE)
	SetWindowLongPtr(hwnd, GWL_EXSTYLE, exStyle|WS_EX_LAYERED)
	_, _, _ = procSetLayeredWindowAttributes.Call(
		hwnd,
		0,
		uintptr(alpha),
		uintptr(LWA_ALPHA),
	)
}

// DwmSetSystemBackdrop sets the DWM system backdrop type on a
// window (Win11 22H2+). Use DWMSBT_MAINWINDOW for Mica,
// DWMSBT_TRANSIENTWINDOW for Mica Alt, etc.
// Silently fails on older Windows versions.
func DwmSetSystemBackdrop(hwnd uintptr, backdropType uint32) {
	var bt uint32 = backdropType
	_, _, _ = procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(DWMWA_SYSTEMBACKDROP_TYPE),
		uintptr(unsafe.Pointer(&bt)),
		unsafe.Sizeof(bt),
	)
}

// DwmExtendFrameIntoClientArea extends the DWM frame into the
// client area. Pass Margins{-1,-1,-1,-1} to extend into the
// entire window — required for Mica backdrop to show through.
func DwmExtendFrameIntoClientArea(hwnd uintptr, margins Margins) {
	_, _, _ = procDwmExtendFrameIntoClientArea.Call(
		hwnd,
		uintptr(unsafe.Pointer(&margins)),
	)
}

// DwmSetWindowCornerPreference sets rounded-corner preference
// on Win11. Value 2 = DWMWCP_ROUND.
func DwmSetWindowCornerPreference(hwnd uintptr, pref uint32) {
	var p uint32 = pref
	_, _, _ = procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(DWMWA_WINDOW_CORNER_PREFERENCE),
		uintptr(unsafe.Pointer(&p)),
		unsafe.Sizeof(p),
	)
}

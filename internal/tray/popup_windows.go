//go:build windows

package tray

// Custom popup-menu implementation. We replace the system TrackPopupMenu
// with a borderless top-level WS_POPUP window that we paint ourselves
// via GDI. This lets us:
//   - apply Windows 11 DWM rounded corners (DwmSetWindowAttribute)
//   - control item height and padding (Telegram-style spacious layout)
//   - pick our own font (Segoe UI 9pt) and colors
//
// The popup runs on the same OS thread as the tray's GetMessage loop,
// so its messages are dispatched naturally without an extra modal loop.

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"copynote/internal/winutil"
)

// Visual constants in 96-DPI pixels, scaled by popupMetricsFor.
const (
	popupItemHeight = 36
	popupPadV       = 6
	popupPadH       = 16
	popupMinWidth   = 180
	popupHoverInset = 4
	popupFontHeight = 14
)

// popupMetrics holds the popup sizes in physical pixels for the DPI of
// the monitor the popup opens on (the process is per-monitor DPI aware).
type popupMetrics struct {
	dpi                              uint32
	itemHeight, padV, padH, minWidth int32
	hoverInset, fontHeight           int32
}

func popupMetricsFor(dpi uint32) popupMetrics {
	s := func(v int32) int32 { return winutil.ScaleForDPI(v, dpi) }
	return popupMetrics{
		dpi:        dpi,
		itemHeight: s(popupItemHeight),
		padV:       s(popupPadV),
		padH:       s(popupPadH),
		minWidth:   s(popupMinWidth),
		hoverInset: s(popupHoverInset),
		fontHeight: s(popupFontHeight),
	}
}

// COLORREF values are 0x00BBGGRR.
// Light theme (default).
var (
	colorBg    uint32 = 0x00FBFBFB
	colorHover uint32 = 0x00EAEAEA
	colorText  uint32 = 0x00202020
)

// Dark theme colors.
const (
	darkColorBg    uint32 = 0x002D2D2D
	darkColorHover uint32 = 0x00383838
	darkColorText  uint32 = 0x00FFFFFF
)

// Light theme colors.
const (
	lightColorBg    uint32 = 0x00FBFBFB
	lightColorHover uint32 = 0x00EAEAEA
	lightColorText  uint32 = 0x00202020
)

// applyPopupTheme sets the popup color vars based on the current system theme.
func applyPopupTheme() {
	if winutil.IsSystemLightTheme() {
		colorBg = lightColorBg
		colorHover = lightColorHover
		colorText = lightColorText
	} else {
		colorBg = darkColorBg
		colorHover = darkColorHover
		colorText = darkColorText
	}
}

// Win32 / GDI / DWM constants used by the popup window only.
const (
	wsPopup     = 0x80000000
	wsBorder    = 0x00800000
	wsExToolWnd = 0x00000080
	wsExTopmost = 0x00000008

	wmPaint      = 0x000F
	wmMouseMove  = 0x0200
	wmMouseLeave = 0x02A3
	wmKillFocus  = 0x0008
	wmActivate   = 0x0006
	waInactive   = 0

	tmeLeave = 0x00000002

	dtLeft       = 0x00000000
	dtVCenter    = 0x00000004
	dtSingleLine = 0x00000020
	dtNoPrefix   = 0x00000800

	bkTransparent = 1

	dwmaWindowCornerPreference = 33
	dwmwcpRound                = 2

	fwNormal         = 400
	defaultCharset   = 1
	clearTypeQuality = 5
)

var (
	modgdi32  = windows.NewLazySystemDLL("gdi32.dll")
	moddwmapi = windows.NewLazySystemDLL("dwmapi.dll")

	procBeginPaint       = moduser32.NewProc("BeginPaint")
	procEndPaint         = moduser32.NewProc("EndPaint")
	procFillRect         = moduser32.NewProc("FillRect")
	procDrawTextW        = moduser32.NewProc("DrawTextW")
	procInvalidateRect   = moduser32.NewProc("InvalidateRect")
	procGetClientRect    = moduser32.NewProc("GetClientRect")
	procTrackMouseEvent  = moduser32.NewProc("TrackMouseEvent")
	procGetSystemMetrics = moduser32.NewProc("GetSystemMetrics")
	procGetDC            = moduser32.NewProc("GetDC")
	procReleaseDC        = moduser32.NewProc("ReleaseDC")

	procCreateFontW          = modgdi32.NewProc("CreateFontW")
	procDeleteObject         = modgdi32.NewProc("DeleteObject")
	procSelectObject         = modgdi32.NewProc("SelectObject")
	procSetBkMode            = modgdi32.NewProc("SetBkMode")
	procSetTextColor         = modgdi32.NewProc("SetTextColor")
	procCreateSolidBrush     = modgdi32.NewProc("CreateSolidBrush")
	procGetTextExtentPoint32 = modgdi32.NewProc("GetTextExtentPoint32W")

	procDwmSetWindowAttribute = moddwmapi.NewProc("DwmSetWindowAttribute")
)

type rect struct {
	left, top, right, bottom int32
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type trackMouseEventStruct struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   uintptr
	dwHoverTime uint32
}

type sizeStruct struct {
	cx, cy int32
}

type popupItem struct {
	id    uint32
	label string
}

// Package-level popup state. Only one popup can be visible at a time
// (mouse-driven, single tray icon), so a single set of vars is enough.
var (
	popupClassNamePtr *uint16
	popupWndProcCB    uintptr
	popupRegistered   bool

	popupHwnd     uintptr
	popupItems    []popupItem
	popupHovered  = -1
	popupOnPick   func(uint32)
	popupTracking bool

	popupM          = popupMetricsFor(winutil.BaseDPI)
	popupFont       uintptr
	popupFontDPI    uint32 // DPI popupFont was created for
	popupBrushBg    uintptr
	popupBrushHover uintptr
)

// showCustomPopup creates and displays a borderless popup window at
// (x, y) with the given items. onPick is invoked from the popup's
// WndProc on the same thread when the user clicks an item.
func showCustomPopup(items []popupItem, x, y int32, onPick func(uint32)) {
	mon := winutil.MonitorFromRect(winutil.Rect{Left: x, Top: y, Right: x + 1, Bottom: y + 1},
		winutil.MONITOR_DEFAULTTONEAREST)
	popupM = popupMetricsFor(winutil.DpiForMonitor(mon))

	ensurePopupRegistered()
	ensurePopupResources()

	popupItems = items
	popupHovered = -1
	popupOnPick = onPick

	width, height := measurePopup(items)

	// Keep the popup on the cursor's monitor — open it above the cursor
	// if it would run off the bottom edge.
	if mb, ok := winutil.MonitorBounds(mon); ok {
		if x+width > mb.Right {
			x = mb.Right - width
		}
		if y+height > mb.Bottom {
			y -= height
		}
	}

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, _ := procCreateWindowExW.Call(
		uintptr(wsExToolWnd|wsExTopmost),
		uintptr(unsafe.Pointer(popupClassNamePtr)),
		uintptr(unsafe.Pointer(popupClassNamePtr)),
		uintptr(wsPopup|wsBorder),
		uintptr(x), uintptr(y),
		uintptr(width), uintptr(height),
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return
	}
	popupHwnd = hwnd

	// Win11 DWM rounded corners — silently no-op on Win10.
	pref := uint32(dwmwcpRound)
	_, _, _ = procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(dwmaWindowCornerPreference),
		uintptr(unsafe.Pointer(&pref)),
		unsafe.Sizeof(pref),
	)

	// Show + activate so we get WM_KILLFOCUS to dismiss on click-out.
	winutil.ShowWindow(hwnd, winutil.SW_SHOW)
	winutil.SetForegroundWindow(hwnd)
}

func ensurePopupRegistered() {
	if popupRegistered {
		return
	}
	if popupClassNamePtr == nil {
		ptr, _ := windows.UTF16PtrFromString("CopyNoteTrayPopup")
		popupClassNamePtr = ptr
	}
	if popupWndProcCB == 0 {
		popupWndProcCB = syscall.NewCallback(popupWndProc)
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   popupWndProcCB,
		hInstance:     hInstance,
		lpszClassName: popupClassNamePtr,
	}
	_, _, _ = procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	popupRegistered = true
}

func ensurePopupResources() {
	// Re-detect theme and recreate brushes if colors changed.
	applyPopupTheme()

	// Recreate brushes each time (theme may have changed).
	if popupBrushBg != 0 {
		_, _, _ = procDeleteObject.Call(popupBrushBg)
		popupBrushBg = 0
	}
	if popupBrushHover != 0 {
		_, _, _ = procDeleteObject.Call(popupBrushHover)
		popupBrushHover = 0
	}

	if popupFont != 0 && popupFontDPI != popupM.dpi {
		_, _, _ = procDeleteObject.Call(popupFont)
		popupFont = 0
	}
	if popupFont == 0 {
		name, _ := windows.UTF16PtrFromString("Segoe UI")
		nHeight := -popupM.fontHeight // negative: character height in pixels
		h, _, _ := procCreateFontW.Call(
			uintptr(uint32(nHeight)),
			0, 0, 0,
			uintptr(fwNormal),
			0, 0, 0,
			uintptr(defaultCharset),
			0, 0,
			uintptr(clearTypeQuality),
			0,
			uintptr(unsafe.Pointer(name)),
		)
		popupFont = h
		popupFontDPI = popupM.dpi
	}
	bg, _, _ := procCreateSolidBrush.Call(uintptr(colorBg))
	popupBrushBg = bg
	hv, _, _ := procCreateSolidBrush.Call(uintptr(colorHover))
	popupBrushHover = hv
}

func releasePopupResources() {
	if popupFont != 0 {
		_, _, _ = procDeleteObject.Call(popupFont)
		popupFont = 0
	}
	if popupBrushBg != 0 {
		_, _, _ = procDeleteObject.Call(popupBrushBg)
		popupBrushBg = 0
	}
	if popupBrushHover != 0 {
		_, _, _ = procDeleteObject.Call(popupBrushHover)
		popupBrushHover = 0
	}
}

func measurePopup(items []popupItem) (int32, int32) {
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)

	_, _, _ = procSelectObject.Call(hdc, popupFont)

	maxW := popupM.minWidth
	for _, it := range items {
		u16, _ := windows.UTF16FromString(it.label)
		// length excludes the null terminator
		var sz sizeStruct
		_, _, _ = procGetTextExtentPoint32.Call(
			hdc,
			uintptr(unsafe.Pointer(&u16[0])),
			uintptr(len(u16)-1),
			uintptr(unsafe.Pointer(&sz)),
		)
		w := sz.cx + popupM.padH*2
		if w > maxW {
			maxW = w
		}
	}
	height := popupM.padV*2 + popupM.itemHeight*int32(len(items))
	return maxW, height
}

func popupWndProc(hwnd, msgID, wParam, lParam uintptr) uintptr {
	switch uint32(msgID) {
	case wmPaint:
		paintPopup(hwnd)
		return 0

	case wmMouseMove:
		if !popupTracking {
			tme := trackMouseEventStruct{
				cbSize:    uint32(unsafe.Sizeof(trackMouseEventStruct{})),
				dwFlags:   tmeLeave,
				hwndTrack: hwnd,
			}
			_, _, _ = procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			popupTracking = true
		}
		y := int32(int16((lParam >> 16) & 0xFFFF))
		idx := int((y - popupM.padV) / popupM.itemHeight)
		if idx < 0 || idx >= len(popupItems) {
			idx = -1
		}
		if idx != popupHovered {
			popupHovered = idx
			_, _, _ = procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0

	case wmMouseLeave:
		popupTracking = false
		if popupHovered != -1 {
			popupHovered = -1
			_, _, _ = procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0

	case winutil.WM_LBUTTONUP:
		if popupHovered >= 0 && popupHovered < len(popupItems) {
			id := popupItems[popupHovered].id
			cb := popupOnPick
			closePopup()
			if cb != nil {
				cb(id)
			}
		}
		return 0

	case wmKillFocus:
		if popupHwnd == hwnd {
			closePopup()
		}
		return 0

	case wmActivate:
		if uint32(wParam)&0xFFFF == waInactive && popupHwnd == hwnd {
			closePopup()
		}
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(hwnd, msgID, wParam, lParam)
	return r
}

func paintPopup(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc rect
	_, _, _ = procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))

	// Background
	_, _, _ = procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), popupBrushBg)

	// Text setup
	_, _, _ = procSelectObject.Call(hdc, popupFont)
	_, _, _ = procSetBkMode.Call(hdc, uintptr(bkTransparent))
	_, _, _ = procSetTextColor.Call(hdc, uintptr(colorText))

	for i, it := range popupItems {
		itemRc := rect{
			left:   rc.left,
			top:    rc.top + popupM.padV + int32(i)*popupM.itemHeight,
			right:  rc.right,
			bottom: rc.top + popupM.padV + int32(i+1)*popupM.itemHeight,
		}
		if i == popupHovered {
			hoverRc := itemRc
			hoverRc.left += popupM.hoverInset
			hoverRc.right -= popupM.hoverInset
			_, _, _ = procFillRect.Call(hdc, uintptr(unsafe.Pointer(&hoverRc)), popupBrushHover)
		}
		textRc := itemRc
		textRc.left += popupM.padH
		u16, _ := windows.UTF16FromString(it.label)
		// DrawTextW: -1 means "string is null-terminated".
		strLen := int32(-1)
		_, _, _ = procDrawTextW.Call(
			hdc,
			uintptr(unsafe.Pointer(&u16[0])),
			uintptr(uint32(strLen)),
			uintptr(unsafe.Pointer(&textRc)),
			uintptr(dtLeft|dtVCenter|dtSingleLine|dtNoPrefix),
		)
	}
}

func closePopup() {
	if popupHwnd != 0 {
		_, _, _ = procDestroyWindow.Call(popupHwnd)
		popupHwnd = 0
	}
	popupTracking = false
	popupHovered = -1
}

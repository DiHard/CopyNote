//go:build windows

// Package popupmenu draws CopyNote's popup menus: a borderless WS_POPUP
// window painted with GDI instead of TrackPopupMenu, which buys Windows 11
// rounded corners, roomy rows, a shortcut column and colours that can follow
// the app's theme rather than the system's. The tray icon's menu and the
// entry list's context menu are both drawn here.
//
// A menu's window belongs to the thread that shows it, and so does the Menu
// value: show and close it from one thread, the one whose message loop is
// meant to drive it. The entry menu lives on the UI thread for a reason —
// activation then moves between two windows of the same thread, so the main
// window gets no WM_ACTIVATEAPP and does not auto-hide while its menu is up.
package popupmenu

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"copynote/internal/winutil"
)

// Item is one row of a menu.
type Item struct {
	// ID is what Show's callback reports; unique within one menu.
	ID    uint32
	Label string
	// Shortcut is drawn right-aligned and dimmed; "" for none.
	Shortcut string
	// Disabled items are drawn dimmed and cannot be picked.
	Disabled bool
	// Separator draws a divider line; the other fields are ignored.
	Separator bool
}

// Theme picks the colours.
type Theme uint8

const (
	// ThemeSystem follows the Windows app theme, which suits the tray icon.
	ThemeSystem Theme = iota
	ThemeLight
	ThemeDark
)

// Options control how Show opens a menu.
type Options struct {
	// Owner is the window the menu belongs to: the menu stays above it, and
	// Windows hands activation back to it when the menu closes. 0 for none.
	Owner uintptr
	Theme Theme
	// Keyboard highlights the first item, as a menu opened from the keyboard
	// does; a menu opened with the mouse waits for the pointer.
	Keyboard bool
}

// Sizes in 96-DPI pixels, scaled by metricsFor.
const (
	baseItemHeight      = 36
	basePadV            = 6
	basePadH            = 16
	baseMinWidth        = 180
	baseHoverInset      = 4
	baseFontHeight      = 14
	baseSeparatorHeight = 9
	baseShortcutGap     = 32
)

// metrics holds the sizes in physical pixels for the DPI of the monitor the
// menu opens on (the process is per-monitor DPI aware).
type metrics struct {
	dpi                              uint32
	itemHeight, padV, padH, minWidth int32
	hoverInset, fontHeight           int32
	separatorHeight, separatorLine   int32
	shortcutGap                      int32
}

func metricsFor(dpi uint32) metrics {
	s := func(v int32) int32 { return winutil.ScaleForDPI(v, dpi) }
	return metrics{
		dpi:             dpi,
		itemHeight:      s(baseItemHeight),
		padV:            s(basePadV),
		padH:            s(basePadH),
		minWidth:        s(baseMinWidth),
		hoverInset:      s(baseHoverInset),
		fontHeight:      s(baseFontHeight),
		separatorHeight: s(baseSeparatorHeight),
		separatorLine:   max(1, s(1)),
		shortcutGap:     s(baseShortcutGap),
	}
}

// palette holds COLORREF values (0x00BBGGRR). The shortcut grey is the app's
// on-surface-dim, which clears 4.5:1 on either background.
type palette struct {
	bg, hover, text, shortcut, disabled, separator uint32
}

var (
	lightPalette = palette{bg: 0xFBFBFB, hover: 0xEAEAEA, text: 0x202020, shortcut: 0x616161, disabled: 0xA0A0A0, separator: 0xE5E5E5}
	darkPalette  = palette{bg: 0x2D2D2D, hover: 0x383838, text: 0xFFFFFF, shortcut: 0xB0B0B0, disabled: 0x797979, separator: 0x3D3D3D}
)

func paletteFor(theme Theme) palette {
	switch theme {
	case ThemeLight:
		return lightPalette
	case ThemeDark:
		return darkPalette
	}
	if winutil.IsSystemLightTheme() {
		return lightPalette
	}
	return darkPalette
}

// Menu is a popup menu that can be shown any number of times. The zero value
// is ready to use; the package comment has its thread rules.
type Menu struct {
	hwnd     uintptr
	items    []Item
	tops     []int32 // top of each item, client coordinates
	hovered  int     // highlighted item, -1 for none
	tracking bool    // TrackMouseEvent is armed
	onClose  func(id uint32, picked bool)

	size           metrics
	colors         palette
	font           uintptr
	fontDPI        uint32 // DPI font was created for
	brushBg        uintptr
	brushHover     uintptr
	brushSeparator uintptr
}

// Show opens the menu at (x, y) in screen coordinates, replacing it if it is
// already open. onClose runs exactly once per Show, on this thread, after the
// window is gone: with the picked item's ID, or with picked false when the
// menu was dismissed — Escape, a click elsewhere, or Close.
func (p *Menu) Show(items []Item, x, y int32, opts Options, onClose func(id uint32, picked bool)) {
	p.Close()

	mon := winutil.MonitorFromRect(winutil.Rect{Left: x, Top: y, Right: x + 1, Bottom: y + 1},
		winutil.MONITOR_DEFAULTTONEAREST)
	p.size = metricsFor(winutil.DpiForMonitor(mon))
	p.colors = paletteFor(opts.Theme)
	registerClass()
	p.prepareResources()

	p.items = items
	var width, height int32
	p.tops, width, height = layout(items, p.size, p.measurer())
	if bounds, ok := winutil.MonitorBounds(mon); ok {
		x, y = place(x, y, width, height, bounds)
	}

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, _ := procCreateWindowExW.Call(
		uintptr(wsExToolWnd|wsExTopmost),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		uintptr(wsPopup|wsBorder),
		uintptr(x), uintptr(y),
		uintptr(width), uintptr(height),
		opts.Owner, 0, hInstance, 0,
	)
	if hwnd == 0 {
		if onClose != nil {
			onClose(0, false)
		}
		return
	}
	p.hwnd = hwnd
	p.onClose = onClose
	p.tracking = false
	p.hovered = -1
	if opts.Keyboard {
		p.hovered = nextEnabled(items, -1, 1)
	}
	menusMu.Lock()
	menus[hwnd] = p
	menusMu.Unlock()

	// Win11 DWM rounded corners — silently a no-op on Win10.
	winutil.DwmSetWindowCornerPreference(hwnd, dwmwcpRound)
	winutil.ShowWindow(hwnd, winutil.SW_SHOW)
	// Active, so a click anywhere else deactivates and closes it, and the
	// arrow keys and Enter arrive here.
	winutil.SetForegroundWindow(hwnd)
}

// Close dismisses the menu if it is open; its callback reports no pick.
func (p *Menu) Close() {
	p.finish(0, false)
}

// IsOpen reports whether the menu is on screen.
func (p *Menu) IsOpen() bool {
	return p.hwnd != 0
}

// Release closes the menu and frees its GDI objects.
func (p *Menu) Release() {
	p.Close()
	p.deleteBrushes()
	if p.font != 0 {
		_, _, _ = procDeleteObject.Call(p.font)
		p.font = 0
	}
}

// finish destroys the window and reports the outcome, at most once per Show:
// the window leaves the registry before DestroyWindow, so the deactivation
// messages its destruction sends find no menu to close a second time.
func (p *Menu) finish(id uint32, picked bool) {
	hwnd := p.hwnd
	if hwnd == 0 {
		return
	}
	p.hwnd = 0
	menusMu.Lock()
	delete(menus, hwnd)
	menusMu.Unlock()
	onClose := p.onClose
	p.onClose = nil
	p.tracking = false
	p.hovered = -1
	_, _, _ = procDestroyWindow.Call(hwnd)
	if onClose != nil {
		onClose(id, picked)
	}
}

// layout returns the top of every item, the menu's width and its height.
// measure reports the width of a string in the menu font.
func layout(items []Item, size metrics, measure func(string) int32) (tops []int32, width, height int32) {
	tops = make([]int32, len(items))
	y := size.padV
	var labelW, shortcutW int32
	for i, it := range items {
		tops[i] = y
		if it.Separator {
			y += size.separatorHeight
			continue
		}
		y += size.itemHeight
		labelW = max(labelW, measure(it.Label))
		if it.Shortcut != "" {
			shortcutW = max(shortcutW, measure(it.Shortcut))
		}
	}
	width = labelW + 2*size.padH
	if shortcutW > 0 {
		width += size.shortcutGap + shortcutW
	}
	return tops, max(width, size.minWidth), y + size.padV
}

// place keeps a width × height menu anchored at (x, y) on the monitor, the
// way Windows places its own menus: pushed left at the right edge, opened
// upwards when it would run off the bottom.
func place(x, y, width, height int32, bounds winutil.Rect) (int32, int32) {
	if x+width > bounds.Right {
		x = bounds.Right - width
	}
	if x < bounds.Left {
		x = bounds.Left
	}
	if y+height > bounds.Bottom {
		y -= height
	}
	if y < bounds.Top {
		y = bounds.Top
	}
	return x, y
}

// itemAt returns the pickable item at client coordinate y, or -1.
func itemAt(items []Item, tops []int32, size metrics, y int32) int {
	for i, it := range items {
		if it.Separator || it.Disabled {
			continue
		}
		if y >= tops[i] && y < tops[i]+size.itemHeight {
			return i
		}
	}
	return -1
}

// nextEnabled returns the next pickable item after from in direction dir
// (+1 or -1), wrapping around, or -1 if there is none. A from of -1 starts
// at the top going down and at the bottom going up, as Windows menus do.
func nextEnabled(items []Item, from, dir int) int {
	n := len(items)
	if from < 0 && dir < 0 {
		from = n
	}
	for step := 1; step <= n; step++ {
		i := ((from+dir*step)%n + n) % n
		if !items[i].Separator && !items[i].Disabled {
			return i
		}
	}
	return -1
}

func (p *Menu) hover(i int) {
	if i == p.hovered {
		return
	}
	p.hovered = i
	_, _, _ = procInvalidateRect.Call(p.hwnd, 0, 0)
}

func (p *Menu) key(vk uintptr) {
	switch vk {
	case vkDown:
		p.hover(nextEnabled(p.items, p.hovered, 1))
	case vkUp:
		p.hover(nextEnabled(p.items, p.hovered, -1))
	case vkHome:
		p.hover(nextEnabled(p.items, -1, 1))
	case vkEnd:
		p.hover(nextEnabled(p.items, -1, -1))
	case vkReturn, vkSpace:
		if p.hovered >= 0 {
			p.finish(p.items[p.hovered].ID, true)
		}
	case vkEscape, vkApps:
		p.finish(0, false)
	}
}

func wndProc(hwnd, msgID, wParam, lParam uintptr) uintptr {
	menusMu.Lock()
	p := menus[hwnd]
	menusMu.Unlock()
	if p == nil {
		r, _, _ := procDefWindowProcW.Call(hwnd, msgID, wParam, lParam)
		return r
	}
	y := int32(int16((lParam >> 16) & 0xFFFF))

	switch uint32(msgID) {
	case wmPaint:
		p.paint()
		return 0

	case wmMouseMove:
		if !p.tracking {
			tme := trackMouseEventStruct{
				cbSize:    uint32(unsafe.Sizeof(trackMouseEventStruct{})),
				dwFlags:   tmeLeave,
				hwndTrack: hwnd,
			}
			_, _, _ = procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			p.tracking = true
		}
		p.hover(itemAt(p.items, p.tops, p.size, y))
		return 0

	case wmMouseLeave:
		p.tracking = false
		p.hover(-1)
		return 0

	case wmLButtonUp:
		if i := itemAt(p.items, p.tops, p.size, y); i >= 0 {
			p.finish(p.items[i].ID, true)
		}
		return 0

	case wmGetDlgCode:
		// go-webview2's message loop hands every message on the UI thread to
		// IsDialogMessage, which takes the arrow keys, Enter and Escape as
		// dialog navigation and never dispatches them. Asking for all keys
		// makes it deliver them here instead.
		return dlgcWantAllKeys

	case wmKeyDown:
		p.key(wParam)
		return 0

	case wmSysKeyDown:
		// Alt or F10 closes a menu in Windows.
		p.finish(0, false)
		return 0

	case wmKillFocus:
		p.finish(0, false)
		return 0

	case wmActivate:
		if wParam&0xFFFF == waInactive {
			p.finish(0, false)
			return 0
		}
		// Activation falls through to DefWindowProc, which gives the menu
		// keyboard focus; without focus the arrow keys would not arrive.
	}

	r, _, _ := procDefWindowProcW.Call(hwnd, msgID, wParam, lParam)
	return r
}

func (p *Menu) paint() {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(p.hwnd, uintptr(unsafe.Pointer(&ps)))
	defer procEndPaint.Call(p.hwnd, uintptr(unsafe.Pointer(&ps)))

	var rc rect
	_, _, _ = procGetClientRect.Call(p.hwnd, uintptr(unsafe.Pointer(&rc)))
	_, _, _ = procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), p.brushBg)

	_, _, _ = procSelectObject.Call(hdc, p.font)
	_, _, _ = procSetBkMode.Call(hdc, uintptr(bkTransparent))

	for i, it := range p.items {
		top := rc.top + p.tops[i]
		if it.Separator {
			mid := top + (p.size.separatorHeight-p.size.separatorLine)/2
			line := rect{
				left:   rc.left + p.size.hoverInset,
				top:    mid,
				right:  rc.right - p.size.hoverInset,
				bottom: mid + p.size.separatorLine,
			}
			_, _, _ = procFillRect.Call(hdc, uintptr(unsafe.Pointer(&line)), p.brushSeparator)
			continue
		}
		row := rect{left: rc.left, top: top, right: rc.right, bottom: top + p.size.itemHeight}
		if i == p.hovered {
			highlight := row
			highlight.left += p.size.hoverInset
			highlight.right -= p.size.hoverInset
			_, _, _ = procFillRect.Call(hdc, uintptr(unsafe.Pointer(&highlight)), p.brushHover)
		}
		text := row
		text.left += p.size.padH
		text.right -= p.size.padH
		labelColor, shortcutColor := p.colors.text, p.colors.shortcut
		if it.Disabled {
			labelColor, shortcutColor = p.colors.disabled, p.colors.disabled
		}
		drawText(hdc, it.Label, text, labelColor, dtLeft)
		if it.Shortcut != "" {
			drawText(hdc, it.Shortcut, text, shortcutColor, dtRight)
		}
	}
}

func drawText(hdc uintptr, s string, r rect, color uint32, align uintptr) {
	u16, err := windows.UTF16FromString(s)
	if err != nil {
		return
	}
	_, _, _ = procSetTextColor.Call(hdc, uintptr(color))
	// DrawTextW: -1 means "string is null-terminated".
	strLen := int32(-1)
	_, _, _ = procDrawTextW.Call(
		hdc,
		uintptr(unsafe.Pointer(&u16[0])),
		uintptr(uint32(strLen)),
		uintptr(unsafe.Pointer(&r)),
		align|dtVCenter|dtSingleLine|dtNoPrefix,
	)
}

// measurer returns a function measuring strings in the menu font. The screen
// DC it selects the font into is released right away: layout runs
// synchronously, before Show goes on.
func (p *Menu) measurer() func(string) int32 {
	return func(s string) int32 {
		u16, err := windows.UTF16FromString(s)
		if err != nil || len(u16) < 2 {
			return 0
		}
		hdc, _, _ := procGetDC.Call(0)
		defer procReleaseDC.Call(0, hdc)
		old, _, _ := procSelectObject.Call(hdc, p.font)
		defer procSelectObject.Call(hdc, old)
		var sz sizeStruct
		_, _, _ = procGetTextExtentPoint32.Call(
			hdc,
			uintptr(unsafe.Pointer(&u16[0])),
			uintptr(len(u16)-1), // without the null terminator
			uintptr(unsafe.Pointer(&sz)),
		)
		return sz.cx
	}
}

// prepareResources creates the font for the current DPI and the brushes for
// the current colours; the theme may have changed since the last Show.
func (p *Menu) prepareResources() {
	p.deleteBrushes()
	if p.font != 0 && p.fontDPI != p.size.dpi {
		_, _, _ = procDeleteObject.Call(p.font)
		p.font = 0
	}
	if p.font == 0 {
		name, _ := windows.UTF16PtrFromString("Segoe UI")
		nHeight := -p.size.fontHeight // negative: character height in pixels
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
		p.font = h
		p.fontDPI = p.size.dpi
	}
	p.brushBg = solidBrush(p.colors.bg)
	p.brushHover = solidBrush(p.colors.hover)
	p.brushSeparator = solidBrush(p.colors.separator)
}

func (p *Menu) deleteBrushes() {
	for _, b := range []*uintptr{&p.brushBg, &p.brushHover, &p.brushSeparator} {
		if *b != 0 {
			_, _, _ = procDeleteObject.Call(*b)
			*b = 0
		}
	}
}

func solidBrush(color uint32) uintptr {
	b, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return b
}

var (
	classOnce sync.Once
	className *uint16

	// menusMu guards menus, the open menus by window. The window procedure
	// is shared by every menu, whichever thread shows it.
	menusMu sync.Mutex
	menus   = map[uintptr]*Menu{}
)

func registerClass() {
	classOnce.Do(func() {
		className, _ = windows.UTF16PtrFromString("CopyNotePopupMenu")
		hInstance, _, _ := procGetModuleHandleW.Call(0)
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		wc := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   syscall.NewCallback(wndProc),
			hInstance:     hInstance,
			hCursor:       cursor,
			lpszClassName: className,
		}
		_, _, _ = procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
}

// Win32 / GDI constants used by the menu window.
const (
	wsPopup     = 0x80000000
	wsBorder    = 0x00800000
	wsExToolWnd = 0x00000080
	wsExTopmost = 0x00000008

	wmActivate   = 0x0006
	wmKillFocus  = 0x0008
	wmPaint      = 0x000F
	wmGetDlgCode = 0x0087
	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104
	wmMouseMove  = 0x0200
	wmLButtonUp  = 0x0202
	wmMouseLeave = 0x02A3
	waInactive   = 0

	vkReturn = 0x0D
	vkEscape = 0x1B
	vkSpace  = 0x20
	vkEnd    = 0x23
	vkHome   = 0x24
	vkUp     = 0x26
	vkDown   = 0x28
	vkApps   = 0x5D

	tmeLeave = 0x00000002

	dtLeft       = 0x00000000
	dtRight      = 0x00000002
	dtVCenter    = 0x00000004
	dtSingleLine = 0x00000020
	dtNoPrefix   = 0x00000800

	bkTransparent   = 1
	dwmwcpRound     = 2
	idcArrow        = 32512
	dlgcWantAllKeys = 0x0004

	fwNormal         = 400
	defaultCharset   = 1
	clearTypeQuality = 5
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

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

var (
	moduser32   = windows.NewLazySystemDLL("user32.dll")
	modgdi32    = windows.NewLazySystemDLL("gdi32.dll")
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW = moduser32.NewProc("RegisterClassExW")
	procCreateWindowExW  = moduser32.NewProc("CreateWindowExW")
	procDestroyWindow    = moduser32.NewProc("DestroyWindow")
	procDefWindowProcW   = moduser32.NewProc("DefWindowProcW")
	procLoadCursorW      = moduser32.NewProc("LoadCursorW")
	procBeginPaint       = moduser32.NewProc("BeginPaint")
	procEndPaint         = moduser32.NewProc("EndPaint")
	procFillRect         = moduser32.NewProc("FillRect")
	procDrawTextW        = moduser32.NewProc("DrawTextW")
	procInvalidateRect   = moduser32.NewProc("InvalidateRect")
	procGetClientRect    = moduser32.NewProc("GetClientRect")
	procTrackMouseEvent  = moduser32.NewProc("TrackMouseEvent")
	procGetDC            = moduser32.NewProc("GetDC")
	procReleaseDC        = moduser32.NewProc("ReleaseDC")

	procCreateFontW          = modgdi32.NewProc("CreateFontW")
	procDeleteObject         = modgdi32.NewProc("DeleteObject")
	procSelectObject         = modgdi32.NewProc("SelectObject")
	procSetBkMode            = modgdi32.NewProc("SetBkMode")
	procSetTextColor         = modgdi32.NewProc("SetTextColor")
	procCreateSolidBrush     = modgdi32.NewProc("CreateSolidBrush")
	procGetTextExtentPoint32 = modgdi32.NewProc("GetTextExtentPoint32W")

	procGetModuleHandleW = modkernel32.NewProc("GetModuleHandleW")
)

//go:build windows

// Package tray installs a system-tray icon with an Open / Quit popup
// menu and runs its own Win32 message loop on a dedicated OS thread.
//
// The package owns three Win32 objects across its lifetime:
//   - a message-only HWND that receives tray callback and broadcast
//     messages,
//   - the tray icon registered via Shell_NotifyIcon,
//   - a popup HMENU shown on right-click.
//
// All cleanup happens when Run() returns (loop exited via PostQuitMessage).
package tray

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"copynote/internal/winutil"
)

// currentIconID tracks which RT_GROUP_ICON ID is active on the tray
// so ReloadIcon can skip work when the theme hasn't actually changed.
// Scoped to the tray package because only one Tray runs per process.
var currentIconID uintptr

// loadTrayIcon reads the system theme and returns an hIcon of the
// appropriate variant (dark stroke on light taskbar, light stroke on
// dark taskbar). Falls back to the stock application icon if neither
// embedded resource can be loaded.
func loadTrayIcon(hInstance uintptr) uintptr {
	var resourceID uintptr = iconResourceDark
	if !winutil.IsSystemLightTheme() {
		resourceID = iconResourceLight
	}
	cx, _, _ := procGetSystemMetrics.Call(uintptr(smCxSmIcon))
	cy, _, _ := procGetSystemMetrics.Call(uintptr(smCySmIcon))
	hIcon, _, _ := procLoadImageW.Call(
		hInstance,
		resourceID,
		uintptr(imageIcon),
		cx, cy,
		uintptr(lrDefaultColor|lrSharedIcon),
	)
	if hIcon == 0 {
		hIcon, _, _ = procLoadIconW.Call(0, uintptr(idiApplication))
		currentIconID = 0
		return hIcon
	}
	currentIconID = resourceID
	return hIcon
}

// Tray is a one-shot tray-icon controller. Construct it, set the
// callbacks, then call Run() from a goroutine that has called
// runtime.LockOSThread().
type Tray struct {
	// OnShow is invoked from the tray thread when the user explicitly
	// wants to surface the main window (Open menu item, or a
	// single-instance "show" broadcast from a second exe launch).
	OnShow func()

	// OnToggle is invoked from the tray thread on left click of the
	// tray icon. The handler should hide the window if it is currently
	// visible, otherwise show and focus it. Falls back to OnShow if
	// nil.
	OnToggle func()

	// OnSettings is invoked from the tray thread when the user picks
	// Settings from the popup menu.
	OnSettings func()

	// OnQuit is invoked from the tray thread when the user picks Quit
	// from the popup menu. After OnQuit returns, the tray's message
	// loop is shut down via PostQuitMessage.
	OnQuit func()

	// GetLocale returns the current UI locale code (e.g. "en", "ru").
	// Called each time the popup menu is shown so labels are up to date.
	GetLocale func() string

	hwnd      uintptr
	showMsgID uint32
	added     bool

	startOnce sync.Once
	startErr  error
}

// Tray callback message ID. WM_APP-range avoids collisions with system
// messages and stock control messages.
const trayCallbackMsg = winutil.WM_APP + 1

// Menu command IDs.
const (
	menuIDOpen     = 1001
	menuIDSettings = 1002
	menuIDQuit     = 1003
)

// Win32 constants used in this file.
const (
	cwUseDefault = 0x80000000

	// Shell_NotifyIcon actions and flags
	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	// LoadIcon stock identifier (IDI_APPLICATION).
	idiApplication = 32512

	// LoadImage parameters for loading RT_ICON resources from the
	// running exe's embedded resources.
	imageIcon      = 1
	lrDefaultColor = 0x00000000
	lrSharedIcon   = 0x00008000

	// RT_GROUP_ICON resource IDs assigned by rsrc when it packed the
	// two .ico files into resource_windows_amd64.syso, in order:
	//   icon-dark.ico  → id 1 (dark stroke, used on LIGHT taskbar)
	//   icon-light.ico → id 9 (light stroke, used on DARK taskbar)
	// The gap (1 → 9) comes from rsrc allocating a per-image id for
	// each of the 7 sizes in the first ico before moving on.
	iconResourceDark  = 1
	iconResourceLight = 9

	// GetSystemMetrics — small icon dimensions (16 px at 100 % DPI).
	smCxSmIcon = 49
	smCySmIcon = 50

	// Custom tray-internal message asking the wndproc to reload the
	// tray icon (used when the system theme changes). WM_APP + 2
	// because + 1 is already taken by trayCallbackMsg.
	msgReloadIcon = 0x8000 + 2 // WM_APP + 2

	// CreateWindowEx hwndParent special value: HWND_MESSAGE creates a
	// message-only window not visible on the desktop.
	hwndMessage = ^uintptr(0) - 2 // (HWND)-3
)

// NOTIFYICONDATAW — first 4 fields needed for NIM_ADD with NIF_MESSAGE
// | NIF_ICON | NIF_TIP. Only fields up to szTip are populated; the
// later balloon-tip fields are left zero. Note: cbSize must equal the
// full struct size, so we declare all fields the runtime expects.
type notifyIconDataW struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     uintptr
}

// WNDCLASSEXW for RegisterClassExW.
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

// MSG struct for GetMessageW.
type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

// POINT for GetCursorPos.
type point struct {
	x, y int32
}

var (
	moduser32   = windows.NewLazySystemDLL("user32.dll")
	modshell32  = windows.NewLazySystemDLL("shell32.dll")
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW = moduser32.NewProc("RegisterClassExW")
	procCreateWindowExW  = moduser32.NewProc("CreateWindowExW")
	procDestroyWindow    = moduser32.NewProc("DestroyWindow")
	procDefWindowProcW   = moduser32.NewProc("DefWindowProcW")
	procGetMessageW      = moduser32.NewProc("GetMessageW")
	procTranslateMessage = moduser32.NewProc("TranslateMessage")
	procDispatchMessageW = moduser32.NewProc("DispatchMessageW")
	procPostQuitMessage  = moduser32.NewProc("PostQuitMessage")
	procPostMessageW     = moduser32.NewProc("PostMessageW")
	procLoadIconW        = moduser32.NewProc("LoadIconW")
	procLoadImageW       = moduser32.NewProc("LoadImageW")
	procGetCursorPos     = moduser32.NewProc("GetCursorPos")

	procShellNotifyIconW = modshell32.NewProc("Shell_NotifyIconW")

	procGetModuleHandleW = modkernel32.NewProc("GetModuleHandleW")

	// Singleton state — only one Tray runs per process.
	instance     *Tray
	wndProcCB    uintptr
	classNamePtr *uint16
)

// Run installs the tray icon and blocks on a Win32 message loop until
// Stop() is called or the user picks Quit. Must be called from a
// goroutine that has called runtime.LockOSThread() — Win32 windows
// have hard thread affinity.
func (t *Tray) Run() error {
	t.startOnce.Do(func() { t.startErr = t.setup() })
	if t.startErr != nil {
		return t.startErr
	}
	defer t.teardown()

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(r) {
		case 0: // WM_QUIT
			return nil
		case -1: // error
			return errors.New("tray: GetMessage failed")
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// Stop posts WM_QUIT to the tray's hidden window so Run() returns.
// Safe to call from any goroutine.
func (t *Tray) Stop() {
	if t.hwnd == 0 {
		return
	}
	_, _, _ = procPostMessageW.Call(t.hwnd, uintptr(winutil.WM_QUIT), 0, 0)
}

// ShowMessageID returns the registered window message that triggers
// OnShow when broadcast from another process. Used by main.go to wake
// up an existing instance from the second exe launch.
func (t *Tray) ShowMessageID() uint32 { return t.showMsgID }

func (t *Tray) setup() error {
	if instance != nil {
		return errors.New("tray: another instance already running in this process")
	}
	instance = t

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	// Register the window class once. classNamePtr is a package-level
	// var so the underlying memory survives until process exit.
	if classNamePtr == nil {
		ptr, err := windows.UTF16PtrFromString("CopyNoteTrayWnd")
		if err != nil {
			return fmt.Errorf("class name: %w", err)
		}
		classNamePtr = ptr
	}
	if wndProcCB == 0 {
		wndProcCB = syscall.NewCallback(trayWndProc)
	}

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProcCB,
		hInstance:     hInstance,
		lpszClassName: classNamePtr,
	}
	atom, _, regErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		// ERROR_CLASS_ALREADY_EXISTS (1410) is fine — the class is
		// process-global, so subsequent Tray instances reuse it.
		if !errors.Is(regErr, windows.Errno(1410)) {
			return fmt.Errorf("RegisterClassExW: %w", regErr)
		}
	}

	hwnd, _, createErr := procCreateWindowExW.Call(
		0,                                 // dwExStyle
		uintptr(unsafe.Pointer(classNamePtr)), // lpClassName
		uintptr(unsafe.Pointer(classNamePtr)), // lpWindowName
		0,                                 // dwStyle
		0, 0, 0, 0,                        // x, y, w, h
		hwndMessage, // hWndParent (HWND_MESSAGE)
		0,           // hMenu
		hInstance,   // hInstance
		0,           // lpParam
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %w", createErr)
	}
	t.hwnd = hwnd

	// IPC channel: a system-wide registered window message that the
	// second exe launch broadcasts to wake us up.
	id, err := winutil.RegisterWindowMessage("dev.copynote.app.SHOW")
	if err != nil {
		return fmt.Errorf("register show msg: %w", err)
	}
	t.showMsgID = id

	// Tray icon: pick the dark-stroke or light-stroke variant based
	// on the system's current taskbar theme. See loadTrayIcon for
	// details; this initial call caches which resource ID is active
	// so later theme swaps can compare cheaply.
	hIcon := loadTrayIcon(hInstance)

	nid := notifyIconDataW{
		cbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:             hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: trayCallbackMsg,
		hIcon:            hIcon,
	}
	copyTip(&nid.szTip, "CopyNote")

	if r, _, addErr := procShellNotifyIconW.Call(uintptr(nimAdd), uintptr(unsafe.Pointer(&nid))); r == 0 {
		return fmt.Errorf("Shell_NotifyIconW NIM_ADD: %w", addErr)
	}
	t.added = true
	return nil
}

func (t *Tray) teardown() {
	if t.added {
		nid := notifyIconDataW{
			cbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
			hWnd:   t.hwnd,
			uID:    1,
		}
		_, _, _ = procShellNotifyIconW.Call(uintptr(nimDelete), uintptr(unsafe.Pointer(&nid)))
		t.added = false
	}
	if t.hwnd != 0 {
		_, _, _ = procDestroyWindow.Call(t.hwnd)
		t.hwnd = 0
	}
	releasePopupResources()
	instance = nil
}

func copyTip(dst *[128]uint16, s string) {
	u16, _ := windows.UTF16FromString(s)
	if len(u16) > len(dst) {
		u16 = u16[:len(dst)]
	}
	for i := range u16 {
		dst[i] = u16[i]
	}
}

// trayWndProc dispatches messages received by the hidden tray window.
// Runs on the same OS thread as the tray's GetMessage loop.
func trayWndProc(hwnd, msgID, wParam, lParam uintptr) uintptr {
	t := instance
	switch uint32(msgID) {
	case trayCallbackMsg:
		// lParam is the actual mouse event from the tray icon.
		switch uint32(lParam) {
		case winutil.WM_LBUTTONUP:
			if t != nil {
				switch {
				case t.OnToggle != nil:
					t.OnToggle()
				case t.OnShow != nil:
					t.OnShow()
				}
			}
		case winutil.WM_RBUTTONUP:
			if t != nil {
				showTrayPopup(t)
			}
		}
		return 0

	case msgReloadIcon:
		if t != nil {
			reloadTrayIconForTheme(t)
		}
		return 0

	default:
		// Custom show-message broadcast from a second instance launch.
		if t != nil && uint32(msgID) == t.showMsgID {
			if t.OnShow != nil {
				t.OnShow()
			}
			return 0
		}
		r, _, _ := procDefWindowProcW.Call(hwnd, msgID, wParam, lParam)
		return r
	}
}

// reloadTrayIconForTheme re-reads the system theme and, if it differs
// from the icon currently shown in the tray, swaps the icon via
// Shell_NotifyIcon(NIM_MODIFY). Must run on the tray's OS thread.
func reloadTrayIconForTheme(t *Tray) {
	desired := uintptr(iconResourceDark)
	if !winutil.IsSystemLightTheme() {
		desired = iconResourceLight
	}
	if desired == currentIconID {
		return
	}
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	hIcon := loadTrayIcon(hInstance)
	if hIcon == 0 {
		return
	}
	nid := notifyIconDataW{
		cbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:   t.hwnd,
		uID:    1,
		uFlags: nifIcon,
		hIcon:  hIcon,
	}
	_, _, _ = procShellNotifyIconW.Call(uintptr(nimModify), uintptr(unsafe.Pointer(&nid)))
}

// ReloadIcon asks the tray's message loop to re-evaluate the system
// theme and swap the icon to the matching variant. Safe to call from
// any goroutine — it just posts a message to the tray's own window.
func (t *Tray) ReloadIcon() {
	if t == nil || t.hwnd == 0 {
		return
	}
	winutil.PostMessage(t.hwnd, msgReloadIcon, 0, 0)
}

// Tray menu labels by locale. Falls back to English.
var trayMenuLabels = map[string][3]string{
	"en": {"Open CopyNote", "Settings", "Quit"},
	"ru": {"Открыть CopyNote", "Настройки", "Выход"},
}

func showTrayPopup(t *Tray) {
	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	locale := "en"
	if t.GetLocale != nil {
		locale = t.GetLocale()
	}
	labels, ok := trayMenuLabels[locale]
	if !ok {
		labels = trayMenuLabels["en"]
	}

	items := []popupItem{
		{id: menuIDOpen, label: labels[0]},
		{id: menuIDSettings, label: labels[1]},
		{id: menuIDQuit, label: labels[2]},
	}
	showCustomPopup(items, pt.x, pt.y, func(id uint32) {
		switch id {
		case menuIDOpen:
			if t.OnShow != nil {
				t.OnShow()
			}
		case menuIDSettings:
			if t.OnSettings != nil {
				t.OnSettings()
			}
		case menuIDQuit:
			if t.OnQuit != nil {
				t.OnQuit()
			}
			_, _, _ = procPostQuitMessage.Call(0)
		}
	})
}

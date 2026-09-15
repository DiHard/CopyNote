//go:build windows

// Package tray installs a system-tray icon with an Open / Quit popup
// menu and runs its own Win32 message loop on a dedicated OS thread.
//
// The package owns three Win32 objects across its lifetime:
//   - a message-only HWND that receives tray callbacks and the show
//     request posted by a second exe launch (see ShowRunningInstance),
//   - the tray icon registered via Shell_NotifyIcon,
//   - a right-click menu, drawn by internal/popupmenu.
//
// All cleanup happens when Run() returns (loop exited via PostQuitMessage).
package tray

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"copynote/internal/hotkey"
	"copynote/internal/popupmenu"
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
	// The process is DPI aware, so this is the small-icon size for the
	// system DPI (20 px at 125 %) — the size the tray draws, no stretching.
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
	// wants to surface the main window (Open menu item, or the show
	// request from a second exe launch).
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

	// OnHotkey is invoked from the tray thread when the global hotkey fires.
	// Same intent as a left click on the icon, so it should toggle.
	OnHotkey func()

	// Hotkey is the stored preference ("" = the built-in default, "off" =
	// disabled) registered during setup. Later changes go through SetHotkey.
	Hotkey string

	// ShowOnStart surfaces the window as soon as the UI is ready, without
	// waiting for a click. Set it for a launch the user performed
	// themselves — a Windows sign-in must not pop the window up. Read once
	// during setup, so it has to be assigned before Run.
	ShowOnStart bool

	hwnd      uintptr
	showMsgID uint32
	added     bool
	ready     atomic.Bool // true once WebView2 has finished loading
	timerID   uintptr     // pulse timer (0 = not running)

	// pending is a request that arrived before WebView2 finished loading;
	// it is honoured on msgSetReady. Tray thread only.
	pending pendingRequest

	// menu is the right-click menu. Tray thread only.
	menu popupmenu.Menu

	// hotkeyMu guards the combination SetHotkey hands to the tray thread.
	hotkeyMu      sync.Mutex
	hotkeyWanted  hotkey.Spec
	hotkeyEnabled bool
	hotkeyOn      bool // a registration is currently live; tray thread only

	startOnce sync.Once
	startErr  error
}

// pendingRequest is what the user asked for while the UI was still loading.
// Sliding out a blank window during a cold start looks like a crash, and
// dropping the request looks like a dead icon, so it waits instead.
type pendingRequest uint8

const (
	pendingNone pendingRequest = iota
	pendingShow
	pendingSettings
)

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
	nimAdd     = 0x00000000
	nimModify  = 0x00000001
	nimDelete  = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifState   = 0x00000008

	nisHidden = 0x00000001

	pulseTimerID    = 42
	pulseIntervalMs = 80 // ms between animation frames (~12 fps)
	pulseFrames     = 10 // number of opacity steps in one direction

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
	modgdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procRegisterClassExW   = moduser32.NewProc("RegisterClassExW")
	procCreateWindowExW    = moduser32.NewProc("CreateWindowExW")
	procDestroyWindow      = moduser32.NewProc("DestroyWindow")
	procDefWindowProcW     = moduser32.NewProc("DefWindowProcW")
	procGetMessageW        = moduser32.NewProc("GetMessageW")
	procTranslateMessage   = moduser32.NewProc("TranslateMessage")
	procDispatchMessageW   = moduser32.NewProc("DispatchMessageW")
	procPostQuitMessage    = moduser32.NewProc("PostQuitMessage")
	procPostMessageW       = moduser32.NewProc("PostMessageW")
	procLoadIconW          = moduser32.NewProc("LoadIconW")
	procLoadImageW         = moduser32.NewProc("LoadImageW")
	procGetCursorPos       = moduser32.NewProc("GetCursorPos")
	procSetTimer           = moduser32.NewProc("SetTimer")
	procKillTimer          = moduser32.NewProc("KillTimer")
	procRegisterHotKey     = moduser32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = moduser32.NewProc("UnregisterHotKey")
	procGetIconInfo        = moduser32.NewProc("GetIconInfo")
	procCreateIconIndirect = moduser32.NewProc("CreateIconIndirect")
	procDestroyIcon        = moduser32.NewProc("DestroyIcon")

	procGetSystemMetrics = moduser32.NewProc("GetSystemMetrics")
	procGetDC            = moduser32.NewProc("GetDC")
	procReleaseDC        = moduser32.NewProc("ReleaseDC")

	// GDI procs for the pulse animation.
	procDeleteObject     = modgdi32.NewProc("DeleteObject")
	procGetObjectW       = modgdi32.NewProc("GetObjectW")
	procGetDIBits        = modgdi32.NewProc("GetDIBits")
	procCreateDIBSection = modgdi32.NewProc("CreateDIBSection")

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

func (t *Tray) setup() error {
	if instance != nil {
		return errors.New("tray: another instance already running in this process")
	}
	instance = t

	hInstance, _, _ := procGetModuleHandleW.Call(0)

	// Register the window class once. classNamePtr is a package-level
	// var so the underlying memory survives until process exit.
	if classNamePtr == nil {
		ptr, err := windows.UTF16PtrFromString(trayClassName)
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
		0,                                     // dwExStyle
		uintptr(unsafe.Pointer(classNamePtr)), // lpClassName
		uintptr(unsafe.Pointer(classNamePtr)), // lpWindowName
		0,                                     // dwStyle
		0, 0, 0, 0,                            // x, y, w, h
		hwndMessage, // hWndParent (HWND_MESSAGE)
		0,           // hMenu
		hInstance,   // hInstance
		0,           // lpParam
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %w", createErr)
	}
	t.hwnd = hwnd

	// IPC channel: a system-wide registered window message that a second
	// exe launch posts to this window (see ShowRunningInstance).
	id, err := winutil.RegisterWindowMessage(showMessageName)
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
	copyTip(&nid.szTip, t.tip())

	if r, _, addErr := procShellNotifyIconW.Call(uintptr(nimAdd), uintptr(unsafe.Pointer(&nid))); r == 0 {
		return fmt.Errorf("Shell_NotifyIconW NIM_ADD: %w", addErr)
	}
	t.added = true

	// A launch the user performed themselves opens the window as soon as
	// the UI can render it. Reusing the pending slot keeps that on the
	// same path as a click and a second exe launch.
	if t.ShowOnStart {
		t.pending = pendingShow
	}

	// A combination another program already owns is a normal outcome, not a
	// startup failure: the app still works from the icon, and Settings shows
	// the problem once the UI asks again through SetHotkey.
	if spec, enabled, err := hotkey.Resolve(t.Hotkey); err != nil {
		log.Printf("hotkey %q: %v", t.Hotkey, err)
	} else {
		t.hotkeyMu.Lock()
		t.hotkeyWanted, t.hotkeyEnabled = spec, enabled
		t.hotkeyMu.Unlock()
		if code := applyHotkey(t); code != 0 {
			log.Printf("register hotkey %q: %v", t.Hotkey, windows.Errno(code))
		}
	}

	// Build pulse animation frames from the current icon and start timer.
	buildPulseIcons(hIcon)
	procSetTimer.Call(hwnd, pulseTimerID, pulseIntervalMs, 0)
	t.timerID = pulseTimerID
	return nil
}

func (t *Tray) teardown() {
	if t.hotkeyOn {
		_, _, _ = procUnregisterHotKey.Call(t.hwnd, hotkeyID)
		t.hotkeyOn = false
	}
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
	t.menu.Release()
	instance = nil
}

func copyTip(dst *[128]uint16, s string) {
	u16, _ := windows.UTF16FromString(s)
	if len(u16) > len(dst) {
		// The tip is localized now, so a long translation has to truncate
		// to a still-terminated string rather than fill the buffer.
		u16 = u16[:len(dst)]
		u16[len(u16)-1] = 0
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
			if t == nil {
				break
			}
			// Cold start: remember the click instead of dropping it. A
			// second click must not toggle the queued request back off —
			// there is no window on screen for the user to be toggling.
			if !t.ready.Load() {
				t.pending = pendingShow
				break
			}
			switch {
			case t.OnToggle != nil:
				t.OnToggle()
			case t.OnShow != nil:
				t.OnShow()
			}
		case winutil.WM_RBUTTONUP:
			if t != nil {
				showTrayPopup(t)
			}
		}
		return 0

	case 0x0113: // WM_TIMER
		if t != nil && wParam == pulseTimerID {
			advancePulseFrame(t)
		}
		return 0

	case msgReloadIcon:
		if t != nil {
			reloadTrayIconForTheme(t)
		}
		return 0

	case msgSetReady:
		if t == nil {
			return 0
		}
		if t.timerID != 0 {
			procKillTimer.Call(hwnd, t.timerID)
			t.timerID = 0
			// Restore the full-opacity theme-appropriate icon.
			hInst, _, _ := procGetModuleHandleW.Call(0)
			hIcon := loadTrayIcon(hInst)
			nid := notifyIconDataW{
				cbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
				hWnd:   t.hwnd,
				uID:    1,
				uFlags: nifIcon,
				hIcon:  hIcon,
			}
			_, _, _ = procShellNotifyIconW.Call(uintptr(nimModify), uintptr(unsafe.Pointer(&nid)))
			destroyPulseIcons()
		}
		// ready is already set by SetReady, so this picks the "click to
		// open" wording.
		applyTip(t, t.tip())
		switch t.pending {
		case pendingShow:
			if t.OnShow != nil {
				t.OnShow()
			}
		case pendingSettings:
			if t.OnSettings != nil {
				t.OnSettings()
			}
		}
		t.pending = pendingNone
		return 0

	case msgRefreshTip:
		if t != nil {
			applyTip(t, t.tip())
		}
		return 0

	case msgApplyHotkey:
		if t == nil {
			return 0
		}
		return applyHotkey(t)

	case wmHotkey:
		// Same intent as a left click on the icon, including during the cold
		// start — the request waits rather than opening a blank window.
		if t == nil {
			return 0
		}
		if !t.ready.Load() {
			t.pending = pendingShow
			return 0
		}
		if t.OnHotkey != nil {
			t.OnHotkey()
		}
		return 0

	default:
		// Show request from a second exe launch (see ShowRunningInstance).
		if t != nil && uint32(msgID) == t.showMsgID {
			if !t.ready.Load() {
				// Still loading: show once WebView2 is ready instead of
				// sliding out an empty window.
				t.pending = pendingShow
				return 0
			}
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

// ── Pulse animation ─────────────────────────────────────────────
//
// Pre-generates N frames of the tray icon with varying alpha
// (0.2 → 1.0 → 0.2 triangle wave). A WM_TIMER cycles through
// them to create a smooth "breathing" pulse during loading.

var (
	pulseIcons []uintptr // HICON handles for each frame
	pulseIdx   int       // current frame index
	pulseDir   int       // +1 or -1
)

// buildPulseIcons creates pulseFrames HICON copies of srcIcon with
// alpha scaled from ~20% to 100%. Must be called on the tray thread.
func buildPulseIcons(srcIcon uintptr) {
	if len(pulseIcons) > 0 {
		return // already built
	}
	pulseIcons = make([]uintptr, pulseFrames)
	for i := 0; i < pulseFrames; i++ {
		// Alpha fraction: 0.2 .. 1.0 linearly
		alpha := 0.2 + 0.8*float64(i)/float64(pulseFrames-1)
		h := createAlphaIcon(srcIcon, alpha)
		if h == 0 {
			h = srcIcon // fallback
		}
		pulseIcons[i] = h
	}
	pulseIdx = pulseFrames - 1 // start at full opacity
	pulseDir = -1              // fade out first
}

// advancePulseFrame sets the next pulse frame on the tray icon.
func advancePulseFrame(t *Tray) {
	if len(pulseIcons) == 0 {
		return
	}
	pulseIdx += pulseDir
	if pulseIdx >= pulseFrames {
		pulseIdx = pulseFrames - 1
		pulseDir = -1
	} else if pulseIdx < 0 {
		pulseIdx = 0
		pulseDir = 1
	}
	nid := notifyIconDataW{
		cbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:   t.hwnd,
		uID:    1,
		uFlags: nifIcon,
		hIcon:  pulseIcons[pulseIdx],
	}
	_, _, _ = procShellNotifyIconW.Call(uintptr(nimModify), uintptr(unsafe.Pointer(&nid)))
}

// destroyPulseIcons frees all generated HICON handles.
func destroyPulseIcons() {
	for _, h := range pulseIcons {
		procDestroyIcon.Call(h)
	}
	pulseIcons = nil
}

// BITMAPINFOHEADER for GetDIBits/SetDIBits.
type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type bitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

// createAlphaIcon duplicates srcIcon with every pixel's alpha
// multiplied by alphaFrac (0.0–1.0). Returns a new HICON.
func createAlphaIcon(srcIcon uintptr, alphaFrac float64) uintptr {
	// Get source icon bitmaps.
	var ii iconInfo
	r, _, _ := procGetIconInfo.Call(srcIcon, uintptr(unsafe.Pointer(&ii)))
	if r == 0 {
		return 0
	}
	defer procDeleteObject.Call(ii.hbmMask)
	defer procDeleteObject.Call(ii.hbmColor)

	// Get bitmap dimensions.
	var bm bitmap
	procGetObjectW.Call(ii.hbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	w := int(bm.bmWidth)
	h := int(bm.bmHeight)
	if w == 0 || h == 0 {
		return 0
	}

	// Read BGRA pixel data from the color bitmap.
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)

	bih := bitmapInfoHeader{
		biSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		biWidth:    int32(w),
		biHeight:   -int32(h), // top-down
		biPlanes:   1,
		biBitCount: 32,
	}
	pixels := make([]byte, w*h*4)
	procGetDIBits.Call(hdc, ii.hbmColor, 0, uintptr(h),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bih)), 0)

	// Scale alpha channel of every pixel.
	for i := 3; i < len(pixels); i += 4 {
		a := float64(pixels[i]) * alphaFrac
		if a > 255 {
			a = 255
		}
		pixels[i] = byte(a)
		// Pre-multiply RGB by the new alpha ratio.
		ratio := alphaFrac
		pixels[i-3] = byte(float64(pixels[i-3]) * ratio)
		pixels[i-2] = byte(float64(pixels[i-2]) * ratio)
		pixels[i-1] = byte(float64(pixels[i-1]) * ratio)
	}

	// Create a new color bitmap with modified pixels.
	var pBits uintptr
	bihUp := bih
	bihUp.biHeight = int32(h) // bottom-up for CreateDIBSection
	hbm, _, _ := procCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bihUp)),
		0, uintptr(unsafe.Pointer(&pBits)), 0, 0)
	if hbm == 0 {
		return 0
	}

	// Copy modified pixels into the new bitmap (flip rows for bottom-up).
	stride := w * 4
	for row := 0; row < h; row++ {
		srcOff := row * stride
		dstOff := (h - 1 - row) * stride
		winutil.WriteNativeMemory(pBits+uintptr(dstOff), pixels[srcOff:srcOff+stride])
	}

	// Build a new icon from modified color bitmap + original mask.
	newII := iconInfo{
		fIcon:    1,
		hbmMask:  ii.hbmMask,
		hbmColor: hbm,
	}
	newIcon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&newII)))
	procDeleteObject.Call(hbm)
	return newIcon
}

// SetReady stops the pulse animation, ensures the icon is visible,
// and enables LMB click handling. Safe to call from any goroutine.
func (t *Tray) SetReady() {
	if t == nil {
		return
	}
	t.ready.Store(true)
	// Post a message to the tray thread to stop the timer and
	// ensure the icon is in a visible state.
	if t.hwnd != 0 {
		winutil.PostMessage(t.hwnd, msgSetReady, 0, 0)
	}
}

const msgSetReady = winutil.WM_APP + 3

// ── Global hotkey ────────────────────────────────────────────────────
// RegisterHotKey delivers WM_HOTKEY to the thread owning the window, so all
// of this lives on the tray thread; SetHotkey reaches it with a blocking
// SendMessage and reads the outcome out of the return value. That is safe
// here because the tray loop never waits on the caller.

const (
	msgApplyHotkey = winutil.WM_APP + 5
	wmHotkey       = 0x0312
	hotkeyID       = 1
)

// SetHotkey re-registers the global hotkey and reports whether Windows
// accepted it. A combination another program already owns fails here, which
// is the whole reason this is not fire-and-forget.
func (t *Tray) SetHotkey(setting string) error {
	if t == nil || t.hwnd == 0 {
		return errors.New("tray is not running")
	}
	spec, enabled, err := hotkey.Resolve(setting)
	if err != nil {
		return err
	}
	t.hotkeyMu.Lock()
	t.hotkeyWanted, t.hotkeyEnabled = spec, enabled
	t.hotkeyMu.Unlock()

	if code := winutil.SendMessage(t.hwnd, msgApplyHotkey, 0, 0); code != 0 {
		// Windows' own text, unadorned: the UI already knows which
		// combination it asked for and recognises the ordinary "already
		// registered" case to phrase it for a person. Prefixing the raw
		// setting here printed "register : …" for the empty default.
		return windows.Errno(code)
	}
	return nil
}

// liveHotkey is the combination currently registered, valid while
// t.hotkeyOn. Package-scoped like currentIconID: one Tray per process, and
// only the tray thread touches it.
var liveHotkey hotkey.Spec

// registerHotkey returns 0, or the Win32 error code. Tray thread only.
func registerHotkey(t *Tray, spec hotkey.Spec) uintptr {
	// MOD_NOREPEAT: holding the combination down must open the window once,
	// not once per key repeat.
	r, _, err := procRegisterHotKey.Call(
		t.hwnd, hotkeyID, uintptr(spec.Mods|hotkey.ModNoRepeat), uintptr(spec.VK))
	if r != 0 {
		return 0
	}
	if errno, ok := err.(windows.Errno); ok && errno != 0 {
		return uintptr(errno)
	}
	return uintptr(windows.ERROR_HOTKEY_ALREADY_REGISTERED)
}

// applyHotkey runs on the tray thread. Returns 0, or the Win32 error code.
func applyHotkey(t *Tray) uintptr {
	prev, prevOn := liveHotkey, t.hotkeyOn
	if prevOn {
		_, _, _ = procUnregisterHotKey.Call(t.hwnd, hotkeyID)
		t.hotkeyOn = false
	}
	t.hotkeyMu.Lock()
	spec, enabled := t.hotkeyWanted, t.hotkeyEnabled
	t.hotkeyMu.Unlock()
	if !enabled {
		return 0
	}
	code := registerHotkey(t, spec)
	if code == 0 {
		liveHotkey, t.hotkeyOn = spec, true
		return 0
	}
	// Refused. The UI keeps the old preference stored, so the old shortcut
	// has to keep working as well — trying a taken combination must not
	// silently cost the user the one they already had. It was ours a moment
	// ago, so restoring practically always succeeds; if not, say so.
	if prevOn {
		if restore := registerHotkey(t, prev); restore == 0 {
			liveHotkey, t.hotkeyOn = prev, true
		} else {
			log.Printf("restore hotkey after refusal: %v", windows.Errno(restore))
		}
	}
	return code
}

// RefreshTip re-applies the icon's hover text. The text is localized, so
// it has to follow a language change the way the popup menu already does.
// Safe to call from any goroutine.
func (t *Tray) RefreshTip() {
	if t == nil || t.hwnd == 0 {
		return
	}
	winutil.PostMessage(t.hwnd, msgRefreshTip, 0, 0)
}

const msgRefreshTip = winutil.WM_APP + 4

// applyTip updates the hover text of an icon already in the tray.
// Must run on the tray's OS thread.
func applyTip(t *Tray, text string) {
	if !t.added {
		return
	}
	nid := notifyIconDataW{
		cbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:   t.hwnd,
		uID:    1,
		uFlags: nifTip,
	}
	copyTip(&nid.szTip, text)
	_, _, _ = procShellNotifyIconW.Call(uintptr(nimModify), uintptr(unsafe.Pointer(&nid)))
}

// trayText is everything the shell renders on our behalf: the popup menu
// and the icon's hover text.
type trayText struct {
	open, settings, quit string
	// tipLoading covers the WebView2 cold start, where a click cannot open
	// anything yet; tipReady doubles as the only place the app ever tells
	// the user what the icon is for.
	tipLoading, tipReady string
}

// Tray strings by locale. Falls back to English.
var trayTexts = map[string]trayText{
	"en": {
		open: "Open CopyNote", settings: "Settings", quit: "Quit",
		tipLoading: "CopyNote — loading…", tipReady: "CopyNote — click to open",
	},
	"ru": {
		open: "Открыть CopyNote", settings: "Настройки", quit: "Выход",
		tipLoading: "CopyNote — загрузка…", tipReady: "CopyNote — нажмите, чтобы открыть",
	},
}

// text resolves the strings for the current UI locale.
func (t *Tray) text() trayText {
	locale := "en"
	if t != nil && t.GetLocale != nil {
		locale = t.GetLocale()
	}
	if s, ok := trayTexts[locale]; ok {
		return s
	}
	return trayTexts["en"]
}

// tip is the hover text matching the current loading state.
func (t *Tray) tip() string {
	if t.ready.Load() {
		return t.text().tipReady
	}
	return t.text().tipLoading
}

func showTrayPopup(t *Tray) {
	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	labels := t.text()

	items := []popupmenu.Item{
		{ID: menuIDOpen, Label: labels.open},
		{ID: menuIDSettings, Label: labels.settings},
		{ID: menuIDQuit, Label: labels.quit},
	}
	t.menu.Show(items, pt.x, pt.y, popupmenu.Options{}, func(id uint32, picked bool) {
		if !picked {
			return
		}
		switch id {
		case menuIDOpen:
			// Same cold-start rule as a left click: queue, never drop.
			if !t.ready.Load() {
				t.pending = pendingShow
				return
			}
			if t.OnShow != nil {
				t.OnShow()
			}
		case menuIDSettings:
			if !t.ready.Load() {
				t.pending = pendingSettings
				return
			}
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

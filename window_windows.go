package main

import (
	"copynote/internal/tray"
	"copynote/internal/winutil"
	"math"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Subclassing state for the webview window. The callback must outlive
// the window, so it lives in package scope. quitting flips to true
// just before w.Terminate() so a stray WM_CLOSE during shutdown is
// allowed through to the original WndProc.
//
// lastShownNS records the last time the window was shown (or first
// painted), used to suppress an immediate auto-hide if a focus race
// fires WM_ACTIVATEAPP=false within ~300 ms of the show.
//
// activationLossNS records the last time the visible window lost
// WM_ACTIVATEAPP foreground. This is used by a tray click to recover the
// state before Explorer temporarily activated the notification area.
//
// activationLossHideNS records the last time WM_ACTIVATEAPP hid the
// window. This is used by toggleVisibility to avoid a race where a
// left click on the tray icon triggers both:
//  1. WM_ACTIVATEAPP=0 on the main window (explorer.exe took
//     foreground to dispatch the click) → auto-hide fires, AND
//  2. the tray's WM_LBUTTONUP → OnToggle sees the window already
//     hidden and would re-show it, defeating the toggle.
//
// If OnToggle runs within the debounce window of an auto-hide, it
// treats the click as "the user wanted to hide" and keeps it hidden.
var (
	origWndProc          uintptr
	wndProcCB            uintptr
	quitting             atomic.Bool
	lastShownNS          atomic.Int64
	activationLossNS     atomic.Int64
	activationLossHideNS atomic.Int64
	windowGeneration     atomic.Uint64
	windowHidden         atomic.Bool // true = window is parked off-screen
	topmostEnabled       atomic.Bool // user preference for always-on-top
	// autoHideDisabled keeps the window on screen when another program takes
	// focus. Stored inverted so the zero value is the default behaviour.
	autoHideDisabled atomic.Bool
	// windowHiddenCallback is set by main once WebView2 exists. The callback
	// receives the hide generation so a fast reopen cannot reset the newly
	// visible window.
	windowHiddenCallback             func(uint64)
	windowPrepareCallback            func(uint64, bool)
	windowShownCallback              func()
	windowTransitionStartedCallback  func(uint64)
	windowTransitionFinishedCallback func(uint64)
)

// Preparation state is owned by the UI thread. A show requested during a
// slide-out waits for the parked page to acknowledge its final layout.
var windowPrepared = true
var windowParked = true
var showPending bool
var settingsPending bool
var preparationID uint64

func prepareParkedWindow() {
	preparationID++
	windowPrepareCallback(preparationID, settingsPending)
}

func completeWindowPreparation(hwnd uintptr, id uint64, height int) {
	if id != preparationID || !windowHidden.Load() || !windowParked || windowPrepared {
		return
	}
	contentHeightCSS = height
	applyWindowSize(hwnd, winutil.DpiForWindow(hwnd))
	windowPrepared = true
	if showPending {
		showAndFocus(hwnd)
	}
}

// hideGuardWindow is the minimum time after a show during which we
// will NOT auto-hide on focus loss.
const hideGuardWindow = 300 * time.Millisecond

// toggleDebounce is the window during which a toggle-click after an
// activation-loss hide is interpreted as "user wanted hidden" rather
// than "user wants to show again".
const toggleDebounce = 150 * time.Millisecond

// Window metrics below are in 96-DPI (CSS) pixels. The process is
// per-monitor DPI aware, so they are scaled with winutil.ScaleForDPI for
// the monitor the window is on.

// trayCornerMargin is the gap between the window and the screen /
// taskbar edges when anchored to the tray corner.
const trayCornerMargin = 8

// windowWidth is the fixed width of the CopyNote window.
const windowWidth = 420

// initialWindowHeight is used until the frontend reports its content.
const initialWindowHeight = 640

// minWindowHeight is the minimum window height (header + some padding).
const minWindowHeight = 80

// contentHeightCSS is the last content height reported by the frontend,
// in CSS pixels. The window size is derived from it and the DPI, so a DPI
// change can re-scale the window without a new report. UI thread only.
var contentHeightCSS = initialWindowHeight

// windowSizeForDPI returns the physical window size for dpi: the content
// height clamped to [minWindowHeight, workAreaHeight - 2*margin].
func windowSizeForDPI(dpi uint32, contentCSS int, workAreaHeight int32) (width, height int32) {
	width = winutil.ScaleForDPI(windowWidth, dpi)
	height = winutil.ScaleForDPI(int32(max(contentCSS, minWindowHeight)), dpi)
	if maxH := workAreaHeight - 2*winutil.ScaleForDPI(trayCornerMargin, dpi); height > maxH {
		height = maxH
	}
	return width, height
}

// applyWindowSize sizes the window for dpi and the current content height
// and reports whether the size changed.
func applyWindowSize(hwnd uintptr, dpi uint32) bool {
	wa, ok := winutil.GetWorkArea()
	wr, ok2 := winutil.GetWindowRect(hwnd)
	if !ok || !ok2 {
		return false
	}
	w, h := windowSizeForDPI(dpi, contentHeightCSS, wa.Bottom-wa.Top)
	if wr.Right-wr.Left == w && wr.Bottom-wr.Top == h {
		return false
	}
	winutil.SetWindowPos(hwnd, 0, 0, 0, w, h,
		winutil.SWP_NOMOVE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
	return true
}

// trayDPI is the DPI of the primary monitor, whose work area the window
// is anchored to.
func trayDPI() uint32 {
	return winutil.DpiForMonitor(winutil.PrimaryMonitor())
}

// resizeToContent adjusts the window height to fit contentHeight CSS
// pixels reported by the frontend, clamped to [minWindowHeight,
// workAreaHeight - 2*margin]. A visible window whose size changed is
// re-anchored to the bottom-right tray corner.
//
// The frontend animates the height and reports every frame of it, for about
// a quarter of a second after whatever changed the content, so these calls
// keep arriving in the middle of slides. Only a visible window that really
// changed size cancels one: a slide-in aimed for the old height would stop
// short of the corner. A hidden window is resized in place and its slide-out
// left running. Cancelled, it stopped on screen with windowHidden already
// set, and Escape, the ✕ and auto-hide all took it for hidden.
func resizeToContent(hwnd uintptr, contentHeight int) {
	if windowParked && !windowPrepared {
		return // only the preparation acknowledgement supplies the final size
	}
	// The frontend measured the content at WebView2's current scale,
	// which follows the monitor the window is on.
	contentHeightCSS = contentHeight
	if !applyWindowSize(hwnd, winutil.DpiForWindow(hwnd)) || windowHidden.Load() {
		// Nothing moved, or the window is parked or sliding away — and a
		// parked window must not jump into view.
		return
	}
	cancelAnim.Store(true)
	anchorToTrayCorner(hwnd)
}

// installSubclass installs a subclass WndProc on hwnd that intercepts
// a small set of messages and forwards everything else to the
// webview's original WndProc. Handled messages:
//
//   - WM_CLOSE        → hide instead of destroying (real quit goes
//     through the quitting flag so shutdown still works).
//   - WM_ACTIVATEAPP  → auto-hide when another process takes focus,
//     respecting the 300 ms startup guard. Fires only on cross-
//     process focus changes, so our own tray popup window (same
//     process) does NOT trigger an auto-hide when shown.
//   - WM_SETTINGCHANGE("ImmersiveColorSet") → the system theme was
//     toggled, ask the tray to reload its icon variant.
//   - WM_DPICHANGED   → the window's monitor DPI changed: re-scale it
//     from the CSS content height and keep a visible window anchored.
func installSubclass(hwnd uintptr, tr *tray.Tray) {
	wndProcCB = syscall.NewCallback(func(h, msg, wParam, lParam uintptr) uintptr {
		if quitting.Load() {
			return winutil.CallWindowProc(origWndProc, h, msg, wParam, lParam)
		}
		switch msg {
		case winutil.WM_NCCALCSIZE:
			// Return 0 so Windows treats the entire window as client
			// area — no title bar strip, no non-client frame at all.
			if wParam != 0 {
				return 0
			}
		case winutil.WM_CLOSE:
			moveOffScreen(h)
			return 0
		case winutil.WM_ACTIVATEAPP:
			// The user can turn this off to keep the window up while working
			// in another program; it is then closed only on purpose (the ✕,
			// Escape, or the tray icon).
			if wParam == 0 {
				elapsed := time.Now().UnixNano() - lastShownNS.Load()
				if elapsed > int64(hideGuardWindow) && !windowHidden.Load() {
					now := time.Now().UnixNano()
					activationLossNS.Store(now)
					if !autoHideDisabled.Load() {
						activationLossHideNS.Store(now)
						moveOffScreen(h)
					}
				}
			}
		case winutil.WM_SETTINGCHANGE:
			if winutil.StringFromLPCWSTR(lParam) == "ImmersiveColorSet" && tr != nil {
				tr.ReloadIcon()
			}
		case winutil.WM_DPICHANGED:
			// The suggested rect in lParam is ignored: the size follows
			// from the content height, the position from the tray corner.
			// A window already sized for this DPI (see showAndFocus) keeps
			// its slide-in; a parked or hiding one is resized in place.
			if applyWindowSize(h, uint32(wParam&0xFFFF)) && !windowHidden.Load() {
				cancelAnim.Store(true)
				anchorToTrayCorner(h)
			}
			return 0
		}
		return winutil.CallWindowProc(origWndProc, h, msg, wParam, lParam)
	})
	origWndProc = winutil.SetWindowLongPtr(hwnd, winutil.GWLP_WNDPROC, wndProcCB)
}

// showAndFocus restores the window if minimized, makes it visible,
// and brings it to the foreground. Called via w.Dispatch from the
// tray thread. Updates the show-guard timestamp so auto-hide doesn't
// fire immediately on the activation race.
//
// The window is re-anchored to the tray corner on every show, so
// secondary monitor changes or taskbar resizes between runs don't
// leave it stranded off-screen.
// animMu serializes show/hide animations so they don't overlap.
// cancelAnim aborts a running animation (set by resizeToContent).
var (
	animMu     sync.Mutex
	cancelAnim atomic.Bool
)

func showAndFocus(hwnd uintptr) {
	if windowHidden.Load() && !windowPrepared {
		showPending = true
		return
	}
	showPending = false
	settingsPending = false
	windowParked = false
	if windowShownCallback != nil {
		defer windowShownCallback()
	}
	lastShownNS.Store(time.Now().UnixNano())
	generation := windowGeneration.Add(1)
	windowHidden.Store(false)

	// A parked window keeps the DPI it last had on screen; size it for the
	// tray monitor first so the slide-in below targets the final size.
	dpi := trayDPI()
	applyWindowSize(hwnd, dpi)

	// Compute the target (tray corner) position.
	wa, ok := winutil.GetWorkArea()
	if !ok {
		anchorToTrayCorner(hwnd)
		winutil.SetForegroundWindow(hwnd)
		return
	}
	wr, ok := winutil.GetWindowRect(hwnd)
	if !ok {
		anchorToTrayCorner(hwnd)
		winutil.SetForegroundWindow(hwnd)
		return
	}
	width := wr.Right - wr.Left
	height := wr.Bottom - wr.Top

	borderRight, borderBottom := dwmInvisibleBorder(hwnd, wr)
	margin := winutil.ScaleForDPI(trayCornerMargin, dpi)

	targetX := wa.Right - width - margin + borderRight
	targetY := wa.Bottom - height - margin + borderBottom
	startY := wa.Bottom // start just below the screen
	if windowTransitionStartedCallback != nil {
		windowTransitionStartedCallback(generation)
	}

	// Place at starting position. Use TOPMOST if the user has it
	// enabled (default true) — draws above the overflow tray popup.
	zOrder := winutil.HWND_NOTOPMOST
	if topmostEnabled.Load() {
		zOrder = winutil.HWND_TOPMOST
	}
	winutil.SetWindowPos(hwnd, zOrder, targetX, startY, 0, 0,
		winutil.SWP_NOSIZE|winutil.SWP_NOACTIVATE)
	winutil.SetForegroundWindow(hwnd)

	// Animate slide-up.
	go animateY(hwnd, targetX, startY, targetY, 200*time.Millisecond, easeOutCubic, generation)
}

// offScreenX/Y is where we park the window when "hidden". Kept as
// named constants (not magic numbers) for clarity. The values are
// far enough off any realistic multi-monitor arrangement. A window
// that touches no monitor keeps the DPI it last had on screen, so
// parking never triggers WM_DPICHANGED.
const (
	offScreenX = -30000
	offScreenY = -30000
)

// moveOffScreen hides the window by sliding it down below the screen
// edge, then parking it at offScreenX/Y. Unlike SW_HIDE this keeps
// WS_VISIBLE set so WebView2's renderer is never throttled.
func moveOffScreen(hwnd uintptr) {
	// A context menu must not outlive the window it belongs to.
	entryMenu.Close()
	if windowHidden.Load() {
		return // already hidden
	}
	generation := windowGeneration.Add(1)
	windowHidden.Store(true)
	windowPrepared = false
	windowParked = false
	showPending = false
	settingsPending = false
	wa, ok := winutil.GetWorkArea()
	wr, ok2 := winutil.GetWindowRect(hwnd)
	if !ok || !ok2 {
		parkOffScreen(hwnd)
		notifyWindowHidden(generation)
		return
	}
	if windowTransitionStartedCallback != nil {
		windowTransitionStartedCallback(generation)
	}
	endY := wa.Bottom // below screen
	go animateY(hwnd, wr.Left, wr.Top, endY, 150*time.Millisecond, easeInCubic, generation)
}

// parkOffScreen moves the window to the off-screen parking position
// instantly (no animation).
func parkOffScreen(hwnd uintptr) {
	windowHidden.Store(true)
	winutil.SetWindowPos(hwnd, 0, offScreenX, offScreenY, 0, 0,
		winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
}

// animateY slides the window from fromY to toY over duration.
// Uses SetWindowPos from a background goroutine (safe for top-level
// windows — Windows marshals the call internally).
//
// A slide whose direction no longer matches windowHidden gives way at once
// to the show or hide that flipped it — usually the opposite slide, queued
// behind this one on animMu. A slide-out that ran on to the end parked the
// window right after the user had asked for it back, and marked it hidden
// while the slide-in put it on screen.
func animateY(hwnd uintptr, x, fromY, toY int32, duration time.Duration, ease func(float64) float64, generation uint64) {
	animMu.Lock()
	defer animMu.Unlock()
	cancelAnim.Store(false)

	hiding := toY > fromY
	const steps = 20
	stepDur := duration / steps
	for i := 1; i <= steps; i++ {
		if cancelAnim.Load() {
			finishWindowTransition(generation, hiding)
			return // aborted by resizeToContent or another caller
		}
		if windowHidden.Load() != hiding || windowGeneration.Load() != generation {
			return
		}
		t := ease(float64(i) / float64(steps))
		y := fromY + int32(float64(toY-fromY)*t)
		winutil.SetWindowPos(hwnd, 0, x, y, 0, 0,
			winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
		time.Sleep(stepDur)
	}
	if hiding && windowHidden.Load() && windowGeneration.Load() == generation {
		// Park the window exactly. Not parkOffScreen: moveOffScreen has
		// already marked it hidden, and marking it again here could
		// overwrite a show that arrived during the last step.
		winutil.SetWindowPos(hwnd, 0, offScreenX, offScreenY, 0, 0,
			winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
		notifyWindowHidden(generation)
	}
	finishWindowTransition(generation, hiding)
}

// finishWindowTransition unlocks pointer input only for the animation that
// still owns the window. A stale animation must not unlock a newer one.
func finishWindowTransition(generation uint64, hiding bool) {
	if windowGeneration.Load() != generation || windowHidden.Load() != hiding {
		return
	}
	if windowTransitionFinishedCallback != nil {
		windowTransitionFinishedCallback(generation)
	}
}

// notifyWindowHidden schedules the frontend reset only after the window is
// fully parked. The generation check makes the notification harmless if a
// show request raced with the end of the hide animation.
func notifyWindowHidden(generation uint64) {
	if !windowHidden.Load() || windowGeneration.Load() != generation {
		return
	}
	if windowHiddenCallback != nil {
		windowHiddenCallback(generation)
	}
}

func easeOutCubic(t float64) float64 {
	return 1 - math.Pow(1-t, 3)
}

func easeInCubic(t float64) float64 {
	return math.Pow(t, 3)
}

// anchorToTrayCorner moves the window to the bottom-right corner of
// the primary monitor's work area, with a small inset — matching
// Windows' own tray flyouts (Calendar, Volume, Action Center).
//
// The work area excludes the taskbar, so this correctly handles
// taskbars docked at the top/left/right as well. Multi-monitor
// placement targets the primary monitor since that's where the
// tray icon lives in the vast majority of setups.
//
// GetWindowRect on Win10+ includes the invisible resize border
// (~7–9 px per DPI), which would push the visually rendered edge
// away from the screen by that extra amount. We compensate by
// querying DWMWA_EXTENDED_FRAME_BOUNDS for the truly visible rect
// and offsetting the target position accordingly.
func anchorToTrayCorner(hwnd uintptr) {
	wa, ok := winutil.GetWorkArea()
	if !ok {
		return
	}
	wr, ok := winutil.GetWindowRect(hwnd)
	if !ok {
		return
	}
	width := wr.Right - wr.Left
	height := wr.Bottom - wr.Top

	// Compensate for the invisible DWM resize border, if available.
	// If DWM is unreachable (virtualized env, etc.), fall back to
	// raw GetWindowRect bounds.
	borderRight, borderBottom := dwmInvisibleBorder(hwnd, wr)
	margin := winutil.ScaleForDPI(trayCornerMargin, trayDPI())

	x := wa.Right - width - margin + borderRight
	y := wa.Bottom - height - margin + borderBottom
	winutil.SetWindowPos(
		hwnd,
		0,
		x, y, 0, 0,
		winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE,
	)
}

// dwmInvisibleBorder returns the right/bottom offsets of the window's
// invisible DWM resize border (~7-9 px per DPI on Win10+). Computed as
// GetWindowRect minus DWMWA_EXTENDED_FRAME_BOUNDS.
//
// On some machines DWM returns garbage values when the window is parked
// far off-screen (e.g. at -30000, -30000), which would corrupt window
// placement math. The returned offsets are clamped to [0, 32] px.
func dwmInvisibleBorder(hwnd uintptr, wr winutil.Rect) (right, bottom int32) {
	const maxInvisibleBorder = 32
	efb, ok := winutil.GetExtendedFrameBounds(hwnd)
	if !ok {
		return 0, 0
	}
	br := wr.Right - efb.Right
	bb := wr.Bottom - efb.Bottom
	if br < 0 || br > maxInvisibleBorder {
		br = 0
	}
	if bb < 0 || bb > maxInvisibleBorder {
		bb = 0
	}
	return br, bb
}

// toggleVisibility hides the window if it's currently visible and focused,
// otherwise shows and focuses it. A visible but inactive window is not a
// toggle-to-hide target: clicking the tray icon is the user's way to return
// to that window.
//
// Special case: if the window was hidden by WM_ACTIVATEAPP within the
// last few hundred milliseconds, treat the toggle as "stay hidden" —
// the click on the tray icon is what caused that activation loss in
// the first place, and the user's intent is clearly to hide, not to
// immediately re-open.
// toggleVisibility reports whether it requested a show, which may wait for
// preparation. showAndFocus sends the frontend notification on actual show.
type toggleAction uint8

const (
	toggleShow toggleAction = iota
	toggleHide
	toggleFocus
)

func toggleActionForState(hidden bool, foreground, hwnd uintptr) toggleAction {
	if hidden {
		return toggleShow
	}
	if foreground != hwnd {
		return toggleFocus
	}
	return toggleHide
}

func toggleVisibility(hwnd uintptr) (shown bool) {
	return toggleVisibilityWithForeground(hwnd, winutil.GetForegroundWindow())
}

// toggleVisibilityWithFocus uses the focus state captured when a physical
// tray click started. This avoids treating the shell's temporary activation
// of the notification area as proof that the window was inactive.
func toggleVisibilityWithFocus(hwnd uintptr, focused bool) (shown bool) {
	if !focused && !windowHidden.Load() {
		focused = activationLossWasRecent()
	}
	foreground := uintptr(0)
	if focused {
		foreground = hwnd
	}
	return toggleVisibilityWithForeground(hwnd, foreground)
}

// toggleVisibilityFromTray is the fallback for a callback message that has
// no preceding mouse-down (for example, a posted test message). A recent
// activation loss still lets it distinguish an active window whose focus was
// taken by Explorer from one that was already inactive.
func toggleVisibilityFromTray(hwnd uintptr) (shown bool) {
	return toggleVisibilityWithFocus(hwnd, winutil.GetForegroundWindow() == hwnd)
}

func activationLossWasRecent() bool {
	t := activationLossNS.Load()
	if t == 0 {
		return false
	}
	activationLossNS.Store(0)
	return time.Since(time.Unix(0, t)) < toggleDebounce
}

func toggleVisibilityWithForeground(hwnd, foreground uintptr) (shown bool) {
	if t := activationLossHideNS.Load(); t != 0 {
		if time.Since(time.Unix(0, t)) < toggleDebounce {
			activationLossHideNS.Store(0)
			return false
		}
		activationLossHideNS.Store(0)
	}

	switch toggleActionForState(windowHidden.Load(), foreground, hwnd) {
	case toggleFocus:
		// The window remains on screen when auto-hide is disabled. Bring it
		// back to the foreground without resetting the current UI state.
		winutil.SetForegroundWindow(hwnd)
		return false
	case toggleHide:
		moveOffScreen(hwnd)
		return false
	}

	showAndFocus(hwnd)
	return true
}

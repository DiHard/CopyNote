package main

import (
	"fmt"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"copynote/internal/tray"
	"copynote/internal/winutil"
)

// Subclassing state for the webview window. The callback must outlive
// the window, so it lives in package scope. quitting flips to true
// just before w.Terminate() so a stray WM_CLOSE during shutdown is
// allowed through to the original WndProc.
//
// lastShownNS records the last time the window was shown (or first
// painted), used to suppress an immediate auto-hide if a focus race
// fires WM_ACTIVATEAPP=false within ~300 ms of the show.
var (
	origWndProc    uintptr
	wndProcCB      uintptr
	quitting       atomic.Bool
	lastShownNS    atomic.Int64
	topmostEnabled atomic.Bool // user preference for always-on-top
	// autoHideDisabled keeps the window on screen when another program takes
	// focus. Stored inverted so the zero value is the default behaviour.
	autoHideDisabled atomic.Bool
)

// windowPhase is where the window is in its show/hide cycle. The window is
// never destroyed and never SW_HIDEn — "hidden" means parked off-screen — so
// the cycle is all the state there is:
//
//	parked ──show──▶ visible ──hide──▶ hiding ──slid out──▶ preparing ──page answered──▶ parked
//
// While the window is parked the page resets itself and measures the height
// it will need (preparing), so the next show slides in the finished view
// rather than the one the user left. A show asked for before the cycle is
// back at parked waits in pendingRequest.
//
// Written on the UI thread only; atomic so that other goroutines and the
// tests may read it.
type windowPhase uint32

const (
	// phaseParked: off-screen and ready, a show starts at once. The zero
	// value, which is also how the window starts out.
	phaseParked windowPhase = iota
	// phaseVisible: on screen, or sliding in.
	phaseVisible
	// phaseHiding: sliding out.
	phaseHiding
	// phasePreparing: off-screen, waiting for the page to reset itself and
	// report its height.
	phasePreparing
)

var currentPhase atomic.Uint32

func phase() windowPhase     { return windowPhase(currentPhase.Load()) }
func setPhase(p windowPhase) { currentPhase.Store(uint32(p)) }

// windowIsHidden reports whether the window is anywhere but on screen.
func windowIsHidden() bool { return phase() != phaseVisible }

// showRequest is what a show that has to wait is waiting to do.
type showRequest uint8

const (
	requestNone showRequest = iota
	requestShow
	// requestSettings shows the Settings view, which a hidden window has
	// the page prepare off-screen first.
	requestSettings
)

// The rest of the show/hide state. UI thread only.
var (
	// pendingRequest is a show that arrived while the window was hiding or
	// preparing. It is carried out when the cycle reaches phaseParked.
	pendingRequest showRequest
	// preparationID numbers the requests sent to the page, so the answer to
	// one a newer request replaced is recognised.
	preparationID uint64
	// preparationTimer lets the window go when the page does not answer.
	preparationTimer *time.Timer
)

// windowGeneration counts shows and hides. A slide carries the number it was
// started with and gives way as soon as that is no longer the current one;
// the page uses the same number to match the end of a transition to its
// start.
var windowGeneration atomic.Uint64

// preparationTimeout bounds how long a show waits for the page's answer,
// which normally takes a few frames. Without the bound one lost answer — a
// script error in the page, a reload, frames that stopped coming — left the
// window impossible to open until the program was restarted.
const preparationTimeout = 500 * time.Millisecond

// pageHost is what the window code needs from WebView2: run a function on the
// UI thread, and evaluate a script in the page. main attaches the WebView;
// the tests attach a recorder.
type pageHost interface {
	Dispatch(f func())
	Eval(js string)
}

var host pageHost

// Calls into the page. Each does nothing until the page has installed its
// handler.
const (
	// The window has started to come on screen. The argument is the view it
	// was parked with, "main" or "settings", or null when it was on screen
	// already. The page puts the caret in the search box and, for a window
	// that was parked, drops whatever was typed into it in the meantime.
	scriptShown = `window.__onShow && window.__onShow(%s)`
	// The window is parked: reset the view (to Settings when true), measure,
	// and answer through the windowPrepared(id, height) binding.
	scriptPrepare = `window.__onHide && window.__onHide(%d, %t)`
	// A slide started (true) or ended (false). The page keeps pointer input
	// away while the window moves under the cursor; the generation lets it
	// ignore the end of a slide a newer one replaced.
	scriptTransition   = `window.__onWindowTransition && window.__onWindowTransition(%t, %d)`
	scriptOpenSettings = `window.__openSettings && window.__openSettings()`
)

func evalPage(format string, args ...any) {
	if host != nil {
		host.Eval(fmt.Sprintf(format, args...))
	}
}

// onUIThread runs f on the UI thread, unless the program is on its way out.
func onUIThread(f func()) {
	if host == nil || quitting.Load() {
		return
	}
	host.Dispatch(func() {
		if !quitting.Load() {
			f()
		}
	})
}

// hideGuardWindow is the minimum time after a show during which we
// will NOT auto-hide on focus loss.
const hideGuardWindow = 300 * time.Millisecond

// toggleDebounce is how soon after auto-hide put the window away a click on
// the tray icon is still the click that caused it: reaching for the icon
// took the foreground, the window hid, and the click itself arrives a moment
// later. Showing the window again would undo what the user meant.
const toggleDebounce = 150 * time.Millisecond

// trayReachWindow is how long after the foreground went from the window to
// the shell a click on the tray icon still counts as made from the active
// window. It has to cover opening the overflow flyout and finding the icon
// in it, which is where Windows 11 puts a new icon; a click held down for a
// moment needs it too.
const trayReachWindow = 5 * time.Second

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

// applyContentHeight records the height the page needs, in CSS pixels, sizes
// the window for it and reports whether the size changed. The page measured
// at WebView2's current scale, which follows the monitor the window is on.
func applyContentHeight(hwnd uintptr, contentHeight int) bool {
	contentHeightCSS = contentHeight
	return applyWindowSize(hwnd, winutil.DpiForWindow(hwnd))
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
// changed size overtakes one: a slide-in aimed for the old height would stop
// short of the corner. A hidden window is resized in place and its slide-out
// left running. Cancelled, it stopped on screen while already counted as
// hidden, and Escape, the ✕ and auto-hide all took it for hidden.
func resizeToContent(hwnd uintptr, contentHeight int) {
	if phase() == phasePreparing {
		return // the page is mid-reset; its answer carries the height that counts
	}
	if !applyContentHeight(hwnd, contentHeight) || phase() != phaseVisible {
		// Nothing moved, or the window is parked or sliding away — and a
		// parked window must not jump into view.
		return
	}
	cancelledSlide.Store(windowGeneration.Load())
	anchorToTrayCorner(hwnd)
}

// deactivation is the last time another thread took the foreground from the
// visible window, as WM_ACTIVATEAPP reported it. A click on the tray icon
// reads it to tell what the window was doing before the click: the shell
// takes the foreground to deliver the click, so by the time the click
// arrives the window never has it. UI thread only.
type deactivation struct {
	at        time.Time
	toShell   bool // the taskbar or its overflow flyout took the foreground
	hidWindow bool // auto-hide put the window away at that moment
}

var lastDeactivation deactivation

// installSubclass installs a subclass WndProc on hwnd that intercepts
// a small set of messages and forwards everything else to the
// webview's original WndProc. Handled messages:
//
//   - WM_CLOSE        → hide instead of destroying (real quit goes
//     through the quitting flag so shutdown still works).
//   - WM_ACTIVATEAPP  → auto-hide when another thread takes the foreground,
//     respecting the 300 ms startup guard. The entry context menu and the
//     file dialogs run on the UI thread, so they do NOT trigger it; the
//     tray's own menu, on the tray thread, does.
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
			if wParam != 0 {
				// Active again: what happened before no longer explains a click.
				lastDeactivation = deactivation{}
				break
			}
			elapsed := time.Now().UnixNano() - lastShownNS.Load()
			if elapsed <= int64(hideGuardWindow) || phase() != phaseVisible {
				break
			}
			// lParam is the thread whose window is being activated.
			shell := winutil.ShellProcessID()
			hide := !autoHideDisabled.Load()
			lastDeactivation = deactivation{
				at:        time.Now(),
				toShell:   shell != 0 && winutil.ThreadProcessID(uint32(lParam)) == shell,
				hidWindow: hide,
			}
			// The user can turn auto-hide off to keep the window up while
			// working in another program; it is then closed only on purpose
			// (the ✕, Escape, the hotkey or the tray icon).
			if hide {
				moveOffScreen(h)
			}
		case winutil.WM_SETTINGCHANGE:
			if winutil.StringFromLPCWSTR(lParam) == "ImmersiveColorSet" && tr != nil {
				tr.ReloadIcon()
			}
		case winutil.WM_DPICHANGED:
			// The suggested rect in lParam is ignored: the size follows
			// from the content height, the position from the tray corner.
			// A window already sized for this DPI (see slideIn) keeps its
			// slide-in; a parked or hiding one is resized in place.
			if applyWindowSize(h, uint32(wParam&0xFFFF)) && phase() == phaseVisible {
				cancelledSlide.Store(windowGeneration.Load())
				anchorToTrayCorner(h)
			}
			return 0
		}
		return winutil.CallWindowProc(origWndProc, h, msg, wParam, lParam)
	})
	origWndProc = winutil.SetWindowLongPtr(hwnd, winutil.GWLP_WNDPROC, wndProcCB)
}

// showAndFocus brings the window on screen and to the foreground. It slides
// in from below the tray corner and is re-anchored on every show, so monitor
// or taskbar changes between shows do not leave it stranded. A window that
// has not finished hiding shows as soon as it has. UI thread.
func showAndFocus(hwnd uintptr) { requestWindow(hwnd, requestShow) }

// showSettings does the same with the Settings view. A hidden window has the
// page prepare that view off-screen first, so it does not slide in showing
// the list and switch afterwards. UI thread.
func showSettings(hwnd uintptr) { requestWindow(hwnd, requestSettings) }

func requestWindow(hwnd uintptr, want showRequest) {
	switch phase() {
	case phaseVisible:
		if want == requestSettings {
			winutil.SetForegroundWindow(hwnd)
			evalPage(scriptOpenSettings)
			return
		}
		slideIn(hwnd)
	case phaseParked:
		if want == requestSettings {
			pendingRequest = requestSettings
			beginPreparation(hwnd)
			return
		}
		slideIn(hwnd)
	case phaseHiding:
		pendingRequest = max(pendingRequest, want)
	case phasePreparing:
		if want == requestSettings && pendingRequest != requestSettings {
			// The page is getting the list ready; ask again, for Settings.
			pendingRequest = requestSettings
			beginPreparation(hwnd)
			return
		}
		pendingRequest = max(pendingRequest, want)
	}
}

// slideIn puts the window in the tray corner and tells the page. It carries
// out pendingRequest, if there is one. UI thread, from phaseVisible or
// phaseParked only.
func slideIn(hwnd uintptr) {
	view := "null" // already on screen: what the user is looking at stays
	if phase() == phaseParked {
		view = `"main"`
		if pendingRequest == requestSettings {
			view = `"settings"`
		}
	}
	pendingRequest = requestNone
	lastShownNS.Store(time.Now().UnixNano())
	generation := windowGeneration.Add(1)
	setPhase(phaseVisible)
	// Last, so the page hears about the slide before it is told the window is up.
	defer evalPage(scriptShown, view)

	// A parked window keeps the DPI it last had on screen; size it for the
	// tray monitor first so the slide-in below targets the final size.
	dpi := trayDPI()
	applyWindowSize(hwnd, dpi)

	// Compute the target (tray corner) position.
	targetX, targetY, startY, ok := trayCornerPosition(hwnd, dpi)
	if !ok {
		anchorToTrayCorner(hwnd)
		winutil.SetForegroundWindow(hwnd)
		// Nothing slides, so nothing else will report an end; a slide this
		// show replaced must not leave the page shielded from the pointer.
		evalPage(scriptTransition, false, generation)
		return
	}
	evalPage(scriptTransition, true, generation)

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
	go animateY(hwnd, targetX, startY, targetY, 200*time.Millisecond, easeOutCubic, generation, false)
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
// WS_VISIBLE set so WebView2's renderer is never throttled. UI thread.
func moveOffScreen(hwnd uintptr) {
	// A context menu must not outlive the window it belongs to.
	entryMenu.Close()
	if phase() != phaseVisible {
		return // already hidden, or on its way
	}
	generation := windowGeneration.Add(1)
	setPhase(phaseHiding)
	pendingRequest = requestNone
	wa, ok := winutil.GetWorkArea()
	wr, ok2 := winutil.GetWindowRect(hwnd)
	if !ok || !ok2 {
		parkOffScreen(hwnd)
		evalPage(scriptTransition, false, generation)
		hideFinished(hwnd, generation)
		return
	}
	evalPage(scriptTransition, true, generation)
	go animateY(hwnd, wr.Left, wr.Top, wa.Bottom, 150*time.Millisecond, easeInCubic, generation, true)
}

// parkOffScreen moves the window to the off-screen parking position
// instantly (no animation). It changes the position only: the phase is the
// caller's business.
func parkOffScreen(hwnd uintptr) {
	winutil.SetWindowPos(hwnd, 0, offScreenX, offScreenY, 0, 0,
		winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
}

// animMu serializes show/hide animations so they don't overlap.
var animMu sync.Mutex

// cancelledSlide is the generation of a slide-in that resizeToContent has
// overtaken by putting the window in its corner itself. A generation rather
// than a flag: a flag had to be cleared when a slide started, and a resize
// that landed between slideIn starting the slide and its goroutine getting
// going was cleared with it — the slide then ran to the position worked out
// for the old height.
var cancelledSlide atomic.Uint64

// animateY slides the window from fromY to toY over duration.
// Uses SetWindowPos from a background goroutine (safe for top-level
// windows — Windows marshals the call internally).
//
// A slide gives way at once to the show or hide that replaced it — usually
// the opposite slide, queued behind this one on animMu.
//
// hiding is passed in rather than read off the coordinates. A hide asked for
// while the window still stood on the slide-in's starting line has fromY
// equal to toY; it was taken for a show and never parked the window, which
// then could not be opened again.
func animateY(hwnd uintptr, x, fromY, toY int32, duration time.Duration, ease func(float64) float64, generation uint64, hiding bool) {
	animMu.Lock()
	defer animMu.Unlock()

	const steps = 20
	stepDur := duration / steps
	for i := 1; i <= steps; i++ {
		if windowGeneration.Load() != generation {
			return // replaced; the newer slide reports its own end
		}
		if !hiding && cancelledSlide.Load() == generation {
			break // resizeToContent has put the window where it belongs
		}
		t := ease(float64(i) / float64(steps))
		y := fromY + int32(float64(toY-fromY)*t)
		winutil.SetWindowPos(hwnd, 0, x, y, 0, 0,
			winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
		time.Sleep(stepDur)
	}
	if windowGeneration.Load() != generation {
		return
	}
	if hiding {
		parkOffScreen(hwnd)
	}
	onUIThread(func() {
		if windowGeneration.Load() != generation {
			return
		}
		evalPage(scriptTransition, false, generation)
		if hiding {
			hideFinished(hwnd, generation)
		}
	})
}

// hideFinished runs on the UI thread once a hide has parked the window.
func hideFinished(hwnd uintptr, generation uint64) {
	if phase() != phaseHiding || windowGeneration.Load() != generation {
		return
	}
	beginPreparation(hwnd)
}

// beginPreparation asks the parked page to reset and measure itself, and
// makes sure the answer is not waited for indefinitely. UI thread.
func beginPreparation(hwnd uintptr) {
	setPhase(phasePreparing)
	preparationID++
	id := preparationID
	stopPreparationTimer()
	preparationTimer = time.AfterFunc(preparationTimeout, func() {
		onUIThread(func() { expirePreparation(hwnd, id) })
	})
	evalPage(scriptPrepare, id, pendingRequest == requestSettings)
}

func stopPreparationTimer() {
	if preparationTimer != nil {
		preparationTimer.Stop()
		preparationTimer = nil
	}
}

// completeWindowPreparation is the page's answer to scriptPrepare: it has
// reset itself and needs height CSS pixels. UI thread.
func completeWindowPreparation(hwnd uintptr, id uint64, height int) {
	if id != preparationID {
		return // the answer to a request a newer one replaced
	}
	switch phase() {
	case phasePreparing:
		applyContentHeight(hwnd, height)
		finishPreparation(hwnd)
	case phaseParked:
		// The timeout has already let the window go, but it is still
		// parked and the height is still the right one.
		applyContentHeight(hwnd, height)
	}
}

// expirePreparation gives up waiting for the page. The window keeps the size
// it had; the page corrects it through resizeWindow once it is running again.
func expirePreparation(hwnd uintptr, id uint64) {
	if id != preparationID || phase() != phasePreparing {
		return
	}
	log.Printf("the page did not confirm preparation %d within %v; the window opens without it", id, preparationTimeout)
	finishPreparation(hwnd)
}

func finishPreparation(hwnd uintptr) {
	stopPreparationTimer()
	setPhase(phaseParked)
	if pendingRequest != requestNone {
		slideIn(hwnd)
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
	x, y, _, ok := trayCornerPosition(hwnd, trayDPI())
	if !ok {
		return
	}
	winutil.SetWindowPos(
		hwnd,
		0,
		x, y, 0, 0,
		winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE,
	)
}

// trayCornerPosition is shared by immediate positioning and the slide-in target.
// bottom is the off-screen animation start just below the work area.
func trayCornerPosition(hwnd uintptr, dpi uint32) (x, y, bottom int32, ok bool) {
	wa, ok := winutil.GetWorkArea()
	if !ok {
		return 0, 0, 0, false
	}
	wr, ok := winutil.GetWindowRect(hwnd)
	if !ok {
		return 0, 0, 0, false
	}
	width := wr.Right - wr.Left
	height := wr.Bottom - wr.Top

	// Compensate for the invisible DWM resize border, if available.
	// If DWM is unreachable (virtualized env, etc.), fall back to
	// raw GetWindowRect bounds.
	borderRight, borderBottom := dwmInvisibleBorder(hwnd, wr)
	margin := winutil.ScaleForDPI(trayCornerMargin, dpi)

	x = wa.Right - width - margin + borderRight
	y = wa.Bottom - height - margin + borderBottom
	return x, y, wa.Bottom, true
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

// toggleAction is what a click on the tray icon or the global hotkey does.
type toggleAction uint8

const (
	toggleNone toggleAction = iota // leave the window as it is
	toggleShow
	toggleHide
	// toggleFocus: on screen but not the active window — bring it forward.
	toggleFocus
)

// toggleState is everything that decision depends on.
type toggleState struct {
	hidden         bool // the window is not on screen
	ownsForeground bool // the window, or a menu or dialog of its own, has the foreground
	fromTray       bool // a click on the tray icon, as opposed to the hotkey
	// The latest deactivation of the visible window. lostAgo is negative
	// when there is none to speak of.
	lostAgo      time.Duration
	lostToShell  bool
	lostByHiding bool
}

// decideToggle hides the window when it is on screen and was the active one,
// shows it when it is hidden, and brings it forward when it is on screen but
// something else is active. With auto-hide off the window stays up while the
// user works in another program, and the icon or the hotkey is then how they
// come back to it, not how they close it.
func decideToggle(s toggleState) toggleAction {
	lostWithin := func(d time.Duration) bool { return s.lostAgo >= 0 && s.lostAgo < d }
	if s.hidden {
		// Reaching for the icon took the foreground, auto-hide put the
		// window away, and now the click itself arrives: the user meant to
		// close the window, and opening it again would undo that.
		if s.fromTray && s.lostByHiding && lostWithin(toggleDebounce) {
			return toggleNone
		}
		return toggleShow
	}
	if s.ownsForeground {
		return toggleHide
	}
	// The shell has the foreground by now whatever the window was doing
	// before. It was the active one if the foreground went from it straight
	// to the shell a moment ago.
	if s.fromTray && s.lostToShell && lostWithin(trayReachWindow) {
		return toggleHide
	}
	return toggleFocus
}

// toggleVisibility is the tray icon's left click (fromTray) and the global
// hotkey. UI thread.
func toggleVisibility(hwnd uintptr, fromTray bool) {
	state := toggleState{
		hidden:         windowIsHidden(),
		ownsForeground: ownsForeground(hwnd),
		fromTray:       fromTray,
		lostAgo:        -1,
	}
	if last := lastDeactivation; !last.at.IsZero() {
		state.lostAgo = time.Since(last.at)
		state.lostToShell = last.toShell
		state.lostByHiding = last.hidWindow
	}
	lastDeactivation = deactivation{} // one deactivation explains one click
	switch decideToggle(state) {
	case toggleShow:
		showAndFocus(hwnd)
	case toggleHide:
		moveOffScreen(hwnd)
	case toggleFocus:
		// The window stays on screen when auto-hide is off. Bring it back
		// to the foreground without resetting what it shows.
		winutil.SetForegroundWindow(hwnd)
	}
}

// ownsForeground reports whether the foreground window belongs to the UI
// thread: the main window itself, the entry context menu, or a file dialog
// it owns. Any of them is the application being used. Comparing against the
// main window alone took an open context menu for another program, and the
// hotkey then only closed the menu instead of putting the window away.
func ownsForeground(hwnd uintptr) bool {
	foreground := winutil.GetForegroundWindow()
	if foreground == 0 {
		return false
	}
	if foreground == hwnd {
		return true
	}
	ours, _ := winutil.WindowThreadProcess(hwnd)
	theirs, _ := winutil.WindowThreadProcess(foreground)
	return ours != 0 && ours == theirs
}

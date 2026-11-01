package main

import (
	"log"
	"time"

	"copynote/internal/macui"
)

// The macOS window follows the same cycle and page contract as the Windows
// one (window_windows.go) without the parts Windows forced on it: a window
// put away is ordered out rather than parked off-screen, nothing slides yet,
// and the window is measured in points, which are CSS pixels.
//
//	parked ──show──▶ visible ──hide──▶ preparing ──page answered──▶ parked
//
// While preparing the page resets itself and reports its height, so the next
// show brings up a finished view. A show asked for meanwhile waits in
// pendingRequest. Main thread only.

type windowPhase uint8

const (
	phaseParked windowPhase = iota
	phaseVisible
	phasePreparing
)

// showRequest is what a show that has to wait is waiting to do.
type showRequest uint8

const (
	requestNone showRequest = iota
	requestShow
	// requestSettings has the page prepare the Settings view first.
	requestSettings
)

const (
	windowWidth         = 420
	initialWindowHeight = 640
)

var (
	phase            windowPhase
	pendingRequest   showRequest
	preparationID    uint64
	preparationTimer *time.Timer
	lastShown        time.Time
)

func showWindow()   { requestWindow(requestShow) }
func showSettings() { requestWindow(requestSettings) }

func requestWindow(want showRequest) {
	switch phase {
	case phaseVisible:
		macui.FocusWindow()
		if want == requestSettings {
			evalPage(scriptOpenSettings)
			return
		}
		evalPage(scriptShown, "null") // what the user is looking at stays
	case phaseParked:
		if want == requestSettings {
			pendingRequest = requestSettings
			beginPreparation()
			return
		}
		pendingRequest = want
		bringUp()
	case phasePreparing:
		if want == requestSettings && pendingRequest != requestSettings {
			// The page is getting the list ready; ask again, for Settings.
			pendingRequest = requestSettings
			beginPreparation()
			return
		}
		pendingRequest = max(pendingRequest, want)
	}
}

// bringUp shows a parked window with the view pendingRequest asks for.
func bringUp() {
	view := `"main"`
	if pendingRequest == requestSettings {
		view = `"settings"`
	}
	pendingRequest = requestNone
	phase = phaseVisible
	lastShown = time.Now()
	macui.ShowWindow()
	evalPage(scriptShown, view)
}

// hideWindow puts the window away and hands the keyboard back to the app the
// user came from, unless that app has already taken it (giveBackFocus false).
func hideWindow(giveBackFocus bool) {
	macui.CancelMenu() // a context menu must not outlive its window
	if phase != phaseVisible {
		return
	}
	pendingRequest = requestNone
	macui.HideWindow(giveBackFocus)
	beginPreparation()
}

// onResignActive is auto-hide: another app took the keyboard.
func onResignActive() {
	if phase != phaseVisible || autoHideDisabled.Load() || time.Since(lastShown) <= hideGuardWindow {
		return
	}
	hideWindow(false)
}

// toggleWindow is a click on the menu bar icon and the global hotkey: show a
// hidden window, hide one in use, and bring forward one left on screen while
// another app has the keyboard (auto-hide off). Clicking a menu bar icon does
// not take the keyboard from anyone, so unlike on Windows the click and the
// hotkey see the same thing.
func toggleWindow() {
	switch {
	case phase != phaseVisible:
		showWindow()
	case macui.WindowKey():
		hideWindow(true)
	default:
		macui.FocusWindow()
	}
}

func beginPreparation() {
	phase = phasePreparing
	preparationID++
	id := preparationID
	stopPreparationTimer()
	preparationTimer = time.AfterFunc(preparationTimeout, func() {
		onUIThread(func() { expirePreparation(id) })
	})
	evalPage(scriptPrepare, id, pendingRequest == requestSettings)
}

func stopPreparationTimer() {
	if preparationTimer != nil {
		preparationTimer.Stop()
		preparationTimer = nil
	}
}

// completeWindowPreparation is the page's answer to scriptPrepare.
func completeWindowPreparation(id uint64, height int) {
	if id != preparationID {
		return // the answer to a request a newer one replaced
	}
	switch phase {
	case phasePreparing:
		macui.SetContentHeight(height)
		finishPreparation()
	case phaseParked:
		// The timeout let the window go, but it is still parked and the
		// height is still the right one.
		macui.SetContentHeight(height)
	}
}

func expirePreparation(id uint64) {
	if id != preparationID || phase != phasePreparing {
		return
	}
	log.Printf("the page did not confirm preparation %d within %v; the window opens without it", id, preparationTimeout)
	finishPreparation()
}

func finishPreparation() {
	stopPreparationTimer()
	phase = phaseParked
	if pendingRequest != requestNone {
		bringUp()
	}
}

// resizeToContent applies the height the page reports while it animates.
// Ignored while preparing: the page's answer carries the height that counts.
func resizeToContent(height int) {
	if phase == phasePreparing {
		return
	}
	macui.SetContentHeight(height)
}

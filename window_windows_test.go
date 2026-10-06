package main

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// recordingHost stands in for WebView2: it keeps the scripts sent to the page
// and runs dispatched functions when the test asks for them, on the test's
// own goroutine — which plays the UI thread.
type recordingHost struct {
	scripts []string
	queue   chan func()
}

func (h *recordingHost) Eval(js string) { h.scripts = append(h.scripts, js) }

func (h *recordingHost) Dispatch(f func()) { h.queue <- f }

// runDispatched runs everything dispatched so far.
func (h *recordingHost) runDispatched() {
	for {
		select {
		case f := <-h.queue:
			f()
		default:
			return
		}
	}
}

// waitDispatched runs the next dispatched function, waiting for it to arrive.
func (h *recordingHost) waitDispatched(t *testing.T) {
	t.Helper()
	select {
	case f := <-h.queue:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was dispatched to the UI thread")
	}
}

// last returns the most recent script containing name, or "".
func (h *recordingHost) last(name string) string {
	for i := len(h.scripts) - 1; i >= 0; i-- {
		if strings.Contains(h.scripts[i], name+"(") {
			return h.scripts[i]
		}
	}
	return ""
}

// newWindowState starts every test from a parked, ready window with a
// recording host attached, and puts the package state back afterwards.
// The window handle used throughout is 0: the Win32 calls fail harmlessly,
// which takes showing and hiding down their no-animation paths.
func newWindowState(t *testing.T) *recordingHost {
	t.Helper()
	h := &recordingHost{queue: make(chan func(), 16)}
	oldHost, oldPhase, oldGeneration := host, phase(), windowGeneration.Load()
	oldPending, oldID, oldHeight := pendingRequest, preparationID, contentHeightCSS
	oldDeactivation := lastDeactivation
	t.Cleanup(func() {
		stopPreparationTimer()
		host = oldHost
		setPhase(oldPhase)
		windowGeneration.Store(oldGeneration)
		pendingRequest, preparationID, contentHeightCSS = oldPending, oldID, oldHeight
		lastDeactivation = oldDeactivation
	})
	stopPreparationTimer()
	host = h
	setPhase(phaseParked)
	pendingRequest = requestNone
	lastDeactivation = deactivation{}
	return h
}

func TestShowAfterHideWaitsForThePage(t *testing.T) {
	h := newWindowState(t)

	showAndFocus(0)
	if phase() != phaseVisible || h.last("__onShow") != `window.__onShow && window.__onShow("main")` {
		t.Fatalf("a parked window shows at once: phase %d, %q", phase(), h.last("__onShow"))
	}

	moveOffScreen(0)
	if phase() != phasePreparing {
		t.Fatalf("after a hide the page is asked to prepare: phase %d", phase())
	}
	id := preparationID
	if want := "__onHide(" + itoa(id) + ", false)"; !strings.HasSuffix(h.last("__onHide"), want) {
		t.Fatalf("preparation request: %q, want suffix %q", h.last("__onHide"), want)
	}

	shows := len(h.scripts)
	showAndFocus(0)
	if phase() != phasePreparing || pendingRequest != requestShow || len(h.scripts) != shows {
		t.Fatal("a show during preparation waits and tells the page nothing")
	}

	completeWindowPreparation(0, id+1, 111)
	completeWindowPreparation(0, id-1, 111)
	if phase() != phasePreparing || contentHeightCSS == 111 {
		t.Fatal("the answer to another request must not release the show or set the height")
	}

	completeWindowPreparation(0, id, 480)
	if phase() != phaseVisible || pendingRequest != requestNone || contentHeightCSS != 480 {
		t.Fatalf("the page's answer sets the height and releases the show: phase %d, pending %d, height %d",
			phase(), pendingRequest, contentHeightCSS)
	}
}

// One lost answer used to leave the window impossible to open: every show
// only renewed the wait.
func TestShowDoesNotDependOnThePageAnswering(t *testing.T) {
	newWindowState(t)
	showAndFocus(0)
	moveOffScreen(0)
	id := preparationID
	showAndFocus(0)

	expirePreparation(0, id+1)
	if phase() != phasePreparing {
		t.Fatal("the timeout of a replaced request must not end the wait")
	}
	expirePreparation(0, id)
	if phase() != phaseVisible || pendingRequest != requestNone {
		t.Fatalf("after the timeout the pending show goes ahead: phase %d, pending %d", phase(), pendingRequest)
	}

	// Nothing pending: the window is simply ready, and a late answer still
	// brings its height.
	moveOffScreen(0)
	id = preparationID
	expirePreparation(0, id)
	if phase() != phaseParked {
		t.Fatalf("after the timeout a hidden window is ready to show: phase %d", phase())
	}
	completeWindowPreparation(0, id, 333)
	if phase() != phaseParked || contentHeightCSS != 333 {
		t.Fatalf("a late answer still sets the height: phase %d, height %d", phase(), contentHeightCSS)
	}
}

func TestPreparationTimerReleasesTheWindow(t *testing.T) {
	h := newWindowState(t)
	showAndFocus(0)
	moveOffScreen(0)
	if preparationTimer == nil {
		t.Fatal("a preparation request starts its timeout")
	}
	preparationTimer.Reset(time.Millisecond)
	h.waitDispatched(t)
	if phase() != phaseParked || preparationTimer != nil {
		t.Fatalf("the timer ends the wait on the UI thread: phase %d", phase())
	}
}

func TestSettingsFromTheTray(t *testing.T) {
	h := newWindowState(t)

	// Parked: the page prepares Settings off-screen, then the window shows.
	showSettings(0)
	if phase() != phasePreparing || !strings.HasSuffix(h.last("__onHide"), ", true)") {
		t.Fatalf("a parked window prepares Settings first: phase %d, %q", phase(), h.last("__onHide"))
	}
	completeWindowPreparation(0, preparationID, 480)
	if phase() != phaseVisible || h.last("__onShow") != `window.__onShow && window.__onShow("settings")` {
		t.Fatalf("then it shows with Settings: phase %d, %q", phase(), h.last("__onShow"))
	}

	// On screen: no slide, the page just switches.
	generation := windowGeneration.Load()
	showSettings(0)
	if windowGeneration.Load() != generation || !strings.Contains(h.scripts[len(h.scripts)-1], "__openSettings") {
		t.Fatal("a visible window opens Settings in place")
	}

	// While the list is being prepared: ask the page again, for Settings.
	moveOffScreen(0)
	first := preparationID
	showAndFocus(0)
	showSettings(0)
	if preparationID == first || pendingRequest != requestSettings || !strings.HasSuffix(h.last("__onHide"), ", true)") {
		t.Fatal("Settings asked for during preparation restarts it for Settings")
	}
	completeWindowPreparation(0, first, 100)
	if phase() != phasePreparing {
		t.Fatal("the answer for the list must not show the window meant for Settings")
	}
	// A plain show must not downgrade the pending Settings request.
	showAndFocus(0)
	expirePreparation(0, preparationID)
	if phase() != phaseVisible || h.last("__onShow") != `window.__onShow && window.__onShow("settings")` {
		t.Fatalf("Settings is still what shows, even unprepared: %q", h.last("__onShow"))
	}
}

func TestShowingAVisibleWindowKeepsItsView(t *testing.T) {
	h := newWindowState(t)
	showAndFocus(0)
	showAndFocus(0)
	if h.last("__onShow") != `window.__onShow && window.__onShow(null)` {
		t.Fatalf("a window already on screen is not reset: %q", h.last("__onShow"))
	}
}

// A hide asked for while the window still stood on the slide-in's starting
// line has fromY == toY. Read off the coordinates it was taken for a show,
// never parked, and the window could not be opened again.
func TestSlideOutParksWhateverTheCoordinates(t *testing.T) {
	h := newWindowState(t)
	generation := windowGeneration.Add(1)
	setPhase(phaseHiding)
	pendingRequest = requestShow

	animateY(0, 0, 700, 700, 2*time.Millisecond, easeInCubic, generation, true)
	h.waitDispatched(t)
	if phase() != phasePreparing {
		t.Fatalf("the slide-out ends in preparation: phase %d", phase())
	}
	if got := h.last("__onWindowTransition"); !strings.HasSuffix(got, "(false, "+itoa(generation)+")") {
		t.Fatalf("the end of the slide is reported: %q", got)
	}
	completeWindowPreparation(0, preparationID, 480)
	if phase() != phaseVisible {
		t.Fatal("the show that was waiting goes ahead")
	}
}

func TestReplacedSlideReportsNothing(t *testing.T) {
	h := newWindowState(t)
	stale := windowGeneration.Add(1)
	windowGeneration.Add(1)
	animateY(0, 0, 700, 0, 2*time.Millisecond, easeOutCubic, stale, false)
	h.runDispatched()
	if len(h.scripts) != 0 {
		t.Fatalf("a slide a newer one replaced stays silent: %v", h.scripts)
	}
}

func TestResizeIsIgnoredWhileThePagePrepares(t *testing.T) {
	newWindowState(t)
	showAndFocus(0)
	resizeToContent(0, 300)
	if contentHeightCSS != 300 {
		t.Fatal("a visible window takes the reported height")
	}
	moveOffScreen(0)
	resizeToContent(0, 200)
	if contentHeightCSS != 300 {
		t.Fatal("heights reported mid-reset are dropped; the answer carries the one that counts")
	}
	completeWindowPreparation(0, preparationID, 250)
	resizeToContent(0, 260)
	if contentHeightCSS != 260 {
		t.Fatal("a parked, prepared window takes the reported height again")
	}
}

func TestWindowSizeForDPI(t *testing.T) {
	tests := []struct {
		name         string
		dpi          uint32
		contentCSS   int
		workAreaH    int32
		wantW, wantH int32
	}{
		{"100 %", 96, 451, 1032, 420, 451},
		{"125 %", 120, 451, 1140, 525, 564},
		{"150 %", 144, 451, 1400, 630, 677},
		{"200 %", 192, 451, 2000, 840, 902},
		{"tiny content gets the minimum height", 120, 10, 1140, 525, 100},
		{"tall content stops at the work area minus margins", 120, 2000, 1140, 525, 1120},
	}
	for _, tt := range tests {
		w, h := windowSizeForDPI(tt.dpi, tt.contentCSS, tt.workAreaH)
		if w != tt.wantW || h != tt.wantH {
			t.Errorf("%s: windowSizeForDPI(%d, %d, %d) = %dx%d, want %dx%d",
				tt.name, tt.dpi, tt.contentCSS, tt.workAreaH, w, h, tt.wantW, tt.wantH)
		}
	}
}

func TestDecideToggle(t *testing.T) {
	const none = -1
	tests := []struct {
		name  string
		state toggleState
		want  toggleAction
	}{
		{"hidden window is shown by the icon",
			toggleState{hidden: true, fromTray: true, lostAgo: none}, toggleShow},
		{"hidden window is shown by the hotkey",
			toggleState{hidden: true, lostAgo: none}, toggleShow},
		{"the click that made auto-hide fire leaves the window hidden",
			toggleState{hidden: true, fromTray: true, lostAgo: 60 * time.Millisecond, lostToShell: true, lostByHiding: true}, toggleNone},
		{"a click long after auto-hide shows the window",
			toggleState{hidden: true, fromTray: true, lostAgo: 2 * time.Second, lostToShell: true, lostByHiding: true}, toggleShow},
		{"the hotkey right after auto-hide shows the window",
			toggleState{hidden: true, lostAgo: 60 * time.Millisecond, lostByHiding: true}, toggleShow},

		{"active window is hidden by the hotkey",
			toggleState{ownsForeground: true, lostAgo: none}, toggleHide},
		{"window with its own menu or dialog in front is still the active one",
			toggleState{ownsForeground: true, fromTray: true, lostAgo: none}, toggleHide},

		{"pinned window, icon on the taskbar, quick click",
			toggleState{fromTray: true, lostAgo: 80 * time.Millisecond, lostToShell: true}, toggleHide},
		{"pinned window, button held down for a moment",
			toggleState{fromTray: true, lostAgo: 900 * time.Millisecond, lostToShell: true}, toggleHide},
		{"pinned window, icon found in the overflow flyout",
			toggleState{fromTray: true, lostAgo: 3 * time.Second, lostToShell: true}, toggleHide},
		{"pinned window left for another program comes forward on a click",
			toggleState{fromTray: true, lostAgo: 80 * time.Millisecond}, toggleFocus},
		{"pinned window left for the taskbar long ago comes forward on a click",
			toggleState{fromTray: true, lostAgo: time.Minute, lostToShell: true}, toggleFocus},
		{"pinned window that never was active comes forward on a click",
			toggleState{fromTray: true, lostAgo: none}, toggleFocus},
		{"pinned inactive window comes forward on the hotkey",
			toggleState{lostAgo: time.Second, lostToShell: true}, toggleFocus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideToggle(tt.state); got != tt.want {
				t.Fatalf("decideToggle(%+v) = %d, want %d", tt.state, got, tt.want)
			}
		})
	}
}

func itoa(n uint64) string { return strconv.FormatUint(n, 10) }

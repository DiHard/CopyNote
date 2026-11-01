package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"copynote/internal/macui"
	"copynote/internal/service"
	"copynote/internal/singleton"
	"copynote/internal/tray"
)

// Cocoa runs on the main thread only, and the main goroutine starts there.
func init() { runtime.LockOSThread() }

// singletonName names the lock and the socket in ~/Library/Caches. A var so a
// test build can run beside the real application (-X main.singletonName=...).
var singletonName = "dev.copynote.app"

// pageOrigin is the page's origin. Nothing is fetched from it: the page is one
// HTML file with everything inlined.
const pageOrigin = "https://copynote.invalid/"

// devtoolsStart is set by the devtools build only (devtools_darwin.go).
var devtoolsStart func(*macui.WebView)

// The cold start, on the main thread. Requests made before the page reports
// ready wait here, like the tray's pending slot on Windows: a window shown
// half-loaded looks broken, and a click that does nothing looks dead.
var (
	uiReady        bool
	startupRequest showRequest
)

// goQuit ends the run loop; main then shuts down as on any other quit.
func goQuit() {
	quitting.Store(true)
	macui.Stop()
}

// userRequest is a show the user asked for: the menu bar menu, a second
// launch, reopening the app.
func userRequest(want showRequest) {
	if !uiReady {
		startupRequest = max(startupRequest, want)
		return
	}
	requestWindow(want)
}

// userToggle is a click on the icon and the hotkey. Before the page is ready
// there is no window to put away, so it only ever queues a show.
func userToggle() {
	if !uiReady {
		startupRequest = max(startupRequest, requestShow)
		return
	}
	toggleWindow()
}

func main() {
	closeLog := initializeLogging()
	defer closeLog()

	release, already, err := singleton.Acquire(singletonName)
	if err != nil {
		fatalStartup("singleton: %v", err)
	}
	defer release()
	if already {
		delivered := singleton.ShowRunning(singletonName, 15*time.Second)
		log.Printf("another instance is running; show request delivered: %v", delivered)
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		log.Printf("executable path: %v", err)
		exePath = ""
	}

	// ~/Library/Application Support/CopyNote/data.json, the same format as
	// %APPDATA%\CopyNote\data.json on Windows.
	configDir, err := os.UserConfigDir()
	if err != nil {
		fatalStartup("settings folder: %v", err)
	}
	dataFile := filepath.Join(configDir, "CopyNote", "data.json")
	var svc *service.Service
	locale := func() string {
		if s, err := svc.GetSettings(); err == nil && s.Locale != "" && s.Locale != "system" {
			return s.Locale
		}
		return systemLocale()
	}
	refreshTooltip := func() {
		texts := tray.TextsFor(locale())
		if uiReady {
			macui.SetStatusTooltip(texts.TipReady)
		} else {
			macui.SetStatusTooltip(texts.TipLoading)
		}
	}
	svc, err = openService(dataFile, service.Dependencies{
		// Runs under the service lock: only queue the work.
		SettingsSaved: func() { onUIThread(refreshTooltip) },
	})
	if err != nil {
		fatalStartup("service init: %v", err)
	}
	svc.EnsureAutorunPath()

	startupHotkey := ""
	if s, err := svc.GetSettings(); err == nil {
		topmostEnabled.Store(s.Topmost)
		autoHideDisabled.Store(s.DisableAutoHide)
		startupHotkey = s.Hotkey
	} else {
		topmostEnabled.Store(true)
	}

	statusMenu := func() {
		texts := tray.TextsFor(locale())
		items := []macui.MenuItem{{Label: texts.Open}, {Label: texts.Settings}, {Separator: true}, {Label: texts.Quit}}
		macui.ShowMenu(items, 0, 0, true, macui.AppearanceSystem, func(index int) {
			switch index {
			case 0:
				userRequest(requestShow)
			case 1:
				userRequest(requestSettings)
			case 3:
				goQuit()
			}
		})
	}
	view := macui.New(windowWidth, initialWindowHeight, macui.Handlers{
		StatusClick: func(right bool) {
			if right {
				statusMenu()
			} else {
				userToggle()
			}
		},
		Hotkey:        userToggle,
		ResignActive:  onResignActive,
		Reopen:        func() { userRequest(requestShow) },
		QuitRequested: goQuit,
	})
	host = view
	macui.SetTopmost(topmostEnabled.Load())
	refreshTooltip()

	updates := bindApplication(view, svc, exePath)
	defer updates.Close()
	mustBind(view, "notifyReady", func() {
		if uiReady {
			return // a reload
		}
		uiReady = true
		refreshTooltip()
		if want := startupRequest; want != requestNone {
			startupRequest = requestNone
			requestWindow(want)
		}
	})

	// A shortcut another program holds is not a startup failure: the app
	// still works from the icon, and Settings shows the problem.
	if err := applyHotkey(startupHotkey); err != nil {
		log.Printf("hotkey %q: %v", startupHotkey, err)
	}

	stopListening, err := singleton.Listen(singletonName, func() {
		onUIThread(func() { userRequest(requestShow) })
	})
	if err != nil {
		log.Printf("listen for second launches: %v", err)
	} else {
		defer stopListening()
	}

	// Started by hand, the window comes up as soon as the page is ready; at
	// sign-in the app stays in the menu bar.
	if !startedByAutorun() {
		startupRequest = requestShow
	}

	if devtoolsStart != nil {
		devtoolsStart(view)
	}

	html, err := frontendHTML()
	if err != nil {
		fatalStartup("embedded page: %v", err)
	}
	view.LoadHTML(html, pageOrigin)
	macui.Run()
	updates.Close()
}

package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jchv/go-webview2"

	"copynote/internal/singleton"
	"copynote/internal/tray"
	"copynote/internal/updater"
	"copynote/internal/winutil"
)

// Single-file frontend produced by Vite + vite-plugin-singlefile.
// Served over loopback HTTP — NavigateToString gives an about:blank
// origin that breaks ES modules, so we use http://127.0.0.1:PORT/
// instead. Only one request is made (HTML+CSS+JS are all inlined).
//
//go:embed all:web/dist
var distFS embed.FS

// Chromium switches passed to WebView2 to disable background services
// that add startup latency on first navigation (Safe Browsing updater,
// domain reliability telemetry, component updater, etc.).
// singletonName is the single-instance mutex. A var so a test build can run
// beside the real application (-X main.singletonName=...; the tray class and
// show message names in internal/tray have the same override).
var singletonName = `Local\dev.copynote.app.singleton`

var browserArgs = []string{
	"--disable-background-networking",
	"--disable-component-update",
	"--disable-sync",
	"--no-first-run",
}

func main() {
	closeLog := initializeLogging()
	defer closeLog()
	// 0. Per-monitor DPI awareness has to be set before the first window
	//    exists. Without it Windows bitmap-stretches the window on scaled
	//    displays and the WebView2 text looks blurry.
	if err := winutil.EnablePerMonitorDPIAwareness(); err != nil {
		log.Printf("per-monitor DPI awareness: %v", err)
	}
	// 1. Single-instance lock. If another CopyNote is already running,
	//    ask it to show its window and exit.
	release, already, err := singleton.Acquire(singletonName)
	if err != nil {
		fatalStartup("singleton: %v", err)
	}
	releaseOnce := sync.OnceFunc(release)
	defer releaseOnce()

	if already {
		// The wait covers a running instance that is still in its
		// WebView2 cold start and has no tray window yet.
		delivered := tray.ShowRunningInstance(15 * time.Second)
		log.Printf("another instance is running; show request delivered: %v", delivered)
		return
	}

	// 1b. Where this binary lives. Self-update swaps the file in place and
	//     relaunches it; a download interrupted last time is discarded.
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("executable path: %v (self-update disabled)", err)
		exePath = ""
	} else {
		updater.RemoveStaging(exePath)
	}

	// 2. Persistent WebView2 user-data folder in
	//    %LOCALAPPDATA%\CopyNote\WebView2 so cache survives between runs.
	var dataPath string
	if appData := os.Getenv("LOCALAPPDATA"); appData != "" {
		dataPath = filepath.Join(appData, "CopyNote", "WebView2")
		if err := os.MkdirAll(dataPath, 0o755); err != nil {
			log.Printf("mkdir datapath: %v", err)
		}
	}

	// 3. Actual user data (entries) lives in %APPDATA%\CopyNote\data.json.
	appDataRoaming := os.Getenv("APPDATA")
	if appDataRoaming == "" {
		fatalStartup("APPDATA env var is not set")
	}
	dataFile := filepath.Join(appDataRoaming, "CopyNote", "data.json")
	svc, err := openService(dataFile)
	if err != nil {
		fatalStartup("service init: %v", err)
	}

	// 3b. Self-heal autorun registry if exe was moved.
	svc.EnsureAutorunPath()

	// 3c. Load settings early so topmost preference is known before the
	//     first showAndFocus call.
	if s, err := svc.GetSettings(); err == nil {
		topmostEnabled.Store(s.Topmost)
	} else {
		topmostEnabled.Store(true) // default
	}

	// 4. WebView2 picks up this env var before spawning msedgewebview2.exe.
	//    A value already in the environment is kept: it is the documented
	//    WebView2 debugging hook (e.g. --remote-debugging-port=9222 to attach
	//    DevTools or drive the UI over CDP in end-to-end tests).
	args := browserArgs
	if extra := os.Getenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"); extra != "" {
		args = append(append([]string(nil), browserArgs...), extra)
	}
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", strings.Join(args, " "))

	// 5. Loopback HTTP server serving the embedded frontend.
	staticFS, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/", http.FileServer(http.FS(staticFS)))
		_ = http.Serve(ln, mux)
	}()
	url := fmt.Sprintf("http://%s/", ln.Addr().String())

	// 6. Create the webview window.
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  dataPath,
		WindowOptions: webview2.WindowOptions{
			Title:  "CopyNote",
			Width:  windowWidth,
			Height: initialWindowHeight,
		},
	})
	if w == nil {
		fatalStartup("failed to create webview")
	}
	defer w.Destroy()

	hwnd := uintptr(w.Window())

	// 6b. Immediately move the window off-screen so the unstyled
	//     title-bar window isn't visible during the ~9 s WebView2
	//     cold-start. The window will be moved to the tray corner
	//     and shown later when the user clicks the tray icon.
	parkOffScreen(hwnd)
	// go-webview2 took the size in physical pixels; scale it for the DPI
	// of the monitor the window now belongs to.
	applyWindowSize(hwnd, winutil.DpiForWindow(hwnd))

	updates := bindApplication(w, hwnd, svc, exePath)
	defer updates.Close()

	// 8. Tray. Runs on a dedicated OS-locked goroutine; communicates
	//    with the webview UI thread via w.Dispatch.
	trayCtrl := &tray.Tray{
		OnShow: func() {
			w.Dispatch(func() { showAndFocus(hwnd) })
		},
		OnToggle: func() {
			w.Dispatch(func() { toggleVisibility(hwnd) })
		},
		OnSettings: func() {
			w.Dispatch(func() {
				showAndFocus(hwnd)
				w.Eval(`window.__openSettings && window.__openSettings()`)
			})
		},
		OnQuit: func() {
			quitting.Store(true)
			w.Dispatch(func() { w.Terminate() })
		},
		GetLocale: func() string {
			s, err := svc.GetSettings()
			if err != nil {
				return winutil.SystemLocale()
			}
			if s.Locale == "" || s.Locale == "system" {
				return winutil.SystemLocale()
			}
			return s.Locale
		},
	}
	if err := w.Bind("notifyReady", func() {
		trayCtrl.SetReady()
		if exePath != "" {
			// The version replaced by a self-update may still be exiting and
			// holding its file, so removal is retried for a while.
			go func() {
				if err := updater.RemovePrevious(exePath, time.Minute); err != nil {
					log.Printf("remove previous version: %v", err)
				}
			}()
		}
	}); err != nil {
		log.Fatal(err)
	}

	trayDone := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(trayDone)
		if err := trayCtrl.Run(); err != nil {
			log.Printf("tray: %v", err)
		}
		// Tray exited (crash, error, or explorer.exe restart removed
		// the icon). Terminate webview so the process doesn't hang as
		// an invisible zombie with no tray icon.
		quitting.Store(true)
		w.Dispatch(func() { w.Terminate() })
	}()

	// 9. Subclass FIRST — installs WM_NCCALCSIZE handler that
	//    eliminates the non-client frame strip. Must be in place
	//    BEFORE SWP_FRAMECHANGED triggers WM_NCCALCSIZE.
	installSubclass(hwnd, trayCtrl)

	// 10. Frameless window: strip title bar, hide from taskbar/Alt+Tab,
	//     set app icon, apply rounded corners. Order matters — subclass
	//     must be installed before SWP_FRAMECHANGED triggers WM_NCCALCSIZE.
	style := winutil.GetWindowLongPtr(hwnd, winutil.GWL_STYLE)
	style &^= winutil.WS_CAPTION
	winutil.SetWindowLongPtr(hwnd, winutil.GWL_STYLE, style)

	exStyle := winutil.GetWindowLongPtr(hwnd, winutil.GWL_EXSTYLE)
	exStyle |= winutil.WS_EX_TOOLWINDOW
	winutil.SetWindowLongPtr(hwnd, winutil.GWL_EXSTYLE, exStyle)

	// Set the window icon to our embedded resource (dark variant = id 1).
	winutil.SetWindowIcon(hwnd, 1)

	// Set window background brush to light surface color so any area
	// exposed during resize is #f3f3f3 instead of black.
	winutil.SetWindowBackgroundColor(hwnd, 0xf3, 0xf3, 0xf3)
	winutil.SetWindowPos(hwnd, 0, 0, 0, 0, 0,
		winutil.SWP_FRAMECHANGED|winutil.SWP_NOMOVE|winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
	winutil.DwmSetWindowCornerPreference(hwnd, 2) // DWMWCP_ROUND

	// 11. Navigate. The window is parked off-screen (windowHidden=true)
	//     but WS_VISIBLE so WebView2's renderer is NOT throttled.
	//     We never use SW_HIDE — instead, "hidden" means off-screen
	//     and "shown" means anchored to the tray corner.
	w.Navigate(url)

	// 12. Run the webview message loop. Blocks until Terminate().
	w.Run()
	updates.Close()

	// 11. Cleanup: tear down tray and wait for its goroutine to exit.
	trayCtrl.Stop()
	<-trayDone

	// 12. A self-update was installed: free the single-instance mutex before
	//     the replaced executable checks it, then start it.
	if restartRequested.Load() && exePath != "" {
		_ = ln.Close()
		releaseOnce()
		relaunch(exePath)
	}
}

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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jchv/go-webview2"

	"copynote/internal/service"
	"copynote/internal/singleton"
	"copynote/internal/tray"
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
var browserArgs = []string{
	"--disable-background-networking",
	"--disable-component-update",
	"--disable-domain-reliability",
	"--disable-sync",
	"--disable-breakpad",
	"--disable-client-side-phishing-detection",
	"--disable-default-apps",
	"--disable-features=AutofillServerCommunication,CertificateTransparencyComponentUpdater,OptimizationHints",
	"--no-pings",
	"--no-first-run",
	"--no-default-browser-check",
	"--no-service-autorun",
	"--metrics-recording-only",
}

// Subclassing state for the webview window. The callback must outlive
// the window, so it lives in package scope. quitting flips to true
// just before w.Terminate() so a stray WM_CLOSE during shutdown is
// allowed through to the original WndProc.
//
// lastShownNS records the last time the window was shown (or first
// painted), used to suppress an immediate auto-hide if a focus race
// fires WM_ACTIVATEAPP=false within ~300 ms of the show.
//
// activationLossHideNS records the last time WM_ACTIVATEAPP hid the
// window. This is used by toggleVisibility to avoid a race where a
// left click on the tray icon triggers both:
//  1. WM_ACTIVATEAPP=0 on the main window (explorer.exe took
//     foreground to dispatch the click) → auto-hide fires, AND
//  2. the tray's WM_LBUTTONUP → OnToggle sees the window already
//     hidden and would re-show it, defeating the toggle.
// If OnToggle runs within the debounce window of an auto-hide, it
// treats the click as "the user wanted to hide" and keeps it hidden.
var (
	origWndProc          uintptr
	wndProcCB            uintptr
	quitting             atomic.Bool
	lastShownNS          atomic.Int64
	activationLossHideNS atomic.Int64
)

// hideGuardWindow is the minimum time after a show during which we
// will NOT auto-hide on focus loss.
const hideGuardWindow = 300 * time.Millisecond

// toggleDebounce is the window during which a toggle-click after an
// activation-loss hide is interpreted as "user wanted hidden" rather
// than "user wants to show again".
const toggleDebounce = 150 * time.Millisecond

func main() {
	// 1. Single-instance lock. If another CopyNote is already running,
	//    broadcast a "show window" message to it and exit immediately.
	release, already, err := singleton.Acquire(`Local\dev.copynote.app.singleton`)
	if err != nil {
		log.Fatalf("singleton: %v", err)
	}
	defer release()

	if already {
		showMsgID, err := winutil.RegisterWindowMessage("dev.copynote.app.SHOW")
		if err == nil && showMsgID != 0 {
			winutil.PostMessage(winutil.HWND_BROADCAST, showMsgID, 0, 0)
		}
		return
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
		log.Fatal("APPDATA env var is not set")
	}
	dataFile := filepath.Join(appDataRoaming, "CopyNote", "data.json")
	svc, err := service.New(dataFile)
	if err != nil {
		log.Fatalf("service init: %v", err)
	}

	// 4. WebView2 picks up this env var before spawning msedgewebview2.exe.
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", strings.Join(browserArgs, " "))

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
			Width:  420,
			Height: 640,
			Center: true,
		},
	})
	if w == nil {
		log.Fatal("failed to create webview")
	}
	defer w.Destroy()

	// 7. Subclass the webview window so the close button hides instead
	//    of destroying it.
	hwnd := uintptr(w.Window())
	hideOnClose(hwnd)

	// 8. Bind CRUD bridge methods.
	mustBind := func(name string, fn any) {
		if err := w.Bind(name, fn); err != nil {
			log.Fatalf("bind %s: %v", name, err)
		}
	}
	mustBind("list", svc.List)
	mustBind("create", svc.Create)
	mustBind("update", svc.Update)
	mustBind("remove", svc.Delete) // "delete" is a JS operator, use "remove"
	mustBind("copy", svc.Copy)

	// 9. Tray. Runs on a dedicated OS-locked goroutine; communicates
	//    with the webview UI thread via w.Dispatch.
	trayCtrl := &tray.Tray{
		OnShow: func() {
			w.Dispatch(func() { showAndFocus(hwnd) })
		},
		OnToggle: func() {
			w.Dispatch(func() { toggleVisibility(hwnd) })
		},
		OnQuit: func() {
			quitting.Store(true)
			w.Dispatch(func() { w.Terminate() })
		},
	}
	trayDone := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(trayDone)
		if err := trayCtrl.Run(); err != nil {
			log.Printf("tray: %v", err)
		}
	}()

	// 10. Run the webview message loop. Blocks until Terminate().
	//     Mark "just shown" so the focus-loss auto-hide guard doesn't
	//     fire on a startup race.
	lastShownNS.Store(time.Now().UnixNano())
	w.Navigate(url)
	w.Run()

	// 11. Cleanup: tear down tray and wait for its goroutine to exit.
	trayCtrl.Stop()
	<-trayDone
}

// hideOnClose installs a subclass WndProc on hwnd that intercepts
// WM_CLOSE (close button → hide instead of destroy) and WM_ACTIVATEAPP
// (focus moves to another process → hide). Quitting bypasses both.
//
// WM_ACTIVATEAPP fires only on cross-process focus changes, so our
// own tray popup window (same process) does NOT trigger an auto-hide
// when shown.
func hideOnClose(hwnd uintptr) {
	wndProcCB = syscall.NewCallback(func(h, msg, wParam, lParam uintptr) uintptr {
		if quitting.Load() {
			return winutil.CallWindowProc(origWndProc, h, msg, wParam, lParam)
		}
		switch msg {
		case winutil.WM_CLOSE:
			winutil.ShowWindow(h, winutil.SW_HIDE)
			return 0
		case winutil.WM_ACTIVATEAPP:
			// wParam == 0 → window is becoming inactive (another
			// process took foreground).
			if wParam == 0 {
				elapsed := time.Now().UnixNano() - lastShownNS.Load()
				if elapsed > int64(hideGuardWindow) && winutil.IsWindowVisible(h) {
					activationLossHideNS.Store(time.Now().UnixNano())
					winutil.ShowWindow(h, winutil.SW_HIDE)
				}
			}
		}
		return winutil.CallWindowProc(origWndProc, h, msg, wParam, lParam)
	})
	origWndProc = winutil.SetWindowLongPtr(hwnd, winutil.GWLP_WNDPROC, wndProcCB)
}

// showAndFocus restores the window if minimized, makes it visible,
// and brings it to the foreground. Called via w.Dispatch from the
// tray thread. Updates the show-guard timestamp so auto-hide doesn't
// fire immediately on the activation race.
func showAndFocus(hwnd uintptr) {
	lastShownNS.Store(time.Now().UnixNano())
	if winutil.IsIconic(hwnd) {
		winutil.ShowWindow(hwnd, winutil.SW_RESTORE)
	} else {
		winutil.ShowWindow(hwnd, winutil.SW_SHOW)
	}
	winutil.SetForegroundWindow(hwnd)
}

// toggleVisibility hides the window if it's currently visible and on
// screen, otherwise shows and focuses it.
//
// Special case: if the window was hidden by WM_ACTIVATEAPP within the
// last few hundred milliseconds, treat the toggle as "stay hidden" —
// the click on the tray icon is what caused that activation loss in
// the first place, and the user's intent is clearly to hide, not to
// immediately re-open.
func toggleVisibility(hwnd uintptr) {
	if t := activationLossHideNS.Load(); t != 0 {
		if time.Since(time.Unix(0, t)) < toggleDebounce {
			activationLossHideNS.Store(0)
			return
		}
		activationLossHideNS.Store(0)
	}
	if winutil.IsWindowVisible(hwnd) && !winutil.IsIconic(hwnd) {
		winutil.ShowWindow(hwnd, winutil.SW_HIDE)
		return
	}
	showAndFocus(hwnd)
}

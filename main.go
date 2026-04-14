package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	"--disable-sync",
	"--no-first-run",
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

// trayCornerMargin is the gap between the window and the screen /
// taskbar edges when anchored to the tray corner.
const trayCornerMargin = 8

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
		},
	})
	if w == nil {
		log.Fatal("failed to create webview")
	}
	defer w.Destroy()

	hwnd := uintptr(w.Window())

	// 6b. Immediately move the window off-screen so the unstyled
	//     title-bar window isn't visible during the ~9 s WebView2
	//     cold-start. The window will be moved to the tray corner
	//     and shown later when the user clicks the tray icon.
	winutil.SetWindowPos(hwnd, 0, -10000, -10000, 0, 0,
		winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)

	// 7. Bind CRUD bridge methods.
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
	mustBind("hide", func() {
		w.Dispatch(func() {
			moveOffScreen(hwnd)
		})
	})
	mustBind("getSettings", svc.GetSettings)
	mustBind("saveSettings", svc.SaveSettings)
	mustBind("resizeWindow", func(contentHeight int) {
		w.Dispatch(func() {
			resizeToContent(hwnd, contentHeight)
		})
	})

	mustBind("openExternal", func(url string) {
		winutil.OpenURL(url)
	})

	const fileFilter = "CopyNote Backup (*.json)|*.json|All Files|*.*"

	mustBind("exportData", func() error {
		data, err := svc.ExportData()
		if err != nil {
			return err
		}
		path, ok := winutil.SaveFileDialog(hwnd, fileFilter, "copynote-backup.json")
		if !ok {
			return nil // user cancelled
		}
		return os.WriteFile(path, data, 0o644)
	})

	mustBind("importData", func() error {
		path, ok := winutil.OpenFileDialog(hwnd, fileFilter)
		if !ok {
			return nil // user cancelled
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		if err := svc.ImportData(raw); err != nil {
			return err
		}
		// Refresh UI: reload entries + settings.
		w.Dispatch(func() {
			w.Eval(`window.__refreshAfterImport && window.__refreshAfterImport()`)
		})
		return nil
	})

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
	trayDone := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(trayDone)
		if err := trayCtrl.Run(); err != nil {
			log.Printf("tray: %v", err)
		}
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

	// 11. Navigate. The window is already off-screen (-10000,-10000)
	//     but WS_VISIBLE so WebView2's renderer is NOT throttled.
	//     We never use SW_HIDE — instead, "hidden" means off-screen
	//     and "shown" means anchored to the tray corner.
	w.Navigate(url)

	// 12. Run the webview message loop. Blocks until Terminate().
	w.Run()

	// 11. Cleanup: tear down tray and wait for its goroutine to exit.
	trayCtrl.Stop()
	<-trayDone
}

// windowWidth is the fixed width of the CopyNote window.
const windowWidth = 420

// minWindowHeight is the minimum window height (header + some padding).
const minWindowHeight = 80

// resizeToContent adjusts the window height to fit contentHeight
// pixels reported by the frontend, clamped to [minWindowHeight,
// workAreaHeight - 2*margin]. The window is re-anchored to the
// bottom-right tray corner after resizing.
func resizeToContent(hwnd uintptr, contentHeight int) {
	wa, ok := winutil.GetWorkArea()
	if !ok {
		return
	}
	maxH := int(wa.Bottom-wa.Top) - 2*trayCornerMargin
	h := contentHeight
	if h < minWindowHeight {
		h = minWindowHeight
	}
	if h > maxH {
		h = maxH
	}

	winutil.SetWindowPos(hwnd, 0, 0, 0, int32(windowWidth), int32(h),
		winutil.SWP_NOMOVE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
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
			if wParam == 0 {
				elapsed := time.Now().UnixNano() - lastShownNS.Load()
				if elapsed > int64(hideGuardWindow) && isOnScreen(h) {
					activationLossHideNS.Store(time.Now().UnixNano())
					moveOffScreen(h)
				}
			}
		case winutil.WM_SETTINGCHANGE:
			if winutil.StringFromLPCWSTR(lParam) == "ImmersiveColorSet" && tr != nil {
				tr.ReloadIcon()
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
//
// The window is re-anchored to the tray corner on every show, so
// secondary monitor changes or taskbar resizes between runs don't
// leave it stranded off-screen.
// animMu serializes show/hide animations so they don't overlap.
var animMu sync.Mutex

// animating is true while a slide animation is in progress.
// Used by toggleVisibility to avoid conflicting actions.
var animating atomic.Bool

func showAndFocus(hwnd uintptr) {
	lastShownNS.Store(time.Now().UnixNano())

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

	var borderRight, borderBottom int32
	if efb, ok := winutil.GetExtendedFrameBounds(hwnd); ok {
		borderRight = wr.Right - efb.Right
		borderBottom = wr.Bottom - efb.Bottom
	}

	targetX := wa.Right - width - trayCornerMargin + borderRight
	targetY := wa.Bottom - height - trayCornerMargin + borderBottom
	startY := wa.Bottom // start just below the screen

	// Place at starting position and bring to front.
	winutil.SetWindowPos(hwnd, 0, targetX, startY, 0, 0,
		winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
	winutil.SetForegroundWindow(hwnd)

	// Animate slide-up.
	go animateY(hwnd, targetX, startY, targetY, 200*time.Millisecond, easeOutCubic)
}

// moveOffScreen hides the window by sliding it down below the screen edge.
// Unlike SW_HIDE this keeps WS_VISIBLE set so WebView2's renderer
// is never throttled — preventing the blank-page bug.
func moveOffScreen(hwnd uintptr) {
	wa, ok := winutil.GetWorkArea()
	wr, ok2 := winutil.GetWindowRect(hwnd)
	if !ok || !ok2 || wr.Left < -5000 {
		// Already off-screen or can't read — just move instantly.
		winutil.SetWindowPos(hwnd, 0, -10000, -10000, 0, 0,
			winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
		return
	}
	endY := wa.Bottom // below screen
	go animateY(hwnd, wr.Left, wr.Top, endY, 150*time.Millisecond, easeInCubic)
}

// isOnScreen reports whether the window's left edge is within a
// plausible screen range (i.e., not moved off-screen to hide).
func isOnScreen(hwnd uintptr) bool {
	if animating.Load() {
		return false // treat as off-screen during animation
	}
	wr, ok := winutil.GetWindowRect(hwnd)
	if !ok {
		return false
	}
	return wr.Left > -5000
}

// animateY slides the window from startY to endY over duration.
// Uses SetWindowPos from a background goroutine (safe for top-level
// windows — Windows marshals the call internally).
func animateY(hwnd uintptr, x, fromY, toY int32, duration time.Duration, ease func(float64) float64) {
	animMu.Lock()
	defer animMu.Unlock()
	animating.Store(true)
	defer animating.Store(false)

	const steps = 20
	stepDur := duration / steps
	for i := 1; i <= steps; i++ {
		t := ease(float64(i) / float64(steps))
		y := fromY + int32(float64(toY-fromY)*t)
		winutil.SetWindowPos(hwnd, 0, x, y, 0, 0,
			winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
		time.Sleep(stepDur)
	}
	// Ensure final position is exact.
	if toY > fromY {
		// Hiding — park off-screen.
		winutil.SetWindowPos(hwnd, 0, -10000, -10000, 0, 0,
			winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE)
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
	var borderRight, borderBottom int32
	if efb, ok := winutil.GetExtendedFrameBounds(hwnd); ok {
		borderRight = wr.Right - efb.Right
		borderBottom = wr.Bottom - efb.Bottom
	}

	x := wa.Right - width - trayCornerMargin + borderRight
	y := wa.Bottom - height - trayCornerMargin + borderBottom
	winutil.SetWindowPos(
		hwnd,
		0,
		x, y, 0, 0,
		winutil.SWP_NOSIZE|winutil.SWP_NOZORDER|winutil.SWP_NOACTIVATE,
	)
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
	if isOnScreen(hwnd) {
		moveOffScreen(hwnd)
		return
	}
	showAndFocus(hwnd)
}

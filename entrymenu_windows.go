package main

import (
	"copynote/internal/popupmenu"
	"copynote/internal/winutil"
	"encoding/json"
	"fmt"
	"github.com/jchv/go-webview2"
	"log"
	"math"
)

// The entry list's context menu is the tray icon's owner-drawn popup
// (internal/popupmenu), not something drawn inside the page: with one or two
// entries the window is barely taller than the menu, and only a window of its
// own can run past the edges.
//
// It is shown on the UI thread with the main window as its owner. Activation
// then stays within one thread, so the main window receives no WM_ACTIVATEAPP
// and auto-hide leaves it alone while the menu is up; when the menu closes,
// Windows activates the owner again and go-webview2's AutoFocus puts keyboard
// focus back into the page.

// entryMenu is the main window's only context menu. UI thread only.
var entryMenu popupmenu.Menu

// maxEntryMenuItems bounds what the page may ask for; the menu has six rows.
const maxEntryMenuItems = 16

// entryMenuItem matches the page's MenuItem.
type entryMenuItem struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Shortcut  string `json:"shortcut"`
	Disabled  bool   `json:"disabled"`
	Separator bool   `json:"separator"`
}

// entryMenuRequest matches the page's EntryMenuRequest. X and Y are CSS pixels
// in the client area. Token goes back with the answer, so the page can tell
// the answer for a menu a newer one replaced from the current one.
type entryMenuRequest struct {
	Token    int             `json:"token"`
	X        float64         `json:"x"`
	Y        float64         `json:"y"`
	Dark     bool            `json:"dark"`
	Keyboard bool            `json:"keyboard"`
	Items    []entryMenuItem `json:"items"`
}

func bindEntryMenu(w webview2.WebView, hwnd uintptr) {
	err := w.Bind("showEntryMenu", func(req entryMenuRequest) error {
		if len(req.Items) == 0 || len(req.Items) > maxEntryMenuItems {
			return fmt.Errorf("a menu takes 1 to %d items, got %d", maxEntryMenuItems, len(req.Items))
		}
		// Bindings run inside WebView2's message callback; a window that
		// takes activation is opened from the message loop instead.
		w.Dispatch(func() { openEntryMenu(w, hwnd, req) })
		return nil
	})
	if err != nil {
		log.Fatalf("bind showEntryMenu: %v", err)
	}
}

func openEntryMenu(w webview2.WebView, hwnd uintptr, req entryMenuRequest) {
	answer := func(id string) {
		raw, _ := json.Marshal(id)
		w.Eval(fmt.Sprintf("window.__entryMenuClosed && window.__entryMenuClosed(%d, %s)", req.Token, raw))
	}
	// Put away between the click and this dispatch: nothing to open it over.
	if windowHidden.Load() {
		answer("")
		return
	}

	items := make([]popupmenu.Item, len(req.Items))
	for i, it := range req.Items {
		items[i] = popupmenu.Item{
			ID:        uint32(i + 1),
			Label:     it.Label,
			Shortcut:  it.Shortcut,
			Disabled:  it.Disabled,
			Separator: it.Separator,
		}
	}
	// The page knows its theme; the system's may differ.
	theme := popupmenu.ThemeLight
	if req.Dark {
		theme = popupmenu.ThemeDark
	}
	x, y := cssClientToScreen(hwnd, req.X, req.Y)
	opts := popupmenu.Options{Owner: hwnd, Theme: theme, Keyboard: req.Keyboard}
	entryMenu.Show(items, x, y, opts, func(id uint32, picked bool) {
		if !picked {
			answer("")
			return
		}
		answer(req.Items[id-1].ID)
	})
}

// cssClientToScreen converts a point the page measured — CSS pixels in the
// client area — into physical screen coordinates. WebView2 renders at the
// window's DPI, so a CSS pixel is DPI/96 physical ones.
func cssClientToScreen(hwnd uintptr, x, y float64) (int32, int32) {
	scale := float64(winutil.DpiForWindow(hwnd)) / winutil.BaseDPI
	return winutil.ClientToScreen(hwnd, int32(math.Round(x*scale)), int32(math.Round(y*scale)))
}

package main

import (
	"math"

	"github.com/jchv/go-webview2"

	"copynote/internal/popupmenu"
	"copynote/internal/winutil"
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

func bindEntryMenu(w webview2.WebView, hwnd uintptr) {
	mustBind(w, "showEntryMenu", func(req entryMenuRequest) error {
		if err := req.validate(); err != nil {
			return err
		}
		// Bindings run inside WebView2's message callback; a window that
		// takes activation is opened from the message loop instead.
		w.Dispatch(func() { openEntryMenu(w, hwnd, req) })
		return nil
	})
}

func openEntryMenu(w webview2.WebView, hwnd uintptr, req entryMenuRequest) {
	answer := func(id string) {
		w.Eval(entryMenuAnswer(req.Token, id))
	}
	// Put away between the click and this dispatch: nothing to open it over.
	if windowIsHidden() {
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

package main

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

// The contract between Go and the page that every platform's window code
// follows: what Go calls in the page, how long it waits for an answer, and
// the shape of an entry menu request.

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

// preparationTimeout bounds how long a show waits for the page's answer,
// which normally takes a few frames. Without the bound one lost answer — a
// script error in the page, a reload, frames that stopped coming — left the
// window impossible to open until the program was restarted.
const preparationTimeout = 500 * time.Millisecond

// hideGuardWindow is the minimum time after a show during which we
// will NOT auto-hide on focus loss.
const hideGuardWindow = 300 * time.Millisecond

// Window preferences, read whenever they matter, so plain atomic stores are
// all the bindings need.
var (
	quitting       atomic.Bool // set just before the web view is torn down
	topmostEnabled atomic.Bool // user preference for always-on-top
	// autoHideDisabled keeps the window on screen when another program takes
	// focus. Stored inverted so the zero value is the default behaviour.
	autoHideDisabled atomic.Bool
)

// pageHost is what the window code needs from the web view: run a function
// on the UI thread, and evaluate a script in the page. main attaches the web
// view; the tests attach a recorder.
type pageHost interface {
	Dispatch(f func())
	Eval(js string)
}

var host pageHost

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

func (r entryMenuRequest) validate() error {
	if len(r.Items) == 0 || len(r.Items) > maxEntryMenuItems {
		return fmt.Errorf("a menu takes 1 to %d items, got %d", maxEntryMenuItems, len(r.Items))
	}
	return nil
}

// entryMenuAnswer tells the page which item of menu token was picked, "" for
// none.
func entryMenuAnswer(token int, id string) string {
	raw, _ := json.Marshal(id) // a string always marshals
	return fmt.Sprintf("window.__entryMenuClosed && window.__entryMenuClosed(%d, %s)", token, raw)
}

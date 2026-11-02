// Package macui is the macOS host: a borderless window with a WKWebView, the
// menu bar icon, context menus, the global hotkey and the file dialogs.
//
// Cocoa wants all of it on the main thread. The program's main goroutine must
// therefore be locked to it (runtime.LockOSThread in an init function) and
// call Run there; bindings, Dispatch callbacks and the handlers below all run
// on that thread, the way go-webview2 runs them on its UI thread on Windows.
package macui

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -Wall
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore -framework WebKit -framework Carbon -framework UniformTypeIdentifiers
#include <stdlib.h>
#include "macui.h"
*/
import "C"

import (
	"encoding/json"
	"log"
	"sync"
	"unsafe"
)

// Handlers are the events Cocoa reports. Each runs on the main thread and may
// be nil.
type Handlers struct {
	// StatusClick is a click on the menu bar icon; right covers a right
	// click and a Control-click.
	StatusClick func(right bool)
	Hotkey      func()
	// ResignActive: another app took the keyboard.
	ResignActive func()
	// Reopen: CopyNote.app was opened again while running.
	Reopen func()
	// QuitRequested: Cmd+Q. The handler decides how to shut down.
	QuitRequested func()
}

var handlers Handlers

var (
	bindingsMu sync.Mutex
	bindings   = map[string]any{}

	dispatchMu   sync.Mutex
	dispatchNext uintptr
	dispatched   = map[uintptr]func(){}

	menusNext int
	menus     = map[int]func(index int){} // main thread only
)

// WebView is the window's page. Its methods match what bridge.Async and the
// shared bindings expect of go-webview2.
type WebView struct{}

// New creates the application, the hidden window and the menu bar icon.
func New(width, height int, h Handlers) *WebView {
	handlers = h
	C.macuiInit(C.int(width), C.int(height))
	C.macuiAddUserScript(cstr(rpcScript))
	return &WebView{}
}

// cstr is a C copy of s that lives as long as the process; for the few
// strings handed over once at startup.
func cstr(s string) *C.char { return C.CString(s) }

func withCString(s string, f func(*C.char)) {
	p := C.CString(s)
	defer C.free(unsafe.Pointer(p))
	f(p)
}

// Bind makes fn callable from the page as window[name]. Before LoadHTML only.
func (*WebView) Bind(name string, fn any) error {
	if err := checkBindable(fn); err != nil {
		return err
	}
	bindingsMu.Lock()
	bindings[name] = fn
	bindingsMu.Unlock()
	withCString(bindScript(name), func(p *C.char) { C.macuiAddUserScript(p) })
	return nil
}

// Init runs js before the page's own scripts on every load.
func (*WebView) Init(js string) {
	withCString(js, func(p *C.char) { C.macuiAddUserScript(p) })
}

// Dispatch runs f on the main thread, later. Safe from any goroutine.
func (*WebView) Dispatch(f func()) {
	dispatchMu.Lock()
	dispatchNext++
	id := dispatchNext
	dispatched[id] = f
	dispatchMu.Unlock()
	C.macuiDispatch(C.uintptr_t(id))
}

// Eval runs js in the page. Main thread only.
func (*WebView) Eval(js string) {
	withCString(js, func(p *C.char) { C.macuiEval(p) })
}

// LoadHTML shows the page. baseURL is its origin; nothing is fetched from it.
func (*WebView) LoadHTML(html, baseURL string) {
	withCString(html, func(h *C.char) {
		withCString(baseURL, func(b *C.char) { C.macuiLoadHTML(h, b) })
	})
}

// Run processes events until Stop.
func Run() { C.macuiRun() }

// Stop makes Run return. Main thread only.
func Stop() { C.macuiStop() }

// ShowWindow puts the window under the menu bar icon and gives it the keyboard.
func ShowWindow() { C.macuiShowWindow() }

// HideWindow takes the window off screen; giveBackFocus also hands the
// keyboard back to the app that had it before.
func HideWindow(giveBackFocus bool) { C.macuiHideWindow(cbool(giveBackFocus)) }

// FocusWindow brings a window that is already on screen to the front.
func FocusWindow() { C.macuiFocusWindow() }

// SetContentHeight sizes the window for the page's height in points (CSS px),
// within the screen.
func SetContentHeight(height int) { C.macuiSetContentHeight(C.int(height)) }

// WindowKey reports whether the window is on screen and has the keyboard.
func WindowKey() bool { return C.macuiWindowKey() != 0 }

// SetTopmost floats the window above other apps' windows.
func SetTopmost(on bool) { C.macuiSetTopmost(cbool(on)) }

// SetStatusTooltip sets the menu bar icon's hover text.
func SetStatusTooltip(text string) {
	withCString(text, func(p *C.char) { C.macuiSetStatusTooltip(p) })
}

// MenuItem is one row of a popup menu.
type MenuItem struct {
	Label     string
	Shortcut  string // shown beside the label, written the Mac way: "⌘E"
	Disabled  bool
	Separator bool
}

// Appearance of a menu, for a page whose theme differs from the system's.
type Appearance int

const (
	AppearanceSystem Appearance = iota
	AppearanceLight
	AppearanceDark
)

// ShowMenu opens a popup menu at (x, y) in the page, or under the menu bar
// icon when atStatusItem. done gets the picked index, or -1, once it closes.
// A newer menu closes this one.
func ShowMenu(items []MenuItem, x, y float64, atStatusItem bool, appearance Appearance, done func(index int)) {
	menusNext++
	id := menusNext
	menus[id] = done
	labels := make([]*C.char, len(items))
	keys := make([]*C.char, len(items))
	flags := make([]C.int, len(items))
	keyMods := make([]C.ulong, len(items))
	for i, it := range items {
		labels[i] = C.CString(it.Label)
		key, mods, _ := keyEquivalent(it.Shortcut)
		keys[i] = C.CString(key)
		keyMods[i] = C.ulong(mods)
		if it.Disabled {
			flags[i] |= 1
		}
		if it.Separator {
			flags[i] |= 2
		}
	}
	defer func() {
		for i := range labels {
			C.free(unsafe.Pointer(labels[i]))
			C.free(unsafe.Pointer(keys[i]))
		}
	}()
	// The C arrays are copied into an NSMenu before ShowMenu returns. cgo
	// does not allow passing a Go slice of C pointers, hence C memory.
	n := len(items)
	cLabels := (**C.char)(C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(labels[0]))))
	defer C.free(unsafe.Pointer(cLabels))
	copy(unsafe.Slice(cLabels, n), labels)
	cKeys := (**C.char)(C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(keys[0]))))
	defer C.free(unsafe.Pointer(cKeys))
	copy(unsafe.Slice(cKeys, n), keys)
	C.macuiShowMenu(C.int(id), cLabels, cKeys, &keyMods[0], &flags[0], C.int(n), C.double(x), C.double(y), cbool(atStatusItem), C.int(appearance))
}

// CancelMenu closes an open popup menu; it reports -1.
func CancelMenu() { C.macuiCancelMenu() }

// RegisterHotkey registers the global hotkey, replacing none: call
// UnregisterHotkey first. It returns macOS's OSStatus, 0 on success.
func RegisterHotkey(keyCode, modifiers uint32) int {
	return int(C.macuiRegisterHotkey(C.uint32_t(keyCode), C.uint32_t(modifiers)))
}

// UnregisterHotkey drops the registered hotkey, if any.
func UnregisterHotkey() { C.macuiUnregisterHotkey() }

// SavePanel asks where to save a file; ok is false when cancelled.
func SavePanel(defaultName string) (path string, ok bool) {
	var p *C.char
	withCString(defaultName, func(n *C.char) { p = C.macuiSavePanel(n) })
	return takePath(p)
}

// OpenPanel asks for a JSON file to open; ok is false when cancelled.
func OpenPanel() (path string, ok bool) { return takePath(C.macuiOpenPanel()) }

func takePath(p *C.char) (string, bool) {
	if p == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p), true
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

//export goDispatch
func goDispatch(id C.uintptr_t) {
	dispatchMu.Lock()
	f := dispatched[uintptr(id)]
	delete(dispatched, uintptr(id))
	dispatchMu.Unlock()
	if f != nil {
		f()
	}
}

//export goScriptMessage
func goScriptMessage(raw *C.char) {
	var msg rpcMessage
	if err := json.Unmarshal([]byte(C.GoString(raw)), &msg); err != nil {
		log.Printf("invalid message from the page: %v", err)
		return
	}
	if msg.ID == 0 {
		var text string
		if len(msg.Params) == 1 && json.Unmarshal(msg.Params[0], &text) == nil {
			log.Printf("page: %s", text)
		}
		return
	}
	bindingsMu.Lock()
	fn := bindings[msg.Method]
	bindingsMu.Unlock()
	var result any
	var err error
	if fn != nil { // an unknown name resolves to null, as on Windows
		result, err = call(fn, msg.Params)
	}
	(&WebView{}).Eval(answer(msg.ID, result, err))
}

//export goStatusClick
func goStatusClick(right C.int) {
	if handlers.StatusClick != nil {
		handlers.StatusClick(right != 0)
	}
}

//export goMenuClosed
func goMenuClosed(id, index C.int) {
	done := menus[int(id)]
	delete(menus, int(id))
	if done != nil {
		done(int(index))
	}
}

//export goHotkey
func goHotkey() {
	if handlers.Hotkey != nil {
		handlers.Hotkey()
	}
}

//export goResignActive
func goResignActive() {
	if handlers.ResignActive != nil {
		handlers.ResignActive()
	}
}

//export goReopen
func goReopen() {
	if handlers.Reopen != nil {
		handlers.Reopen()
	}
}

//export goQuitRequested
func goQuitRequested() {
	if handlers.QuitRequested != nil {
		handlers.QuitRequested()
	}
}

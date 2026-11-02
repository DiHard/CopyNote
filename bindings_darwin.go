package main

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	"copynote/internal/bridge"
	"copynote/internal/hotkey"
	"copynote/internal/macui"
	"copynote/internal/service"
)

// Bindings run on the main thread, each from a block of its own, so they may
// drive the window and open panels directly.
func bindApplication(view *macui.WebView, svc *service.Service, exePath string) *bridge.Async {
	bindData(view, svc,
		func() (string, bool) { return macui.SavePanel(backupFileName) },
		macui.OpenPanel)

	mustBind(view, "hide", func() { hideWindow(true) })
	mustBind(view, "resizeWindow", resizeToContent)
	mustBind(view, "windowPrepared", completeWindowPreparation)
	mustBind(view, "openExternal", openExternal)
	mustBind(view, "openAppFolder", func() error { return revealApp(exePath) })
	mustBind(view, "applyAutoHide", func(enabled bool) { autoHideDisabled.Store(!enabled) })
	mustBind(view, "applyTopmost", func(enabled bool) {
		topmostEnabled.Store(enabled)
		macui.SetTopmost(enabled)
	})
	// The one preference macOS can refuse, so the error goes back to the UI.
	mustBind(view, "applyHotkey", applyHotkey)
	mustBind(view, "showEntryMenu", func(req entryMenuRequest) error {
		if err := req.validate(); err != nil {
			return err
		}
		openEntryMenu(view, req)
		return nil
	})

	updates := bridge.NewAsync(view)
	bindUpdates(view, updates)
	return updates
}

// openEntryMenu shows the entry list's context menu as a native NSMenu; the
// pick goes back through __entryMenuClosed, as on Windows.
func openEntryMenu(view *macui.WebView, req entryMenuRequest) {
	if phase != phaseVisible {
		view.Eval(entryMenuAnswer(req.Token, "")) // put away meanwhile
		return
	}
	items := make([]macui.MenuItem, len(req.Items))
	for i, it := range req.Items {
		items[i] = macui.MenuItem{Label: it.Label, Shortcut: it.Shortcut, Disabled: it.Disabled, Separator: it.Separator}
	}
	// The page knows its theme; the system's may differ.
	appearance := macui.AppearanceLight
	if req.Dark {
		appearance = macui.AppearanceDark
	}
	macui.ShowMenu(items, req.X, req.Y, false, appearance, func(index int) {
		id := ""
		if index >= 0 && index < len(req.Items) {
			id = req.Items[index].ID
		}
		view.Eval(entryMenuAnswer(req.Token, id))
	})
}

// openExternal opens a web link in the default browser. Only web and mail
// links: open(1) would just as happily start an application.
func openExternal(link string) error {
	u, err := url.Parse(link)
	if err != nil {
		return err
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return exec.Command("/usr/bin/open", u.String()).Run()
	}
	return fmt.Errorf("not a web link: %s", link)
}

// revealApp shows CopyNote.app selected in Finder, or the folder holding a
// binary started outside a bundle.
func revealApp(exePath string) error {
	if exePath == "" {
		return errors.New("executable path is unavailable")
	}
	if bundle, _, found := strings.Cut(exePath, ".app/Contents/MacOS/"); found {
		return exec.Command("/usr/bin/open", "-R", bundle+".app").Run()
	}
	return exec.Command("/usr/bin/open", filepath.Dir(exePath)).Run()
}

// liveHotkey is the combination registered now, valid while liveHotkeyOn.
var (
	liveHotkey   hotkey.Spec
	liveHotkeyOn bool
)

// eventHotKeyExistsErr: this process registered the combination already.
const eventHotKeyExistsErr = -9878

// applyHotkey registers the stored preference, keeping the previous shortcut
// working when macOS refuses the new one — the UI keeps the old preference
// stored in that case, exactly as on Windows.
func applyHotkey(setting string) error {
	spec, enabled, err := hotkey.Resolve(setting)
	if err != nil {
		return err
	}
	prev, prevOn := liveHotkey, liveHotkeyOn
	if prevOn {
		macui.UnregisterHotkey()
		liveHotkeyOn = false
	}
	if !enabled {
		return nil
	}
	err = registerHotkey(spec)
	if err == nil {
		liveHotkey, liveHotkeyOn = spec, true
		return nil
	}
	if prevOn {
		if restoreErr := registerHotkey(prev); restoreErr == nil {
			liveHotkey, liveHotkeyOn = prev, true
		} else {
			log.Printf("restore hotkey after refusal: %v", restoreErr)
		}
	}
	return err
}

func registerHotkey(spec hotkey.Spec) error {
	mods, key, ok := spec.Carbon()
	if !ok {
		return errors.New("a Mac keyboard has no such key")
	}
	switch status := macui.RegisterHotkey(key, mods); status {
	case 0:
		return nil
	case eventHotKeyExistsErr:
		// The UI recognises this wording, as it does Windows' own.
		return errors.New("hot key is already registered")
	default:
		return fmt.Errorf("macOS refused the hot key (OSStatus %d)", status)
	}
}

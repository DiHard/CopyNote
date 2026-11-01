package main

import (
	"context"
	"errors"

	"copynote/internal/bridge"
	"copynote/internal/macui"
)

// Releases carry only the Windows executable so far, so the macOS build
// reports no update rather than offer one it cannot install. The bindings
// still exist: the Settings view calls them on every platform.
func bindUpdates(view *macui.WebView, updates *bridge.Async) {
	none := func(context.Context) (any, error) { return nil, nil }
	for _, name := range []string{"checkForUpdates", "forceCheckForUpdates"} {
		if err := updates.Bind(name, none); err != nil {
			panic(err)
		}
	}
	errNoUpdates := errors.New("updates are not available for macOS yet")
	if err := updates.Bind("installUpdate", func(context.Context) (any, error) { return nil, errNoUpdates }); err != nil {
		panic(err)
	}
	mustBind(view, "updateProgress", func() map[string]any {
		return map[string]any{"stage": "", "done": 0, "total": 0}
	})
	mustBind(view, "restartApp", func() error { return errNoUpdates })
}

package main

import (
	"errors"
	"log"
	"path/filepath"
	"sync/atomic"

	"github.com/jchv/go-webview2"

	"copynote/internal/model"
	"copynote/internal/relocate"
	"copynote/internal/service"
	"copynote/internal/winutil"
)

// relaunchTarget is the executable main should start on the way out. A
// self-update replaces the file in place and leaves this empty; a move
// puts the new copy's path here, because the running file is about to
// stop being the one worth starting.
var relaunchTarget atomic.Pointer[string]

// installLocation is the JSON shape the page reads to decide whether to
// offer the move.
type installLocation struct {
	// Path is the running executable, Dir the folder holding it.
	Path string `json:"path"`
	Dir  string `json:"dir"`
	// Permanent is true when Dir is already a program folder, which is
	// what hides the offer.
	Permanent bool `json:"permanent"`
	// DefaultDir is where the one-click move puts it.
	DefaultDir string `json:"defaultDir"`
	// CanRelocate is false when the executable path could not be resolved
	// at startup, which makes every move impossible.
	CanRelocate bool `json:"canRelocate"`
}

func bindRelocate(w webview2.WebView, svc *service.Service, hwnd uintptr, exePath, dataDir string) {
	mustBind := func(name string, fn any) {
		if err := w.Bind(name, fn); err != nil {
			log.Fatalf("bind %s: %v", name, err)
		}
	}

	mustBind("getInstallLocation", func() installLocation {
		loc := installLocation{
			Path:        exePath,
			Permanent:   relocate.IsPermanent(exePath),
			CanRelocate: exePath != "",
		}
		if exePath != "" {
			loc.Dir = filepath.Dir(exePath)
		}
		if dir, err := relocate.DefaultDir(); err == nil {
			loc.DefaultDir = dir
		} else {
			// Without LOCALAPPDATA there is no default destination, but the
			// user can still pick a folder themselves.
			log.Printf("default install folder: %v", err)
		}
		return loc
	})

	// Returns "" when the user cancels, which the page treats as "do nothing".
	mustBind("pickInstallFolder", func(title string) string {
		dir, ok := winutil.PickFolder(hwnd, title)
		if !ok {
			return ""
		}
		return dir
	})

	// relocateApp copies the running executable into targetDir (empty means
	// the default folder), records the file left behind for the new copy to
	// delete, and asks main to quit and start that copy. The original is
	// deliberately still on disk when this returns: if the new copy fails to
	// start, the user's existing file still works.
	mustBind("relocateApp", func(targetDir string) (string, error) {
		if exePath == "" {
			return "", errors.New("executable path is unavailable")
		}
		if targetDir == "" {
			dir, err := relocate.DefaultDir()
			if err != nil {
				return "", err
			}
			targetDir = dir
		}
		newPath, err := relocate.CopyTo(exePath, targetDir)
		if err != nil {
			log.Printf("relocate to %s failed: %v", targetDir, err)
			return "", err
		}
		if err := relocate.MarkPending(dataDir, exePath); err != nil {
			// The copy is in place and usable; only the cleanup of the old
			// file is lost, so this is worth a log and nothing more.
			log.Printf("mark old executable for cleanup: %v", err)
		}
		// The banner has served its purpose either way.
		if err := svc.UpdateSettings(func(s *model.Settings) { s.RelocatePromptDismissed = true }); err != nil {
			log.Printf("dismiss relocate prompt: %v", err)
		}
		log.Printf("relocated to %s; restarting", newPath)
		relaunchTarget.Store(&newPath)
		restartRequested.Store(true)
		quitting.Store(true)
		w.Dispatch(func() { w.Terminate() })
		return newPath, nil
	})

	mustBind("dismissRelocatePrompt", func() error {
		return svc.UpdateSettings(func(s *model.Settings) { s.RelocatePromptDismissed = true })
	})
}

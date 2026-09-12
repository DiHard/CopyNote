package main

import (
	"context"
	"errors"
	"log"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jchv/go-webview2"

	"copynote/internal/bridge"
	"copynote/internal/service"
	"copynote/internal/tray"
	"copynote/internal/updater"
	"copynote/internal/version"
)

// Self-update state shared between the bridge bindings and main's shutdown
// path. One install runs at a time; a successful install flips updateReady,
// and restartApp then asks main to relaunch the (already replaced) file.
var (
	latestRelease    atomic.Pointer[updater.ReleaseInfo]
	installing       atomic.Bool
	updateReady      atomic.Bool
	restartRequested atomic.Bool

	progressMu      sync.Mutex
	installProgress updateProgress
)

// updateProgress is the JSON shape the page polls during installUpdate.
type updateProgress struct {
	Stage string `json:"stage"` // "", "download", "verify" or "apply"
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}

// updateInfo is ReleaseInfo plus whether this installation can replace
// itself: the release must ship a signed binary and the exe directory
// must be writable without elevation.
type updateInfo struct {
	*updater.ReleaseInfo
	SelfUpdate bool `json:"selfUpdate"`
}

// installPromiseTimeout is the page-side limit for installUpdate; the Go
// side bounds the download separately (updater.Install).
const installPromiseTimeout = 10 * time.Minute

func bindUpdates(w webview2.WebView, updates *bridge.Async, svc *service.Service, exePath string) {
	mustBind := func(name string, fn any) {
		if err := w.Bind(name, fn); err != nil {
			log.Fatalf("bind %s: %v", name, err)
		}
	}
	mustAsync := func(name string, timeout time.Duration, fn func(context.Context) (any, error)) {
		if err := updates.BindTimeout(name, timeout, fn); err != nil {
			log.Fatalf("bind %s: %v", name, err)
		}
	}

	check := func(ctx context.Context) (any, error) {
		info, err := updater.CheckLatest(ctx, version.Version)
		if err != nil || info == nil {
			return nil, err
		}
		latestRelease.Store(info)
		return &updateInfo{
			ReleaseInfo: info,
			SelfUpdate:  info.Installable() && len(updater.PublicKey) > 0 && updater.CanSelfUpdate(exePath),
		}, nil
	}
	mustAsync("checkForUpdates", bridge.DefaultTimeout, func(ctx context.Context) (any, error) {
		settings, err := svc.GetSettings()
		if err != nil {
			return nil, err
		}
		if settings.DisableUpdateCheck {
			return nil, nil
		}
		return check(ctx)
	})
	mustAsync("forceCheckForUpdates", bridge.DefaultTimeout, check)

	mustAsync("installUpdate", installPromiseTimeout, func(ctx context.Context) (any, error) {
		info := latestRelease.Load()
		if info == nil {
			return nil, errors.New("no update is available")
		}
		if updateReady.Load() {
			return nil, errors.New("an update is already installed; restart to finish")
		}
		if !installing.CompareAndSwap(false, true) {
			return nil, errors.New("an update is already being installed")
		}
		defer installing.Store(false)

		setProgress(updater.StageDownload, 0, info.Size)
		err := updater.Install(ctx, info, exePath, version.Version, setProgress)
		setProgress("", 0, 0)
		if err != nil {
			log.Printf("self-update to %s failed: %v", info.Version, err)
			return nil, err
		}
		updateReady.Store(true)
		log.Printf("self-update to %s installed; restart pending", info.Version)
		return map[string]string{"version": info.Version}, nil
	})

	mustBind("updateProgress", func() updateProgress {
		progressMu.Lock()
		defer progressMu.Unlock()
		return installProgress
	})

	mustBind("restartApp", func() error {
		if !updateReady.Load() {
			return errors.New("no update has been installed")
		}
		restartRequested.Store(true)
		quitting.Store(true)
		w.Dispatch(func() { w.Terminate() })
		return nil
	})
}

func setProgress(stage updater.Stage, done, total int64) {
	progressMu.Lock()
	installProgress = updateProgress{Stage: string(stage), Done: done, Total: total}
	progressMu.Unlock()
}

// relaunch starts the replaced executable and asks it to show its window,
// the same way a second launch would. main calls it after the tray has
// shut down and the single-instance mutex has been released — otherwise
// the new process would see a running instance and exit.
func relaunch(exePath string) {
	cmd := exec.Command(exePath)
	cmd.Dir = filepath.Dir(exePath)
	if err := cmd.Start(); err != nil {
		log.Printf("relaunch after update: %v", err)
		return
	}
	log.Printf("relaunched %s as pid %d", exePath, cmd.Process.Pid)
	_ = cmd.Process.Release()
	// The new instance is still starting WebView2; the tray holds the
	// request until the UI is ready, so the updated window comes up.
	delivered := tray.ShowRunningInstance(15 * time.Second)
	log.Printf("show request delivered to the new instance: %v", delivered)
}

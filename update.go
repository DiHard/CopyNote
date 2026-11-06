package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"copynote/internal/bridge"
	"copynote/internal/service"
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
// itself: the release must ship a signed binary and the folder holding the
// installation must be writable without elevation.
type updateInfo struct {
	*updater.ReleaseInfo
	SelfUpdate bool `json:"selfUpdate"`
}

// installPromiseTimeout is the page-side limit for installUpdate; the Go
// side bounds the download separately (updater.Install).
const installPromiseTimeout = 10 * time.Minute

// bindUpdates binds the update checks and the install. target is what an
// update replaces — the executable on Windows, the .app bundle on macOS,
// "" when it is unknown — and stop ends the run so main can relaunch.
func bindUpdates(b binder, updates *bridge.Async, svc *service.Service, target string, stop func()) {
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
			SelfUpdate:  info.Installable() && len(updater.PublicKey) > 0 && updater.CanSelfUpdate(target),
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
		err := updater.Install(ctx, info, target, version.Version, setProgress)
		setProgress("", 0, 0)
		if err != nil {
			log.Printf("self-update to %s failed: %v", info.Version, err)
			return nil, err
		}
		updateReady.Store(true)
		log.Printf("self-update to %s installed; restart pending", info.Version)
		return map[string]string{"version": info.Version}, nil
	})

	mustBind(b, "updateProgress", func() updateProgress {
		progressMu.Lock()
		defer progressMu.Unlock()
		return installProgress
	})

	mustBind(b, "restartApp", func() error {
		if !updateReady.Load() {
			return errors.New("no update has been installed")
		}
		restartRequested.Store(true)
		quitting.Store(true)
		stop()
		return nil
	})
}

func setProgress(stage updater.Stage, done, total int64) {
	progressMu.Lock()
	installProgress = updateProgress{Stage: string(stage), Done: done, Total: total}
	progressMu.Unlock()
}

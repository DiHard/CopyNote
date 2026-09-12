package updater

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Stage names reported through InstallProgress.
type Stage string

const (
	StageDownload Stage = "download"
	StageVerify   Stage = "verify"
	StageApply    Stage = "apply"
)

// InstallProgress receives stage changes and download progress; done and
// total are only meaningful during StageDownload, total is 0 while unknown.
type InstallProgress func(stage Stage, done, total int64)

// installTimeout bounds the whole install so a stalled connection cannot
// keep the UI in "downloading" forever. Slow links still get minutes.
const installTimeout = 5 * time.Minute

// Install downloads the release binary next to exePath, verifies its
// signature against the embedded key and swaps it in. When it returns nil
// the file at exePath is the new version and the caller should restart the
// process. On any error the running version stays in place and the
// download is removed.
func Install(ctx context.Context, info *ReleaseInfo, exePath, currentVersion string, progress InstallProgress) error {
	return install(ctx, http.DefaultClient, PublicKey, info, exePath, currentVersion, progress)
}

func install(ctx context.Context, client *http.Client, pub ed25519.PublicKey, info *ReleaseInfo, exePath, currentVersion string, progress InstallProgress) (err error) {
	if !info.Installable() {
		return errors.New("release has no signed binary")
	}
	if len(pub) != ed25519.PublicKeySize {
		return errors.New("no release signing key is embedded in this build")
	}
	if exePath == "" {
		return errors.New("executable path is unknown")
	}
	if progress == nil {
		progress = func(Stage, int64, int64) {}
	}

	ctx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	defer func() {
		if err != nil {
			RemoveStaging(exePath)
		}
	}()

	ua := userAgent(currentVersion)
	progress(StageDownload, 0, info.Size)
	// The signature is tiny; fetching it first fails fast on a broken
	// release instead of after the whole binary.
	sig, err := fetchSignature(ctx, client, info.SignatureURL, ua)
	if err != nil {
		return fmt.Errorf("download signature: %w", err)
	}
	if err := download(ctx, client, info.DownloadURL, StagingPath(exePath), info.Size, ua, func(done, total int64) {
		progress(StageDownload, done, total)
	}); err != nil {
		return err
	}
	progress(StageVerify, 0, 0)
	if err := verifyFile(pub, StagingPath(exePath), info.Version, sig); err != nil {
		return err
	}
	progress(StageApply, 0, 0)
	return Apply(exePath)
}

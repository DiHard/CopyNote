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
//
// exePath is what gets replaced: the executable on Windows, the .app
// bundle on macOS (see the package's platform files).
func Install(ctx context.Context, info *ReleaseInfo, exePath, currentVersion string, progress InstallProgress) error {
	return install(ctx, http.DefaultClient, PublicKey, info, exePath, currentVersion, progress, platformPackaging)
}

// packaging says how a release asset turns into what replaces the
// installation. The asset is saved at download(target); unpack, when set,
// turns it into StagingPath(target). Without unpack the asset is saved at
// the staging path directly and is the new version as it stands.
type packaging struct {
	download func(target string) string
	unpack   func(download, staging string) error
}

// plainFile is a release asset that is the program itself.
var plainFile = packaging{download: StagingPath}

func install(ctx context.Context, client *http.Client, pub ed25519.PublicKey, info *ReleaseInfo, exePath, currentVersion string, progress InstallProgress, pkg packaging) (err error) {
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
	downloaded := pkg.download(exePath)
	defer func() {
		_ = removeIfExists(downloaded) // unpacked by now, or failed
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
	if err := download(ctx, client, info.DownloadURL, downloaded, info.Size, ua, func(done, total int64) {
		progress(StageDownload, done, total)
	}); err != nil {
		return err
	}
	progress(StageVerify, 0, 0)
	if err := verifyFile(pub, downloaded, info.Version, sig); err != nil {
		return err
	}
	progress(StageApply, 0, 0)
	if pkg.unpack != nil {
		if err := pkg.unpack(downloaded, StagingPath(exePath)); err != nil {
			return fmt.Errorf("unpack: %w", err)
		}
	}
	return Apply(exePath)
}

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Release asset names for macOS: the .app bundle zipped with ditto, which
// keeps the bundle's code signature and file modes intact, and the
// detached signature over that zip.
const (
	AssetName          = "CopyNote-macos.zip"
	SignatureAssetName = "CopyNote-macos.zip.sig"
)

// The target is the .app bundle. The zip is saved beside it and unpacked
// into StagingPath, so the swap is two renames within one folder.
var platformPackaging = packaging{
	download: func(bundle string) string { return bundle + ".zip" + stagingSuffix },
	unpack:   unpackBundle,
}

// bundleExecutable is the binary every CopyNote bundle carries.
const bundleExecutable = "Contents/MacOS/CopyNote"

// unpackBundle extracts the one .app in archive and moves it to staging.
func unpackBundle(archive, staging string) error {
	scratch := staging + ".unpack"
	if err := removeIfExists(scratch); err != nil {
		return err
	}
	defer func() { _ = removeIfExists(scratch) }()
	if out, err := exec.Command("/usr/bin/ditto", "-x", "-k", archive, scratch).CombinedOutput(); err != nil {
		return fmt.Errorf("ditto: %w: %s", err, strings.TrimSpace(string(out)))
	}
	entries, err := os.ReadDir(scratch)
	if err != nil {
		return err
	}
	var apps []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			apps = append(apps, e.Name())
		}
	}
	if len(apps) != 1 {
		return fmt.Errorf("archive holds %d app bundles, want 1", len(apps))
	}
	app := filepath.Join(scratch, apps[0])
	if info, err := os.Stat(filepath.Join(app, bundleExecutable)); err != nil || !info.Mode().IsRegular() {
		return errors.New("archive is not a CopyNote bundle")
	}
	if err := removeIfExists(staging); err != nil {
		return err
	}
	return os.Rename(app, staging)
}

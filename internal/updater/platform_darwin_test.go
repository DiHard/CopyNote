package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"copynote/internal/testutil"
)

// makeBundleZip builds a minimal CopyNote.app and zips it the way the
// release does, with ditto --keepParent.
func makeBundleZip(t *testing.T, dir, content string) string {
	t.Helper()
	app := filepath.Join(dir, "src", "CopyNote.app")
	exe := filepath.Join(app, bundleExecutable)
	if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	zip := filepath.Join(dir, AssetName)
	if out, err := exec.Command("/usr/bin/ditto", "-c", "-k", "--keepParent", app, zip).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v: %s", err, out)
	}
	return zip
}

func TestUnpackAndApplyBundle(t *testing.T) {
	dir := testutil.TempDir(t)
	zip := makeBundleZip(t, dir, "new")

	// The installed bundle, as it sits in /Applications.
	bundle := filepath.Join(dir, "Applications", "CopyNote.app")
	old := filepath.Join(bundle, bundleExecutable)
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := unpackBundle(zip, StagingPath(bundle)); err != nil {
		t.Fatal(err)
	}
	if err := Apply(bundle); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(bundle, bundleExecutable)
	if got := readFile(t, exe); got != "new" {
		t.Fatalf("installed executable = %q", got)
	}
	if info, err := os.Stat(exe); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("installed executable lost its mode: %v %v", info, err)
	}
	if got := readFile(t, filepath.Join(PreviousPath(bundle), bundleExecutable)); got != "old" {
		t.Fatalf("previous version = %q", got)
	}
	if err := RemovePrevious(bundle, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PreviousPath(bundle)); !os.IsNotExist(err) {
		t.Fatalf("previous bundle left behind: %v", err)
	}
}

func TestUnpackRejectsAForeignArchive(t *testing.T) {
	dir := testutil.TempDir(t)
	src := filepath.Join(dir, "src", "Other.app", "Contents")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	zip := filepath.Join(dir, "other.zip")
	if out, err := exec.Command("/usr/bin/ditto", "-c", "-k", "--keepParent", filepath.Dir(src), zip).CombinedOutput(); err != nil {
		t.Fatalf("ditto: %v: %s", err, out)
	}
	staging := filepath.Join(dir, "CopyNote.app.new")
	if err := unpackBundle(zip, staging); err == nil {
		t.Fatal("accepted a bundle without the CopyNote executable")
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging left behind: %v", err)
	}
	if _, err := os.Stat(staging + ".unpack"); !os.IsNotExist(err) {
		t.Fatalf("scratch folder left behind: %v", err)
	}
}

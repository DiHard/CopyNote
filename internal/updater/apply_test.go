package updater

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"copynote/internal/testutil"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fastRetries(t *testing.T) {
	t.Helper()
	previous := retryDelay
	retryDelay = time.Millisecond
	t.Cleanup(func() { retryDelay = previous })
}

func TestApplySwapsBinaryAndKeepsPrevious(t *testing.T) {
	exe := filepath.Join(testutil.TempDir(t), "copynote.exe")
	writeFile(t, exe, "running version")
	writeFile(t, StagingPath(exe), "new version")
	writeFile(t, PreviousPath(exe), "stale leftover from an older update")

	if err := Apply(exe); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != "new version" {
		t.Fatalf("exe = %q", got)
	}
	if got := readFile(t, PreviousPath(exe)); got != "running version" {
		t.Fatalf("previous = %q", got)
	}
	if _, err := os.Stat(StagingPath(exe)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging left behind: %v", err)
	}
	if err := RemovePrevious(exe, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PreviousPath(exe)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous not removed: %v", err)
	}
}

func TestApplyRequiresStagedBinary(t *testing.T) {
	exe := filepath.Join(testutil.TempDir(t), "copynote.exe")
	writeFile(t, exe, "running version")
	if err := Apply(exe); err == nil {
		t.Fatal("swap without a staged binary succeeded")
	}
	if got := readFile(t, exe); got != "running version" {
		t.Fatalf("exe = %q", got)
	}
}

func TestApplyRestoresRunningVersionWhenSwapFails(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("relies on Windows sharing violations")
	}
	fastRetries(t)
	exe := filepath.Join(testutil.TempDir(t), "copynote.exe")
	writeFile(t, exe, "running version")
	writeFile(t, StagingPath(exe), "new version")
	// An open handle without FILE_SHARE_DELETE (what a scanner holds) makes
	// the second rename fail after the running version was moved aside.
	held, err := os.Open(StagingPath(exe))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	if err := Apply(exe); err == nil {
		t.Fatal("swap succeeded despite the locked staged binary")
	}
	if got := readFile(t, exe); got != "running version" {
		t.Fatalf("exe after rollback = %q", got)
	}
	if _, err := os.Stat(PreviousPath(exe)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("previous left behind after rollback: %v", err)
	}
}

// The whole scheme rests on Windows allowing a running image to be renamed
// while refusing to delete it, so exercise it on the test binary itself.
func TestApplyOnRunningExecutable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific file locking")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Copy the image through handles that are closed before the swap: a Go
	// os.Open handle lacks FILE_SHARE_DELETE and would itself block the rename.
	src, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	dst, err := os.Create(StagingPath(exe))
	if err != nil {
		src.Close()
		t.Fatal(err)
	}
	_, err = io.Copy(dst, src)
	src.Close()
	dst.Close()
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(exe); err != nil {
		os.Remove(StagingPath(exe))
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Put the running image back under its own name.
		os.Remove(exe)
		if err := os.Rename(PreviousPath(exe), exe); err != nil {
			t.Error(err)
		}
	})
	if _, err := os.Stat(PreviousPath(exe)); err != nil {
		t.Fatalf("running image was not moved aside: %v", err)
	}
	if err := RemovePrevious(exe, 0); err == nil {
		t.Fatal("deleting the running image succeeded; Windows semantics changed")
	}
}

func TestCanSelfUpdate(t *testing.T) {
	exe := filepath.Join(testutil.TempDir(t), "copynote.exe")
	if !CanSelfUpdate(exe) {
		t.Fatal("writable directory reported as read-only")
	}
	if CanSelfUpdate("") {
		t.Fatal("unknown executable path reported as updatable")
	}
	if CanSelfUpdate(filepath.Join(exe, "missing", "copynote.exe")) {
		t.Fatal("missing directory reported as updatable")
	}
}

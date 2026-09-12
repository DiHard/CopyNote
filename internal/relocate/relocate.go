// Package relocate moves the running executable out of a transient
// download folder into a permanent per-user program folder.
//
// A portable exe that keeps living in %USERPROFILE%\Downloads is one
// Storage Sense sweep away from disappearing, and the autorun entry in
// the registry points straight at it. Moving it somewhere durable is the
// one action that fixes both.
//
// The move never deletes the original: the freshly started copy does
// that, through the marker this package leaves behind. If the new copy
// fails to start, the file the user already had is still there.
package relocate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ExeName is the file name the application is published under. A browser
// that downloads it twice produces "copynote (1).exe"; moving normalises
// the name so the install folder holds one predictable executable.
const ExeName = "copynote.exe"

// pendingName is the marker left in the data folder naming the executable
// the next start should delete.
const pendingName = "pending-cleanup"

// DefaultDir is the per-user program folder, %LOCALAPPDATA%\Programs\CopyNote.
// Writing there needs no administrator rights, and it is where per-user
// installs of other applications already live.
func DefaultDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", errors.New("LOCALAPPDATA is not set")
	}
	return filepath.Join(local, "Programs", "CopyNote"), nil
}

// IsPermanent reports whether exePath already sits in a folder meant to
// hold installed programs. Everything else — Downloads, the desktop, a
// temp folder, a USB stick — counts as transient and is worth moving out
// of. A folder the user picked themselves is not recognised here; the
// caller records that choice instead.
func IsPermanent(exePath string) bool {
	if exePath == "" {
		return false
	}
	dir := filepath.Dir(exePath)
	for _, root := range permanentRoots() {
		if within(root, dir) {
			return true
		}
	}
	return false
}

func permanentRoots() []string {
	var roots []string
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		roots = append(roots, filepath.Join(local, "Programs"))
	}
	// ProgramW6432 is the 64-bit Program Files as seen by a 32-bit process;
	// harmless to include and correct if the build ever goes 32-bit.
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		if v := os.Getenv(env); v != "" {
			roots = append(roots, v)
		}
	}
	return roots
}

// within reports whether path is root or sits below it. Windows paths are
// compared case-insensitively.
func within(root, path string) bool {
	r, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	p, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	r = strings.ToLower(filepath.Clean(r))
	p = strings.ToLower(filepath.Clean(p))
	return p == r || strings.HasPrefix(p, r+string(filepath.Separator))
}

// samePath reports whether two paths name the same location.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if fa, err := os.Stat(a); err == nil {
		if fb, err := os.Stat(b); err == nil {
			return os.SameFile(fa, fb)
		}
	}
	aa, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
}

// CopyTo places a copy of the running executable in targetDir under
// ExeName and returns its path. The copy is staged under a temporary name
// and renamed into place, so an interrupted copy never leaves a
// half-written executable that someone could start.
func CopyTo(exePath, targetDir string) (string, error) {
	if exePath == "" {
		return "", errors.New("executable path is unavailable")
	}
	if targetDir == "" {
		return "", errors.New("no target folder given")
	}
	target := filepath.Join(targetDir, ExeName)
	if samePath(exePath, target) {
		return "", fmt.Errorf("%s is already in %s", ExeName, targetDir)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", targetDir, err)
	}

	src, err := os.Open(exePath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", exePath, err)
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", exePath, err)
	}

	tmp, err := os.CreateTemp(targetDir, ".copynote-move-*")
	if err != nil {
		return "", fmt.Errorf("stage in %s: %w", targetDir, err)
	}
	staged := tmp.Name()
	written, copyErr := io.Copy(tmp, src)
	if copyErr == nil {
		copyErr = tmp.Sync()
	}
	closeErr := tmp.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil && written != info.Size() {
		copyErr = fmt.Errorf("copied %d of %d bytes", written, info.Size())
	}
	if copyErr == nil {
		copyErr = os.Chmod(staged, 0o755)
	}
	if copyErr != nil {
		os.Remove(staged)
		return "", fmt.Errorf("copy to %s: %w", targetDir, copyErr)
	}

	// Rename replaces an existing file. A copy already sitting at the
	// target cannot be the running one — the single-instance mutex would
	// have stopped this process before it got here.
	if err := os.Rename(staged, target); err != nil {
		os.Remove(staged)
		return "", fmt.Errorf("install into %s: %w", targetDir, err)
	}
	return target, nil
}

// MarkPending records the executable left behind by a move so the next
// start can delete it.
func MarkPending(dataDir, leftover string) error {
	if dataDir == "" {
		return errors.New("no data folder given")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, pendingName), []byte(leftover), 0o644)
}

// RunPendingCleanup deletes the executable named by the marker, retrying
// while the process that wrote it is still exiting and holding the file
// open. The marker is dropped either way: a leftover that cannot be
// removed is not worth retrying on every future start.
func RunPendingCleanup(dataDir, currentExe string, wait time.Duration) error {
	if dataDir == "" {
		return nil
	}
	marker := filepath.Join(dataDir, pendingName)
	raw, err := os.ReadFile(marker)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer os.Remove(marker)

	leftover := strings.TrimSpace(string(raw))
	if !safeToDelete(leftover, currentExe) {
		return fmt.Errorf("refusing to delete %q named by %s", leftover, pendingName)
	}
	deadline := time.Now().Add(wait)
	for {
		err := os.Remove(leftover)
		if err == nil || errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("remove %s: %w", leftover, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// safeToDelete gates the marker. It is a file on disk, so it is treated as
// untrusted input: only an absolute path to one of this application's own
// executables qualifies, and never the executable running right now.
func safeToDelete(path, currentExe string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	if !strings.EqualFold(filepath.Ext(path), ".exe") {
		return false
	}
	if !strings.HasPrefix(strings.ToLower(filepath.Base(path)), "copynote") {
		return false
	}
	return !samePath(path, currentExe)
}

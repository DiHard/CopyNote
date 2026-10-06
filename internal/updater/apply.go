package updater

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Companion files next to the running executable:
//
//	copynote.exe.new  download in progress, then the verified binary awaiting the swap
//	copynote.exe.old  the previous version, kept until the new one has started
const (
	stagingSuffix  = ".new"
	previousSuffix = ".old"
)

// StagingPath is where the downloaded binary is written before the swap.
func StagingPath(exePath string) string { return exePath + stagingSuffix }

// PreviousPath is where the running binary is moved during the swap.
func PreviousPath(exePath string) string { return exePath + previousSuffix }

// retryDelay separates attempts of a rename or delete that a virus scanner
// or sync client may be blocking for a moment. Shortened by tests.
var retryDelay = 200 * time.Millisecond

const retryAttempts = 10

// probePrefix names the empty file CanSelfUpdate creates to test the folder.
const probePrefix = ".copynote-update-"

// CanSelfUpdate reports whether the directory holding exePath accepts new
// files, i.e. whether a swap can succeed without elevation. False for an
// installation under Program Files or on read-only media.
func CanSelfUpdate(exePath string) bool {
	if exePath == "" {
		return false
	}
	f, err := os.CreateTemp(filepath.Dir(exePath), probePrefix+"*")
	if err != nil {
		return false
	}
	// The folder took a new file, and that was the whole question. Clearing
	// the probe away is housekeeping: a virus scanner holding the fresh file
	// for a moment must not turn the answer into "no" and take the in-place
	// update away. It gets the same patience as the swap itself, and
	// RemoveStaging sweeps up whatever is still there on the next start.
	name := f.Name()
	_ = f.Close()
	_ = retry(func() error { return removeIfExists(name) })
	return true
}

// Apply swaps the verified binary at StagingPath(exePath) in for the
// running executable. Windows refuses to overwrite or delete a running
// image but allows renaming it, so the current file is moved to
// PreviousPath first. If the second rename fails the running version is
// moved back so the install path is never left empty.
func Apply(exePath string) error {
	staging, previous := StagingPath(exePath), PreviousPath(exePath)
	if _, err := os.Stat(staging); err != nil {
		return fmt.Errorf("staged binary: %w", err)
	}
	// A leftover from an earlier update may still be held by a process
	// that is exiting, so removal gets a few attempts too.
	if err := retry(func() error { return removeIfExists(previous) }); err != nil {
		return fmt.Errorf("remove previous version: %w", err)
	}
	if err := retry(func() error { return os.Rename(exePath, previous) }); err != nil {
		return fmt.Errorf("move running version aside: %w", err)
	}
	if err := retry(func() error { return os.Rename(staging, exePath) }); err != nil {
		if back := retry(func() error { return os.Rename(previous, exePath) }); back != nil {
			return fmt.Errorf("install new version: %w (restoring the running version failed too: %w)", err, back)
		}
		return fmt.Errorf("install new version: %w", err)
	}
	return nil
}

// RemoveStaging deletes an unfinished or unverified download, if any, and
// any probe file CanSelfUpdate could not clear away at the time.
func RemoveStaging(exePath string) {
	_ = removeIfExists(StagingPath(exePath))
	dir := filepath.Dir(exePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), probePrefix) {
			_ = removeIfExists(filepath.Join(dir, entry.Name()))
		}
	}
}

// RemovePrevious deletes the version left behind by Apply. The process
// that ran it may still be exiting and holding the file, so the attempt
// is repeated once a second for up to wait.
func RemovePrevious(exePath string, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		err := removeIfExists(PreviousPath(exePath))
		if err == nil || !time.Now().Before(deadline) {
			return err
		}
		time.Sleep(time.Second)
	}
}

// removeFile is os.Remove, replaced by tests that need a deletion to fail.
var removeFile = os.Remove

func removeIfExists(path string) error {
	err := removeFile(path)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func retry(op func() error) error {
	var err error
	for attempt := 0; attempt < retryAttempts; attempt++ {
		if err = op(); err == nil {
			return nil
		}
		time.Sleep(retryDelay)
	}
	return err
}

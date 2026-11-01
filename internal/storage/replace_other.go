//go:build !windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
)

// replaceFile moves from over to. A POSIX rename replaces the destination
// atomically; syncing the directory afterwards makes the rename itself
// durable, the counterpart of MOVEFILE_WRITE_THROUGH on Windows.
func replaceFile(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(to))
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

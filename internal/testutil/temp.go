// Package testutil contains helpers for tests that exercise Windows file I/O.
package testutil

import (
	"os"
	"testing"
	"time"
)

// TempDir retries cleanup briefly when Windows indexing or antivirus software
// temporarily retains a file. Persistent cleanup errors still fail the test.
func TempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() {
		var err error
		for attempt := 0; attempt < 10; attempt++ {
			if err = os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Errorf("clean temporary directory: %v", err)
	})
	return dir
}

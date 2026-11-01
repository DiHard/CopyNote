package singleton

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Unix socket paths are limited to 104 bytes, more than t.TempDir() leaves
// room for on macOS.
func scratch(t *testing.T) {
	t.Helper()
	d, err := os.MkdirTemp("/tmp", "cn")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	old := dir
	dir = func() (string, error) { return d, nil }
	t.Cleanup(func() { dir = old })
}

func TestAcquireIsExclusiveUntilReleased(t *testing.T) {
	scratch(t)
	release, already, err := Acquire("test")
	if err != nil || already {
		t.Fatalf("first Acquire: already=%v err=%v", already, err)
	}
	if _, already, err := Acquire("test"); err != nil || !already {
		t.Fatalf("second Acquire while held: already=%v err=%v", already, err)
	}
	release()
	release2, already, err := Acquire("test")
	if err != nil || already {
		t.Fatalf("Acquire after release: already=%v err=%v", already, err)
	}
	release2()
}

func TestShowRunningReachesTheListener(t *testing.T) {
	scratch(t)
	shown := make(chan struct{}, 1)
	stop, err := Listen("test", func() { shown <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if !ShowRunning("test", time.Second) {
		t.Fatal("request not delivered")
	}
	select {
	case <-shown:
	case <-time.After(time.Second):
		t.Fatal("listener did not hear the request")
	}
}

func TestShowRunningGivesUpWithoutAListener(t *testing.T) {
	scratch(t)
	d, _ := dir()
	if _, err := os.Stat(filepath.Join(d, "test.sock")); err == nil {
		t.Fatal("unexpected socket")
	}
	if ShowRunning("test", 200*time.Millisecond) {
		t.Fatal("delivered to nobody")
	}
}

func TestALongFolderStillGetsAUsableSocket(t *testing.T) {
	scratch(t)
	d, _ := dir()
	long := filepath.Join(d, strings.Repeat("x", 90))
	dir = func() (string, error) { return long, nil }
	_, socket, err := paths("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(socket) > maxSocketPath {
		t.Fatalf("socket path is %d bytes: %s", len(socket), socket)
	}
	stop, err := Listen("test", func() {})
	if err != nil {
		t.Fatal(err)
	}
	stop()
}

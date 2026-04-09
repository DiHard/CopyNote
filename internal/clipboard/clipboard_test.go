//go:build windows && clipboard_real

// These tests touch the live system clipboard. They are gated behind
// the "clipboard_real" build tag so a normal `go test ./...` does not
// stomp on the developer's clipboard. Run manually with:
//
//     go test -tags clipboard_real ./internal/clipboard/
package clipboard

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWriteText_RoundTrip_ASCII(t *testing.T) { roundTrip(t, "hello world") }

func TestWriteText_RoundTrip_Unicode(t *testing.T) { roundTrip(t, "Привет 🌍") }

func TestWriteText_RoundTrip_MultiLine(t *testing.T) {
	roundTrip(t, "line1\nline2\r\nline3")
}

func TestWriteText_Empty(t *testing.T) { roundTrip(t, "") }

func roundTrip(t *testing.T, s string) {
	t.Helper()
	if err := WriteText(s); err != nil {
		t.Fatalf("WriteText(%q): %v", s, err)
	}
	got, err := readText(t)
	if err != nil {
		t.Fatalf("readText: %v", err)
	}
	if got != s {
		t.Errorf("round-trip mismatch:\n want=%q\n  got=%q", s, got)
	}
}

// readText is a test-only helper that reads CF_UNICODETEXT back from
// the clipboard. It is intentionally not part of the package's public
// API — production code only writes.
func readText(t *testing.T) (string, error) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := openClipboardWithRetry(); err != nil {
		return "", err
	}
	defer closeClipboard()

	hMem, _, callErr := procGetClipboardData.Call(uintptr(cfUnicodeText))
	if hMem == 0 {
		return "", callErr
	}
	ptr, _, lockErr := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return "", lockErr
	}
	defer procGlobalUnlock.Call(hMem)

	sizeBytes, _, _ := procGlobalSize.Call(hMem)
	if sizeBytes == 0 {
		return "", nil
	}
	// Copy locked memory into a Go-managed buffer via RtlMoveMemory,
	// avoiding any unsafe.Pointer(uintptr) inversion that would upset
	// vet (and is genuinely unsound under Go's GC rules).
	n := int(sizeBytes / 2)
	buf := make([]uint16, n)
	_, _, _ = procRtlMoveMemory.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		ptr,
		sizeBytes,
	)
	// Strip trailing NULs added by SetClipboardData / source apps.
	for len(buf) > 0 && buf[len(buf)-1] == 0 {
		buf = buf[:len(buf)-1]
	}
	return windows.UTF16ToString(buf), nil
}

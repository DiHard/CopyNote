//go:build clipboard_real

// Touches the live pasteboard, like the Windows test of the same tag:
//
//	go test -tags clipboard_real ./internal/clipboard/
package clipboard

import (
	"os/exec"
	"testing"
)

func TestWriteTextRoundTripsUnicode(t *testing.T) {
	const want = "Привет, мир — 👋"
	if err := WriteText(want); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/usr/bin/pbpaste")
	cmd.Env = []string{"LANG=en_US.UTF-8"}
	got, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("pasteboard holds %q, want %q", got, want)
	}
}

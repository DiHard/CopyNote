package relocate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"copynote/internal/testutil"
)

// writeExe creates a stand-in executable with known content.
func writeExe(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestIsPermanent(t *testing.T) {
	local := testutil.TempDir(t)
	elsewhere := testutil.TempDir(t)
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("ProgramFiles", filepath.Join(local, "PF"))
	t.Setenv("ProgramFiles(x86)", "")
	t.Setenv("ProgramW6432", "")

	cases := []struct {
		name string
		exe  string
		want bool
	}{
		{"installed per-user", filepath.Join(local, "Programs", "CopyNote", "copynote.exe"), true},
		{"nested below Programs", filepath.Join(local, "Programs", "Other", "sub", "copynote.exe"), true},
		{"program files", filepath.Join(local, "PF", "CopyNote", "copynote.exe"), true},
		{"downloads", filepath.Join(elsewhere, "Downloads", "copynote.exe"), false},
		{"empty path", "", false},
		// A sibling folder whose name merely starts with "Programs" must not
		// pass as being inside it.
		{"prefix lookalike", filepath.Join(local, "ProgramsOld", "copynote.exe"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPermanent(tc.exe); got != tc.want {
				t.Errorf("IsPermanent(%q) = %v, want %v", tc.exe, got, tc.want)
			}
		})
	}
}

func TestCopyToPlacesExecutable(t *testing.T) {
	src := testutil.TempDir(t)
	dst := filepath.Join(testutil.TempDir(t), "Programs", "CopyNote")
	// A browser's second download arrives under a decorated name; the move
	// is what normalises it back.
	exe := writeExe(t, src, "copynote (1).exe", "binary-content")

	got, err := CopyTo(exe, dst)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	if want := filepath.Join(dst, ExeName); got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if string(body) != "binary-content" {
		t.Errorf("copy content = %q", body)
	}
	// The original is deliberately left in place: the relaunched copy
	// removes it, so a failed start still leaves a working executable.
	if _, err := os.Stat(exe); err != nil {
		t.Errorf("original should survive the copy: %v", err)
	}
	// Nothing staged may be left behind.
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatalf("read target dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".copynote-move-") {
			t.Errorf("staging file left behind: %s", e.Name())
		}
	}
}

func TestCopyToReplacesExistingCopy(t *testing.T) {
	src := testutil.TempDir(t)
	dst := testutil.TempDir(t)
	writeExe(t, dst, ExeName, "old")
	exe := writeExe(t, src, ExeName, "new")

	got, err := CopyTo(exe, dst)
	if err != nil {
		t.Fatalf("CopyTo: %v", err)
	}
	body, _ := os.ReadFile(got)
	if string(body) != "new" {
		t.Errorf("content = %q, want the newly copied one", body)
	}
}

func TestCopyToRefusesSameLocation(t *testing.T) {
	dir := testutil.TempDir(t)
	exe := writeExe(t, dir, ExeName, "body")
	if _, err := CopyTo(exe, dir); err == nil {
		t.Fatal("copying onto itself should fail")
	}
}

func TestCopyToRejectsEmptyInput(t *testing.T) {
	dir := testutil.TempDir(t)
	if _, err := CopyTo("", dir); err == nil {
		t.Error("empty executable path should fail")
	}
	if _, err := CopyTo(filepath.Join(dir, ExeName), ""); err == nil {
		t.Error("empty target folder should fail")
	}
}

func TestRunPendingCleanupRemovesLeftover(t *testing.T) {
	data := testutil.TempDir(t)
	old := testutil.TempDir(t)
	leftover := writeExe(t, old, ExeName, "stale")
	current := writeExe(t, testutil.TempDir(t), ExeName, "running")

	if err := MarkPending(data, leftover); err != nil {
		t.Fatalf("MarkPending: %v", err)
	}
	if err := RunPendingCleanup(data, current, time.Second); err != nil {
		t.Fatalf("RunPendingCleanup: %v", err)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Errorf("leftover should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, pendingName)); !os.IsNotExist(err) {
		t.Error("marker should be removed")
	}
}

func TestRunPendingCleanupWithoutMarker(t *testing.T) {
	data := testutil.TempDir(t)
	if err := RunPendingCleanup(data, "", time.Second); err != nil {
		t.Errorf("no marker should be a no-op, got %v", err)
	}
}

// The marker is a file on disk. Anything that manages to write it must not
// be able to turn the next start into an arbitrary file deletion.
func TestRunPendingCleanupRefusesForeignTarget(t *testing.T) {
	data := testutil.TempDir(t)
	victim := writeExe(t, testutil.TempDir(t), "important.txt", "keep me")

	if err := MarkPending(data, victim); err != nil {
		t.Fatalf("MarkPending: %v", err)
	}
	if err := RunPendingCleanup(data, "", time.Second); err == nil {
		t.Error("a non-CopyNote target should be refused")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("victim file must survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(data, pendingName)); !os.IsNotExist(err) {
		t.Error("a refused marker should still be cleared")
	}
}

func TestRunPendingCleanupRefusesRunningExecutable(t *testing.T) {
	data := testutil.TempDir(t)
	current := writeExe(t, testutil.TempDir(t), ExeName, "running")

	if err := MarkPending(data, current); err != nil {
		t.Fatalf("MarkPending: %v", err)
	}
	if err := RunPendingCleanup(data, current, time.Second); err == nil {
		t.Error("deleting the running executable should be refused")
	}
	if _, err := os.Stat(current); err != nil {
		t.Errorf("running executable must survive: %v", err)
	}
}

func TestSafeToDeleteGuards(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"our executable", `C:\Users\x\Downloads\copynote.exe`, true},
		{"decorated download", `C:\Users\x\Downloads\copynote (1).exe`, true},
		{"relative path", `copynote.exe`, false},
		{"not an exe", `C:\Users\x\Downloads\copynote.txt`, false},
		{"someone else's exe", `C:\Windows\System32\cmd.exe`, false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeToDelete(tc.path, ""); got != tc.want {
				t.Errorf("safeToDelete(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\x\AppData\Local`)
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if want := filepath.Join(`C:\Users\x\AppData\Local`, "Programs", "CopyNote"); dir != want {
		t.Errorf("DefaultDir = %q, want %q", dir, want)
	}
	t.Setenv("LOCALAPPDATA", "")
	if _, err := DefaultDir(); err == nil {
		t.Error("missing LOCALAPPDATA should fail")
	}
}

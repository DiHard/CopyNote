package updater

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"copynote/internal/testutil"
)

// releaseServer serves a fake release: the binary and a signature made
// with priv for signedVersion. requests counts asset downloads.
func releaseServer(t *testing.T, priv ed25519.PrivateKey, binary []byte, signedVersion string) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/copynote.exe", func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write(binary)
	})
	mux.HandleFunc("/copynote.exe.sig", func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write(EncodeSignature(Sign(priv, signedVersion, binary)))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &requests
}

func releaseInfo(srv *httptest.Server, version string, size int) *ReleaseInfo {
	return &ReleaseInfo{
		Version:      version,
		URL:          srv.URL + "/releases/tag/v" + version,
		Size:         int64(size),
		DownloadURL:  srv.URL + "/copynote.exe",
		SignatureURL: srv.URL + "/copynote.exe.sig",
	}
}

func TestInstallDownloadsVerifiesAndSwaps(t *testing.T) {
	pub, priv := testKeys(t)
	binary := []byte("MZ new version")
	srv, _ := releaseServer(t, priv, binary, "2.1.0")
	exe := filepath.Join(testutil.TempDir(t), "copynote.exe")
	writeFile(t, exe, "old version")

	var stages []Stage
	err := install(context.Background(), srv.Client(), pub, releaseInfo(srv, "2.1.0", len(binary)), exe, "2.0.0", func(stage Stage, done, total int64) {
		if len(stages) == 0 || stages[len(stages)-1] != stage {
			stages = append(stages, stage)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, exe); got != string(binary) {
		t.Fatalf("exe = %q", got)
	}
	if got := readFile(t, PreviousPath(exe)); got != "old version" {
		t.Fatalf("previous = %q", got)
	}
	if _, err := os.Stat(StagingPath(exe)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging left behind: %v", err)
	}
	want := []Stage{StageDownload, StageVerify, StageApply}
	if len(stages) != len(want) {
		t.Fatalf("stages = %v", stages)
	}
	for i := range want {
		if stages[i] != want[i] {
			t.Fatalf("stages = %v", stages)
		}
	}
}

func TestInstallRejectsUnverifiableBinary(t *testing.T) {
	pub, priv := testKeys(t)
	_, otherPriv := testKeys(t)
	binary := []byte("MZ new version")
	for name, tc := range map[string]struct {
		priv    ed25519.PrivateKey
		signed  string
		claimed string
	}{
		"replayed older release": {priv, "2.0.5", "2.1.0"},
		"foreign key":            {otherPriv, "2.1.0", "2.1.0"},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := releaseServer(t, tc.priv, binary, tc.signed)
			exe := filepath.Join(testutil.TempDir(t), "copynote.exe")
			writeFile(t, exe, "old version")

			err := install(context.Background(), srv.Client(), pub, releaseInfo(srv, tc.claimed, len(binary)), exe, "2.0.0", nil)
			if err == nil {
				t.Fatal("unverifiable binary installed")
			}
			if got := readFile(t, exe); got != "old version" {
				t.Fatalf("exe = %q", got)
			}
			if _, err := os.Stat(StagingPath(exe)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unverified download left behind: %v", err)
			}
			if _, err := os.Stat(PreviousPath(exe)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("previous created without a swap: %v", err)
			}
		})
	}
}

func TestInstallRequiresSignedReleaseAndKey(t *testing.T) {
	pub, priv := testKeys(t)
	binary := []byte("MZ new version")
	srv, requests := releaseServer(t, priv, binary, "2.1.0")
	exe := filepath.Join(testutil.TempDir(t), "copynote.exe")
	writeFile(t, exe, "old version")

	unsigned := releaseInfo(srv, "2.1.0", len(binary))
	unsigned.SignatureURL = ""
	if err := install(context.Background(), srv.Client(), pub, unsigned, exe, "2.0.0", nil); err == nil {
		t.Fatal("release without signature installed")
	}
	if err := install(context.Background(), srv.Client(), nil, releaseInfo(srv, "2.1.0", len(binary)), exe, "2.0.0", nil); err == nil {
		t.Fatal("install without an embedded key succeeded")
	}
	if err := install(context.Background(), srv.Client(), pub, releaseInfo(srv, "2.1.0", len(binary)), "", "2.0.0", nil); err == nil {
		t.Fatal("install without an executable path succeeded")
	}
	if *requests != 0 {
		t.Fatalf("%d requests sent before preconditions were checked", *requests)
	}
	if got := readFile(t, exe); got != "old version" {
		t.Fatalf("exe = %q", got)
	}
}

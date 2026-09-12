package updater

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"copynote/internal/testutil"
)

func serveBytes(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("missing user agent")
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadWritesFileAndReportsProgress(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 100_000)
	srv := serveBytes(t, body)
	dst := filepath.Join(testutil.TempDir(t), "copynote.exe.new")

	var lastDone, lastTotal int64
	err := download(context.Background(), srv.Client(), srv.URL, dst, int64(len(body)), "test", func(done, total int64) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("downloaded %d bytes, want %d", len(got), len(body))
	}
	if lastDone != int64(len(body)) || lastTotal != int64(len(body)) {
		t.Fatalf("progress ended at %d/%d", lastDone, lastTotal)
	}
}

func TestDownloadRemovesShortFile(t *testing.T) {
	body := []byte("only part of the binary")
	srv := serveBytes(t, body)
	dst := filepath.Join(testutil.TempDir(t), "copynote.exe.new")

	err := download(context.Background(), srv.Client(), srv.URL, dst, int64(len(body))+10, "test", nil)
	if err == nil {
		t.Fatal("short body accepted")
	}
	if _, statErr := os.Stat(dst); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial file left behind: %v", statErr)
	}
}

func TestDownloadRejectsOversizedRelease(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer srv.Close()
	dst := filepath.Join(testutil.TempDir(t), "copynote.exe.new")

	if err := download(context.Background(), srv.Client(), srv.URL, dst, maxDownloadSize+1, "test", nil); err == nil {
		t.Fatal("oversized release accepted")
	}
	if requests != 0 {
		t.Fatal("request sent despite oversized declared size")
	}
}

func TestDownloadRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer srv.Close()
	dst := filepath.Join(testutil.TempDir(t), "copynote.exe.new")
	if err := download(context.Background(), srv.Client(), srv.URL, dst, 0, "test", nil); err == nil {
		t.Fatal("404 accepted")
	}
}

func TestDownloadStopsOnCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	dst := filepath.Join(testutil.TempDir(t), "copynote.exe.new")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	done := make(chan error, 1)
	go func() { done <- download(ctx, srv.Client(), srv.URL, dst, 0, "test", nil) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled download reported success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download ignored cancellation")
	}
}

func TestFetchSignature(t *testing.T) {
	_, priv := testKeys(t)
	sig := Sign(priv, "1.0.0", []byte("data"))
	good := serveBytes(t, EncodeSignature(sig))
	got, err := fetchSignature(context.Background(), good.Client(), good.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, sig) {
		t.Fatal("signature changed in transit")
	}

	bad := serveBytes(t, []byte("<html>not found</html>"))
	if _, err := fetchSignature(context.Background(), bad.Client(), bad.URL, "test"); err == nil {
		t.Fatal("garbage signature accepted")
	}
}

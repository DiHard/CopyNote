package updater

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckLatestResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body  string
		status      int
		wantVersion string
		wantError   bool
	}{
		{"newer", `{"tag_name":"v1.3.0","html_url":"https://github.com/DiHard/CopyNote/releases/tag/v1.3.0"}`, 200, "1.3.0", false},
		{"newer with assets", `{"tag_name":"v1.3.0","assets":[{"name":"copynote.exe","browser_download_url":"https://dl.example/copynote.exe","size":7627776},{"name":"copynote.exe.sig","browser_download_url":"https://dl.example/copynote.exe.sig","size":89},{"name":"other.zip","browser_download_url":"https://dl.example/other.zip","size":1}]}`, 200, "1.3.0", false},
		{"same", `{"tag_name":"v1.2.0"}`, 200, "", false},
		{"older", `{"tag_name":"v1.1.0"}`, 200, "", false},
		{"rate limit", `{}`, 429, "", true},
		{"invalid JSON", `{`, 200, "", true},
		{"invalid tag", `{"tag_name":"nightly"}`, 200, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") == "" {
					t.Error("missing user agent")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			info, err := checkLatest(context.Background(), server.Client(), server.URL, "1.2.0")
			if (err != nil) != tc.wantError {
				t.Fatalf("error: %v", err)
			}
			if tc.wantVersion == "" {
				if info != nil {
					t.Fatalf("unexpected update: %#v", info)
				}
			} else if info == nil || info.Version != tc.wantVersion {
				t.Fatalf("update: %#v", info)
			}
		})
	}
}

func TestCheckLatestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := checkLatest(ctx, http.DefaultClient, "http://127.0.0.1:1", "1.2.0"); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		current, latest string
		want            bool
	}{
		{"1.2.0", "1.10.0", true}, {"1.9.9", "2.0.0", true}, {"v1.2.0", "v1.2.1", true},
		{"1.2.0", "1.2.0", false}, {"2.0.0", "1.9.9", false}, {"dev", "1.0.0", false}, {"1.0.0", "invalid", false},
	} {
		if got := IsNewer(tc.current, tc.latest); got != tc.want {
			t.Errorf("IsNewer(%q,%q)=%v", tc.current, tc.latest, got)
		}
	}
}

func TestCheckLatestReleaseAssets(t *testing.T) {
	serve := func(body string) (*httptest.Server, func()) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		return srv, srv.Close
	}

	signed, closeSigned := serve(`{"tag_name":"v1.3.0","assets":[
		{"name":"copynote.exe","browser_download_url":"https://dl.example/copynote.exe","size":7627776},
		{"name":"copynote.exe.sig","browser_download_url":"https://dl.example/copynote.exe.sig","size":89},
		{"name":"other.zip","browser_download_url":"https://dl.example/other.zip","size":1}]}`)
	defer closeSigned()
	info, err := checkLatest(context.Background(), signed.Client(), signed.URL, "1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Installable() || info.DownloadURL != "https://dl.example/copynote.exe" || info.SignatureURL != "https://dl.example/copynote.exe.sig" || info.Size != 7627776 {
		t.Fatalf("assets: %#v", info)
	}

	unsigned, closeUnsigned := serve(`{"tag_name":"v1.3.0","assets":[{"name":"copynote.exe","browser_download_url":"https://dl.example/copynote.exe","size":7627776}]}`)
	defer closeUnsigned()
	info, err = checkLatest(context.Background(), unsigned.Client(), unsigned.URL, "1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if info.Installable() || info.DownloadURL == "" || info.SignatureURL != "" {
		t.Fatalf("release without signature: %#v", info)
	}
	if (*ReleaseInfo)(nil).Installable() {
		t.Fatal("nil release reported installable")
	}
}

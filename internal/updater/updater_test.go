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

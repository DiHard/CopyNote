// Package updater checks GitHub Releases for a newer version of the
// application and, when the release ships a signed binary, downloads,
// verifies and installs it in place (see install.go and apply.go).
//
// The check is a single unauthenticated HTTP GET to GitHub's public
// releases API. Anonymous rate limit is 60 requests/hour per IP, which
// is well above what a single user produces. Asset downloads are served
// from GitHub's CDN and do not count against that limit.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// releasesURL is the GitHub endpoint that returns the latest non-draft,
// non-prerelease release for the repository. Pre-releases are excluded
// automatically, which is the desired behavior — beta builds should
// not trigger update notifications for stable users.
//
// A var rather than a const so an end-to-end test build can point the
// updater at a local server:
//
//	go build -ldflags "-X copynote/internal/updater.releasesURL=http://127.0.0.1:18080/latest" .
var releasesURL = "https://api.github.com/repos/DiHard/CopyNote/releases/latest"

// requestTimeout bounds the HTTP request so a slow network never
// blocks startup for long. The check runs in a background goroutine,
// but a tight cap keeps resource usage predictable.
const requestTimeout = 5 * time.Second

// Release asset names. Assets are looked up by exact name, so the release
// process must upload the binary and its detached signature under these.
const (
	AssetName          = "copynote.exe"
	SignatureAssetName = "copynote.exe.sig"
)

// ReleaseInfo is the subset of the GitHub release payload we surface
// to the UI. Fields marshal across the Go ↔ JS bridge; the asset URLs
// stay on the Go side.
type ReleaseInfo struct {
	Version     string `json:"version"`     // e.g. "1.0.2" (no leading v)
	Name        string `json:"name"`        // release title
	URL         string `json:"url"`         // release page (html_url)
	PublishedAt string `json:"publishedAt"` // RFC3339 timestamp
	// Size is the byte length of the release binary, 0 when the release
	// carries no binary asset.
	Size int64 `json:"size"`
	// DownloadURL and SignatureURL point at the release binary and its
	// detached signature. Empty when the release lacks either asset; the
	// UI then falls back to opening the release page.
	DownloadURL  string `json:"-"`
	SignatureURL string `json:"-"`
}

// Installable reports whether the release ships everything a self-update
// needs: the binary and its signature.
func (r *ReleaseInfo) Installable() bool {
	return r != nil && r.DownloadURL != "" && r.SignatureURL != ""
}

// githubRelease mirrors the fields we consume from the GitHub API.
// Everything else in the payload is ignored.
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt string        `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CheckLatest queries GitHub and returns a ReleaseInfo when a newer
// version than currentVersion is available. When the remote is not
// newer (or currentVersion is unparseable), returns (nil, nil).
//
// Errors indicate that the check itself failed — network timeout,
// non-200 status, malformed JSON. Callers should treat errors as
// "try again later" and surface nothing to the user.
func CheckLatest(ctx context.Context, currentVersion string) (*ReleaseInfo, error) {
	return checkLatest(ctx, http.DefaultClient, releasesURL, currentVersion)
}

func checkLatest(ctx context.Context, client *http.Client, endpoint, currentVersion string) (*ReleaseInfo, error) {
	if !isSemverLike(currentVersion) {
		// dev / empty / unparseable — refuse to nag the user.
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	resp, err := get(ctx, client, endpoint, "application/vnd.github+json", userAgent(currentVersion))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap at 1 MB
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}

	latest := strings.TrimPrefix(rel.TagName, "v")
	if !isSemverLike(latest) {
		return nil, errors.New("latest tag is not semver-like")
	}

	if !IsNewer(currentVersion, latest) {
		return nil, nil
	}

	info := &ReleaseInfo{
		Version:     latest,
		Name:        rel.Name,
		URL:         rel.HTMLURL,
		PublishedAt: rel.PublishedAt,
	}
	for _, asset := range rel.Assets {
		switch asset.Name {
		case AssetName:
			info.DownloadURL = asset.BrowserDownloadURL
			info.Size = asset.Size
		case SignatureAssetName:
			info.SignatureURL = asset.BrowserDownloadURL
		}
	}
	return info, nil
}

// userAgent identifies the running version to GitHub; the repository URL
// is what GitHub asks unauthenticated clients to include.
func userAgent(currentVersion string) string {
	return "CopyNote/" + currentVersion + " (+https://github.com/DiHard/CopyNote)"
}

// get performs a GET and returns the response only on HTTP 200.
func get(ctx context.Context, client *http.Client, url, accept, ua string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return resp, nil
}

// IsNewer reports whether latest is a strictly higher semver than
// current. Pre-release and build-metadata suffixes are stripped before
// the numeric comparison — "1.0.2-rc1" compares equal to "1.0.2".
//
// Accepts versions with or without a leading "v". Returns false if
// either side is unparseable.
func IsNewer(current, latest string) bool {
	cMajor, cMinor, cPatch, ok := parseSemver(current)
	if !ok {
		return false
	}
	lMajor, lMinor, lPatch, ok := parseSemver(latest)
	if !ok {
		return false
	}
	switch {
	case lMajor != cMajor:
		return lMajor > cMajor
	case lMinor != cMinor:
		return lMinor > cMinor
	default:
		return lPatch > cPatch
	}
}

// isSemverLike returns true if s parses as MAJOR.MINOR.PATCH with
// optional leading "v" and optional "-suffix" / "+suffix".
func isSemverLike(s string) bool {
	_, _, _, ok := parseSemver(s)
	return ok
}

// parseSemver extracts the numeric MAJOR.MINOR.PATCH components. Any
// "-rc1" or "+build" suffix is dropped before parsing.
func parseSemver(s string) (major, minor, patch int, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	// Strip "-foo" prerelease and "+foo" build metadata.
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	var nums [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}

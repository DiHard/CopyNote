// Package updater checks GitHub Releases for a newer version of the
// application. It does not download or install anything — it only
// reports whether an update is available so the UI can surface a
// notification and link the user to the release page.
//
// The check is a single unauthenticated HTTP GET to GitHub's public
// releases API. Anonymous rate limit is 60 requests/hour per IP, which
// is well above what a single user produces.
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
const releasesURL = "https://api.github.com/repos/DiHard/CopyNote/releases/latest"

// requestTimeout bounds the HTTP request so a slow network never
// blocks startup for long. The check runs in a background goroutine,
// but a tight cap keeps resource usage predictable.
const requestTimeout = 5 * time.Second

// ReleaseInfo is the subset of the GitHub release payload we surface
// to the UI. All fields are plain strings so the struct marshals
// cleanly across the Go ↔ JS bridge.
type ReleaseInfo struct {
	Version     string `json:"version"`     // e.g. "1.0.2" (no leading v)
	Name        string `json:"name"`        // release title
	URL         string `json:"url"`         // release page (html_url)
	PublishedAt string `json:"publishedAt"` // RFC3339 timestamp
}

// githubRelease mirrors the fields we consume from the GitHub API.
// Everything else in the payload is ignored.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

// CheckLatest queries GitHub and returns a ReleaseInfo when a newer
// version than currentVersion is available. When the remote is not
// newer (or currentVersion is unparseable), returns (nil, nil).
//
// Errors indicate that the check itself failed — network timeout,
// non-200 status, malformed JSON. Callers should treat errors as
// "try again later" and surface nothing to the user.
func CheckLatest(ctx context.Context, currentVersion string) (*ReleaseInfo, error) {
	if !isSemverLike(currentVersion) {
		// dev / empty / unparseable — refuse to nag the user.
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CopyNote/"+currentVersion+" (+https://github.com/DiHard/CopyNote)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

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

	return &ReleaseInfo{
		Version:     latest,
		Name:        rel.Name,
		URL:         rel.HTMLURL,
		PublishedAt: rel.PublishedAt,
	}, nil
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

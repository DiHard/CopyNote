// Package version exposes the application's semantic version string.
//
// Version is the single source of truth for the version number shown
// in the About section of the UI and embedded in exported backup files.
//
// It is declared as a var (not a const) so release builds can override
// it via ldflags without editing the source:
//
//	go build -ldflags="-X copynote/internal/version.Version=1.2.3" .
package version

// Version is the application's semantic version (MAJOR.MINOR.PATCH),
// without a leading "v". Bump in-source for normal release commits;
// the ldflags override is only for CI / tagged-release automation.
var Version = "1.2.0"

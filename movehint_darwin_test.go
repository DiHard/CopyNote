package main

import "testing"

func TestNeedsMove(t *testing.T) {
	const home = "/Users/me"
	for _, tc := range []struct {
		bundle string
		want   bool
	}{
		{"/Applications/CopyNote.app", false},
		{"/Users/me/Applications/CopyNote.app", false},
		{"/Users/me/Projects/CopyNote/build/CopyNote.app", false},
		{"/Users/me/Downloads/CopyNote.app", true},
		{"/Users/me/Downloads/CopyNote 2/CopyNote.app", true},
		{"/Users/me/DownloadsArchive/CopyNote.app", false},
		{"/private/var/folders/xy/T/AppTranslocation/0A1B/d/CopyNote.app", true},
		{"", false},
	} {
		if got := needsMove(tc.bundle, home); got != tc.want {
			t.Errorf("needsMove(%q) = %v, want %v", tc.bundle, got, tc.want)
		}
	}
}

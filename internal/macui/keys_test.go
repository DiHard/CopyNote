//go:build darwin

package macui

import "testing"

func TestKeyEquivalent(t *testing.T) {
	cases := []struct {
		in   string
		key  string
		mods uint
		ok   bool
	}{
		{"⌘E", "e", modCommand, true},
		{"⌘⌫", "\b", modCommand, true},
		{"⌘↑", "", modCommand, true},
		{"⌘↓", "", modCommand, true},
		{"⌃⌥⇧⌘7", "7", modControl | modOption | modShift | modCommand, true},
		{"↩", "", 0, false},  // no modifier: would beat the highlighted item
		{"F2", "", 0, false}, // a Windows name
		{"Ctrl+↑", "", 0, false},
		{"⌘", "", 0, false},
		{"", "", 0, false},
	}
	for _, c := range cases {
		key, mods, ok := keyEquivalent(c.in)
		if key != c.key || mods != c.mods || ok != c.ok {
			t.Errorf("keyEquivalent(%q) = %q, %#x, %v; want %q, %#x, %v", c.in, key, mods, ok, c.key, c.mods, c.ok)
		}
	}
}

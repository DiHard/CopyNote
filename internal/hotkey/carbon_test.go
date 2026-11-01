package hotkey

import (
	"fmt"
	"testing"
)

func TestCarbon(t *testing.T) {
	cases := []struct {
		in        string
		mods, key uint32
	}{
		{"Ctrl+Alt+N", CarbonControl | CarbonOption, 0x2D},
		{"Win+Shift+V", CarbonCmd | CarbonShift, 0x09},
		{"Alt+Space", CarbonOption, 0x31},
		{"Ctrl+F5", CarbonControl, 0x60},
		{"Ctrl+Alt+0", CarbonControl | CarbonOption, 0x1D},
		{"Ctrl+Backquote", CarbonControl, 0x32},
	}
	for _, c := range cases {
		spec, err := Parse(c.in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.in, err)
		}
		mods, key, ok := spec.Carbon()
		if !ok || mods != c.mods || key != c.key {
			t.Errorf("%q.Carbon() = %#x, %#x, %v; want %#x, %#x", c.in, mods, key, ok, c.mods, c.key)
		}
	}
}

// Every key Parse accepts must either map or be one a Mac lacks, so the
// table cannot silently fall behind virtualKey.
func TestCarbonCoversEveryParsableKey(t *testing.T) {
	var names []string
	for c := 'A'; c <= 'Z'; c++ {
		names = append(names, string(c))
	}
	for c := '0'; c <= '9'; c++ {
		names = append(names, string(c))
	}
	for n := 1; n <= 24; n++ {
		names = append(names, fmt.Sprintf("F%d", n))
	}
	names = append(names, "Space", "Insert", "Delete", "Home", "End", "PageUp", "PageDown", "Backquote")
	codes := map[uint32]string{}
	for _, name := range names {
		spec, err := Parse("Ctrl+" + name)
		if err != nil {
			t.Fatalf("Parse(Ctrl+%s): %v", name, err)
		}
		_, key, ok := spec.Carbon()
		macLacks := name == "F21" || name == "F22" || name == "F23" || name == "F24"
		if ok == macLacks {
			t.Errorf("Ctrl+%s: ok=%v", name, ok)
			continue
		}
		if ok {
			if other, dup := codes[key]; dup {
				t.Errorf("%s and %s share key code %#x", name, other, key)
			}
			codes[key] = name
		}
	}
}

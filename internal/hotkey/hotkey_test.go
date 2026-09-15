package hotkey

import "testing"

func TestResolve_EmptyMeansTheDefaultNotDisabled(t *testing.T) {
	// storage.decode leaves keys absent from an older data.json at their zero
	// value, so "" is what every existing installation carries.
	spec, enabled, err := Resolve("")
	if err != nil || !enabled {
		t.Fatalf("Resolve(\"\") = %v, %v, %v", spec, enabled, err)
	}
	want, _ := Parse(Default)
	if spec != want {
		t.Errorf("Resolve(\"\") = %v, want the default %v", spec, want)
	}
}

func TestResolve_Off(t *testing.T) {
	for _, in := range []string{"off", "OFF", " off "} {
		_, enabled, err := Resolve(in)
		if err != nil || enabled {
			t.Errorf("Resolve(%q): enabled=%v err=%v, want disabled", in, enabled, err)
		}
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		mods uint32
		vk   uint32
	}{
		{"Ctrl+Alt+N", ModControl | ModAlt, 'N'},
		{"ctrl + shift + f5", ModControl | ModShift, 0x74},
		{"Alt+Space", ModAlt, 0x20},
		{"Win+Shift+V", ModWin | ModShift, 'V'},
		{"Ctrl+Alt+7", ModControl | ModAlt, '7'},
		{"Ctrl+F24", ModControl, 0x87},
		// Order must not matter: the capture UI emits modifiers in its own order.
		{"Alt+Ctrl+N", ModControl | ModAlt, 'N'},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if got.Mods != c.mods || got.VK != c.vk {
			t.Errorf("Parse(%q) = %#v, want mods=%#x vk=%#x", c.in, got, c.mods, c.vk)
		}
	}
}

func TestParse_Rejects(t *testing.T) {
	cases := map[string]string{
		"a bare key would be stolen from every program": "N",
		"modifiers alone are not a hotkey":              "Ctrl+Alt",
		"two keys":                                      "Ctrl+A+B",
		"empty part":                                    "Ctrl++N",
		"unknown key":                                   "Ctrl+Alt+Enter",
		"function key out of range":                     "Ctrl+F25",
		"nothing at all":                                "",
	}
	for why, in := range cases {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error — %s", in, why)
		}
	}
}

func TestValid_AcceptsTheStoredForms(t *testing.T) {
	for _, in := range []string{"", "off", "Ctrl+Alt+N", "Ctrl+Shift+F1"} {
		if err := Valid(in); err != nil {
			t.Errorf("Valid(%q) = %v, want nil", in, err)
		}
	}
	if err := Valid("Ctrl+Alt+Enter"); err == nil {
		t.Error("Valid must reject a combination the app cannot register")
	}
}

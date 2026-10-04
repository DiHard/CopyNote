package hotkey

import (
	"strings"
	"testing"
)

func FuzzResolve(f *testing.F) {
	for _, seed := range []string{"", "off", "Ctrl+Alt+N", "shift + ctrl + f24", "N", "Ctrl++N", "Win+Space", "Ctrl+F25", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, setting string) {
		spec, enabled, err := Resolve(setting)
		if (Valid(setting) == nil) != (err == nil) {
			t.Fatal("validation and resolution disagree")
		}
		if err != nil {
			if enabled || spec != (Spec{}) {
				t.Fatal("invalid hotkey returned an enabled or partial combination")
			}
			return
		}
		if !enabled {
			if !strings.EqualFold(strings.TrimSpace(setting), Off) || spec != (Spec{}) {
				t.Fatal("only the off sentinel may disable a valid hotkey")
			}
			return
		}
		if spec.Mods == 0 || spec.VK == 0 {
			t.Fatal("enabled hotkey must have a modifier and a key")
		}
		padded, paddedEnabled, paddedErr := Resolve(" \t" + setting + "\n ")
		if paddedErr != nil || !paddedEnabled || padded != spec {
			t.Fatal("surrounding whitespace changed the hotkey")
		}
	})
}

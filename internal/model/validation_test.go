package model

import "testing"

func TestValidateSettings_Hotkey(t *testing.T) {
	// "" is what every data.json written before the hotkey existed carries,
	// so it has to stay valid — it means the built-in default.
	for _, ok := range []string{"", "off", "Ctrl+Alt+N", "Ctrl+Shift+F5"} {
		s := DefaultSettings()
		s.Hotkey = ok
		if err := ValidateSettings(s); err != nil {
			t.Errorf("hotkey %q rejected: %v", ok, err)
		}
	}
	// An import carrying a combination the app cannot register must fail
	// validation rather than land in data.json and fail on every start.
	for _, bad := range []string{"N", "Ctrl+Alt", "Ctrl+Alt+Enter"} {
		s := DefaultSettings()
		s.Hotkey = bad
		if err := ValidateSettings(s); err == nil {
			t.Errorf("hotkey %q accepted, want an error", bad)
		}
	}
}

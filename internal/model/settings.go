package model

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Settings holds user-configurable preferences persisted to
// data.json alongside entries. Older settings.json files are migrated on save.
//
// DisableUpdateCheck uses inverted semantics on purpose: zero-value
// (missing from older settings.json files) means "update checks are
// enabled", matching the desired default without needing a migration.
//
// LastSeenUpdateVersion records the latest release version that was
// acknowledged by the user (by opening the Settings view). When the
// remote latest differs from this value, the notification dot on the
// gear icon reappears.
//
// RelocatePromptDismissed silences the banner offering to move the
// executable into a permanent folder. Zero-value means "not yet asked",
// so an existing installation sees the offer once. It only hides the
// banner: the action stays reachable from Settings.
type Settings struct {
	Autorun                 bool   `json:"autorun"`
	Theme                   string `json:"theme"`   // "light" | "dark" | "system"
	Locale                  string `json:"locale"`  // "en" | "ru" | "system"
	Topmost                 bool   `json:"topmost"` // keep window above all others
	DisableUpdateCheck      bool   `json:"disableUpdateCheck"`
	// DisableAutoHide keeps the window on screen when another program takes
	// focus, instead of parking it off-screen. Inverted like the field above
	// for the same reason: storage.decode unmarshals into a zero Settings, so
	// a key missing from an older data.json must mean the default — and the
	// default is that the window hides.
	DisableAutoHide bool `json:"disableAutoHide"`
	LastSeenUpdateVersion   string `json:"lastSeenUpdateVersion"`
	RelocatePromptDismissed bool   `json:"relocatePromptDismissed"`
	// RelocateRemindAfter is an RFC3339 instant before which the move
	// banner stays hidden; "" means no snooze is running. Unlike the
	// permanent dismissal above, this one wears off on its own.
	RelocateRemindAfter string `json:"relocateRemindAfter"`
	// Hotkey is the global shortcut that opens the window, as the user sees
	// it ("Ctrl+Alt+N"). Empty means the built-in default rather than
	// "disabled" — an older data.json carries no such key, and a zero value
	// must not silently switch the feature off. "off" disables it.
	Hotkey string `json:"hotkey"`
}

// DefaultSettings returns the initial settings for a fresh install.
func DefaultSettings() Settings {
	return Settings{
		Autorun: false,
		Theme:   "system",
		Locale:  "system",
		Topmost: true,
		// DisableUpdateCheck: false → update checks are enabled by default.
		// LastSeenUpdateVersion: "" → first non-null remote version will
		// trigger a notification.
	}
}

// DecodeSettings preserves defaults for fields absent in older exports.
// Explicit nulls and missing core fields are malformed, not default values.
func DecodeSettings(raw []byte) (Settings, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Settings{}, err
	}
	if fields == nil || fields["theme"] == nil || fields["locale"] == nil {
		return Settings{}, fmt.Errorf("settings must include theme and locale")
	}
	for key, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Settings{}, fmt.Errorf("setting %s must not be null", key)
		}
	}
	s := DefaultSettings()
	if err := json.Unmarshal(raw, &s); err != nil {
		return Settings{}, err
	}
	if err := ValidateSettings(s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

package model

// Settings holds user-configurable preferences persisted to
// %APPDATA%\CopyNote\settings.json, separate from entry data.
//
// DisableUpdateCheck uses inverted semantics on purpose: zero-value
// (missing from older settings.json files) means "update checks are
// enabled", matching the desired default without needing a migration.
//
// LastSeenUpdateVersion records the latest release version that was
// acknowledged by the user (by opening the Settings view). When the
// remote latest differs from this value, the notification dot on the
// gear icon reappears.
type Settings struct {
	Autorun               bool   `json:"autorun"`
	Theme                 string `json:"theme"`   // "light" | "dark" | "system"
	Locale                string `json:"locale"`  // "en" | "ru" | "system"
	Topmost               bool   `json:"topmost"` // keep window above all others
	DisableUpdateCheck    bool   `json:"disableUpdateCheck"`
	LastSeenUpdateVersion string `json:"lastSeenUpdateVersion"`
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

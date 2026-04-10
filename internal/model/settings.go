package model

// Settings holds user-configurable preferences persisted to
// %APPDATA%\CopyNote\settings.json, separate from entry data.
type Settings struct {
	Autorun bool   `json:"autorun"`
	Theme   string `json:"theme"` // "light" | "dark" | "system"
}

// DefaultSettings returns the initial settings for a fresh install.
func DefaultSettings() Settings {
	return Settings{
		Autorun: false,
		Theme:   "system",
	}
}

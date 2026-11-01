package tray

// Texts is everything the shell renders on our behalf: the icon's menu and
// its hover text. Shared with the macOS menu bar icon.
type Texts struct {
	Open, Settings, Quit string
	// TipLoading covers the cold start, where a click cannot open anything
	// yet; TipReady doubles as the only place the app ever tells the user
	// what the icon is for.
	TipLoading, TipReady string
}

var trayTexts = map[string]Texts{
	"en": {
		Open: "Open CopyNote", Settings: "Settings", Quit: "Quit",
		TipLoading: "CopyNote — loading…", TipReady: "CopyNote — click to open",
	},
	"ru": {
		Open: "Открыть CopyNote", Settings: "Настройки", Quit: "Выход",
		TipLoading: "CopyNote — загрузка…", TipReady: "CopyNote — нажмите, чтобы открыть",
	},
}

// TextsFor returns the strings for a UI locale, English for any other.
func TextsFor(locale string) Texts {
	if s, ok := trayTexts[locale]; ok {
		return s
	}
	return trayTexts["en"]
}

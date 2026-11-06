package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"copynote/internal/macui"
)

// needsMove reports whether bundle runs from a place it cannot stay:
// the Downloads folder, or the read-only copy macOS runs an app from when
// it is opened straight from its download (App Translocation). From there
// an update cannot replace it and autorun points at a path that vanishes.
func needsMove(bundle, home string) bool {
	if bundle == "" {
		return false
	}
	if strings.Contains(bundle, "/AppTranslocation/") {
		return true
	}
	downloads := filepath.Join(home, "Downloads") + string(filepath.Separator)
	return strings.HasPrefix(bundle, downloads)
}

// moveTexts is the alert in each language the app speaks.
var moveTexts = map[string][4]string{
	"en": {
		"Move CopyNote to Applications",
		"CopyNote is running from Downloads, so it cannot update itself or start when you log in.\n\nCopyNote will quit and Finder will show it. Drag it to Applications in the Finder sidebar, then open it from there.",
		"Show in Finder",
		"Not Now",
	},
	"ru": {
		"Перенесите CopyNote в «Программы»",
		"Сейчас CopyNote запущена из «Загрузок», поэтому не сможет обновляться и запускаться при входе в систему.\n\nCopyNote закроется, а Finder покажет её. Перетащите её в «Программы» в боковой панели Finder и откройте оттуда.",
		"Показать в Finder",
		"Не сейчас",
	},
}

// offerMove asks the user to move the app into Applications themselves,
// and calls notNow when they would rather carry on from where it is.
// The app does not move itself: a program that copies and deletes its own
// files is what antivirus heuristics flag, while a drag in Finder is not.
func offerMove(bundle, locale string, notNow func()) {
	texts, ok := moveTexts[locale]
	if !ok {
		texts = moveTexts["en"]
	}
	if macui.Alert(texts[0], texts[1], texts[2], texts[3]) != 0 {
		notNow()
		return
	}
	if err := exec.Command("/usr/bin/open", "-R", movableBundle(bundle)).Run(); err != nil {
		log.Printf("show the app in Finder: %v", err)
	}
	goQuit()
}

// movableBundle is the bundle the user can drag. A translocated bundle is a
// read-only copy with a made-up path; the original almost always sits in
// Downloads under the same name.
func movableBundle(bundle string) string {
	if !strings.Contains(bundle, "/AppTranslocation/") {
		return bundle
	}
	if home, err := os.UserHomeDir(); err == nil {
		original := filepath.Join(home, "Downloads", filepath.Base(bundle))
		if _, err := os.Stat(original); err == nil {
			return original
		}
		return filepath.Join(home, "Downloads")
	}
	return bundle
}

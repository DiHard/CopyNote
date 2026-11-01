package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// logDir is ~/Library/Logs/CopyNote, where Console.app looks for app logs.
func logDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Library", "Logs", "CopyNote")
	}
	return filepath.Join(os.TempDir(), "CopyNote")
}

// systemLocale reads the region the user chose in System Settings, such as
// "ru_RU", and returns its language part. An app started from Finder has no
// LANG, so the environment cannot answer this.
func systemLocale() string {
	out, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return "en"
	}
	lang, _, _ := strings.Cut(strings.TrimSpace(string(out)), "_")
	return lang
}

// messageBox shows text in a system dialog. The text goes in as an argument
// rather than into the script, so nothing in it needs escaping. With ask set
// the dialog has macOS's own localized Cancel and OK, and Cancel makes
// osascript exit non-zero.
func messageBox(text string, ask bool) bool {
	script := `display dialog (item 1 of argv) with title "CopyNote" with icon caution`
	if !ask {
		script += ` buttons {"OK"} default button 1`
	}
	cmd := exec.Command("/usr/bin/osascript", "-e", "on run argv", "-e", script, "-e", "end run", text)
	return cmd.Run() == nil
}

// frontendHTML is the whole page: web/singlefile.ts inlines every script and
// stylesheet into it.
func frontendHTML() (string, error) {
	raw, err := distFS.ReadFile("web/dist/index.html")
	return string(raw), err
}

// Package clipboard writes text to the macOS general pasteboard.
package clipboard

import (
	"os"
	"os/exec"
	"strings"
)

// pasteboard is the one written to. A var so a test build can leave the
// user's clipboard alone (-X copynote/internal/clipboard.pasteboard=find).
var pasteboard = "general"

// WriteText hands s to pbcopy. pbcopy decodes its input by the locale, and an
// app started from Finder or launchd has none, which turns anything outside
// ASCII into mojibake; hence the explicit UTF-8 locale.
func WriteText(s string) error {
	cmd := exec.Command("/usr/bin/pbcopy", "-pboard", pasteboard)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

package autorun

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// SetEnabled writes or removes ~/Library/LaunchAgents/dev.copynote.<valueName>.plist.
// launchd reads that folder at sign-in, so the file alone is enough: loading
// it now would start a second copy at once.
func SetEnabled(valueName string, enabled bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	label := "dev.copynote." + valueName
	path := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	if !enabled {
		err := os.Remove(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, launchAgent(label, exe), 0o644)
}

// launchAgent is the property list that starts exe with AutostartFlag once
// per graphical sign-in.
func launchAgent(label, exe string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>`)
	escape(&b, label)
	b.WriteString(`</string>
	<key>ProgramArguments</key>
	<array>
		<string>`)
	escape(&b, exe)
	b.WriteString(`</string>
		<string>` + AutostartFlag + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`)
	return b.Bytes()
}

func escape(b *bytes.Buffer, s string) {
	_ = xml.EscapeText(b, []byte(s)) // A bytes.Buffer never fails to write.
}

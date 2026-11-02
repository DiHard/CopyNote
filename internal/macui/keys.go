//go:build darwin

package macui

import (
	"strings"
	"unicode/utf8"
)

// NSEventModifierFlags, as NSMenuItem.keyEquivalentModifierMask takes them.
const (
	modShift   = 1 << 17
	modControl = 1 << 18
	modOption  = 1 << 19
	modCommand = 1 << 20
)

// keyEquivalent turns a shortcut the page writes the Mac way ("⌘E", "⌘⌫",
// "⌘↑") into what NSMenuItem shows beside its title. ok is false for
// anything else, which then shows no key at all rather than a wrong one.
//
// A key without a modifier is refused too: an open menu answers its key
// equivalents before its own navigation, so a plain ↩ there would pick
// that item instead of the highlighted one.
func keyEquivalent(shortcut string) (key string, mods uint, ok bool) {
	rest := shortcut
	for {
		r, size := utf8.DecodeRuneInString(rest)
		var m uint
		switch r {
		case '⌃':
			m = modControl
		case '⌥':
			m = modOption
		case '⇧':
			m = modShift
		case '⌘':
			m = modCommand
		}
		if m == 0 {
			break
		}
		mods |= m
		rest = rest[size:]
	}
	if mods == 0 {
		return "", 0, false
	}
	switch rest {
	case "↑":
		return "", mods, true // NSUpArrowFunctionKey
	case "↓":
		return "", mods, true // NSDownArrowFunctionKey
	case "⌫":
		return "\b", mods, true // NSBackspaceCharacter
	case "↩":
		return "\r", mods, true
	}
	if len(rest) == 1 && strings.Contains("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", rest) {
		return strings.ToLower(rest), mods, true
	}
	return "", 0, false
}

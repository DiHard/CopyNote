// Package hotkey parses the global-hotkey preference into the modifier and
// virtual-key codes RegisterHotKey wants.
//
// The stored form is what the user sees — "Ctrl+Alt+N" — because the settings
// UI builds it straight from a captured KeyboardEvent. Parsing therefore has
// to be forgiving about order and spacing but strict about the result: a
// combination without a modifier would swallow a bare keystroke from every
// application on the machine.
package hotkey

import (
	"fmt"
	"strings"
)

// Windows RegisterHotKey modifier flags.
const (
	ModAlt      uint32 = 0x0001
	ModControl  uint32 = 0x0002
	ModShift    uint32 = 0x0004
	ModWin      uint32 = 0x0008
	ModNoRepeat uint32 = 0x4000
)

// Default is used when the setting is empty, which is what an installation
// predating the hotkey has: storage.decode leaves absent keys at their zero
// value, so "" has to mean "the default", not "disabled".
const Default = "Ctrl+Alt+N"

// Off disables the hotkey. A sentinel rather than "" for the reason above.
const Off = "off"

// Spec is a parsed combination.
type Spec struct {
	Mods uint32
	VK   uint32
}

// Resolve turns the stored preference into a spec. enabled is false when the
// user switched the hotkey off.
func Resolve(setting string) (spec Spec, enabled bool, err error) {
	s := strings.TrimSpace(setting)
	if strings.EqualFold(s, Off) {
		return Spec{}, false, nil
	}
	if s == "" {
		s = Default
	}
	spec, err = Parse(s)
	if err != nil {
		return Spec{}, false, err
	}
	return spec, true, nil
}

// Valid reports whether a stored preference is usable. Used by settings
// validation, which must not reject the empty default.
func Valid(setting string) error {
	_, _, err := Resolve(setting)
	return err
}

// Parse reads a combination like "Ctrl+Alt+N" or "ctrl + shift + f5".
func Parse(s string) (Spec, error) {
	parts := strings.Split(s, "+")
	var spec Spec
	var key string
	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			return Spec{}, fmt.Errorf("hotkey %q has an empty part", s)
		}
		switch strings.ToLower(part) {
		case "ctrl", "control":
			spec.Mods |= ModControl
		case "alt":
			spec.Mods |= ModAlt
		case "shift":
			spec.Mods |= ModShift
		case "win", "meta", "super":
			spec.Mods |= ModWin
		default:
			if key != "" {
				return Spec{}, fmt.Errorf("hotkey %q names more than one key", s)
			}
			key = part
		}
	}
	if key == "" {
		return Spec{}, fmt.Errorf("hotkey %q has no key", s)
	}
	vk, ok := virtualKey(key)
	if !ok {
		return Spec{}, fmt.Errorf("hotkey %q uses an unsupported key %q", s, key)
	}
	// Without a modifier the hotkey would take a plain keystroke away from
	// every other program, which no user could diagnose.
	if spec.Mods == 0 {
		return Spec{}, fmt.Errorf("hotkey %q needs at least one modifier", s)
	}
	spec.VK = vk
	return spec, nil
}

// virtualKey maps the key name to a Windows virtual-key code. The set is
// deliberately small: what a person would plausibly bind, and nothing whose
// code varies with the keyboard layout.
func virtualKey(name string) (uint32, bool) {
	if len(name) == 1 {
		c := name[0]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return uint32(c), true
		}
		return 0, false
	}
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "f") {
		if n, ok := functionKey(lower[1:]); ok {
			return 0x70 + n - 1, true // VK_F1 = 0x70
		}
	}
	named := map[string]uint32{
		"space":     0x20,
		"insert":    0x2D,
		"delete":    0x2E,
		"home":      0x24,
		"end":       0x23,
		"pageup":    0x21,
		"pagedown":  0x22,
		"backquote": 0xC0, // VK_OEM_3, the ` key on a US layout
	}
	vk, ok := named[lower]
	return vk, ok
}

func functionKey(digits string) (uint32, bool) {
	if digits == "" || len(digits) > 2 {
		return 0, false
	}
	var n uint32
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint32(c-'0')
	}
	if n < 1 || n > 24 {
		return 0, false
	}
	return n, true
}

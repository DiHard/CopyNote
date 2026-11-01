package hotkey

// Carbon modifier flags, as RegisterEventHotKey takes them.
const (
	CarbonCmd     uint32 = 0x0100
	CarbonShift   uint32 = 0x0200
	CarbonOption  uint32 = 0x0800
	CarbonControl uint32 = 0x1000
)

// Carbon translates the spec for macOS RegisterEventHotKey: the modifier
// flags and the hardware key code. Win means Cmd, Alt means Option. ok is
// false for a key a Mac keyboard does not have (F21–F24), which macOS then
// refuses the way Windows refuses a taken combination.
//
// Key codes are positions on an ANSI keyboard (kVK_* in HIToolbox/Events.h),
// so on a Russian layout Ctrl+Option+N is the key that types «т» — the same
// key Windows binds through its virtual-key code.
func (s Spec) Carbon() (mods, keyCode uint32, ok bool) {
	for _, m := range [...]struct{ win, mac uint32 }{
		{ModControl, CarbonControl},
		{ModAlt, CarbonOption},
		{ModShift, CarbonShift},
		{ModWin, CarbonCmd},
	} {
		if s.Mods&m.win != 0 {
			mods |= m.mac
		}
	}
	keyCode, ok = carbonKeys[s.VK]
	return mods, keyCode, ok
}

// carbonKeys maps every virtual-key code Parse can produce to its kVK_* code.
var carbonKeys = map[uint32]uint32{
	'A': 0x00, 'S': 0x01, 'D': 0x02, 'F': 0x03, 'H': 0x04, 'G': 0x05,
	'Z': 0x06, 'X': 0x07, 'C': 0x08, 'V': 0x09, 'B': 0x0B, 'Q': 0x0C,
	'W': 0x0D, 'E': 0x0E, 'R': 0x0F, 'Y': 0x10, 'T': 0x11, 'O': 0x1F,
	'U': 0x20, 'I': 0x22, 'P': 0x23, 'L': 0x25, 'J': 0x26, 'K': 0x28,
	'N': 0x2D, 'M': 0x2E,
	'1': 0x12, '2': 0x13, '3': 0x14, '4': 0x15, '6': 0x16, '5': 0x17,
	'9': 0x19, '7': 0x1A, '8': 0x1C, '0': 0x1D,
	0x70: 0x7A, 0x71: 0x78, 0x72: 0x63, 0x73: 0x76, 0x74: 0x60, 0x75: 0x61, // F1–F6
	0x76: 0x62, 0x77: 0x64, 0x78: 0x65, 0x79: 0x6D, 0x7A: 0x67, 0x7B: 0x6F, // F7–F12
	0x7C: 0x69, 0x7D: 0x6B, 0x7E: 0x71, 0x7F: 0x6A, 0x80: 0x40, 0x81: 0x4F, // F13–F18
	0x82: 0x50, 0x83: 0x5A, // F19–F20
	0x20: 0x31, // space
	0x2D: 0x72, // insert: a PC keyboard's Insert arrives as Help
	0x2E: 0x75, // delete: forward delete
	0x24: 0x73, // home
	0x23: 0x77, // end
	0x21: 0x74, // page up
	0x22: 0x79, // page down
	0xC0: 0x32, // backquote
}

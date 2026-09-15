//go:build windows

package popupmenu

import (
	"slices"
	"testing"

	"copynote/internal/winutil"
)

func TestMetricsFor(t *testing.T) {
	tests := []struct {
		dpi  uint32
		want metrics
	}{
		{96, metrics{dpi: 96, itemHeight: 36, padV: 6, padH: 16, minWidth: 180, hoverInset: 4, fontHeight: 14, separatorHeight: 9, separatorLine: 1, shortcutGap: 32}},
		{120, metrics{dpi: 120, itemHeight: 45, padV: 8, padH: 20, minWidth: 225, hoverInset: 5, fontHeight: 18, separatorHeight: 11, separatorLine: 1, shortcutGap: 40}},
		{144, metrics{dpi: 144, itemHeight: 54, padV: 9, padH: 24, minWidth: 270, hoverInset: 6, fontHeight: 21, separatorHeight: 14, separatorLine: 2, shortcutGap: 48}},
	}
	for _, tt := range tests {
		if got := metricsFor(tt.dpi); got != tt.want {
			t.Errorf("metricsFor(%d) = %+v, want %+v", tt.dpi, got, tt.want)
		}
	}
}

// The entry menu: three actions, a divider, two moves.
var entryItems = []Item{
	{ID: 1, Label: "Copy", Shortcut: "Enter"},
	{ID: 2, Label: "Edit", Shortcut: "F2"},
	{ID: 3, Label: "Delete", Shortcut: "Delete"},
	{Separator: true},
	{ID: 4, Label: "Move up", Shortcut: "Ctrl+Up"},
	{ID: 5, Label: "Move down", Shortcut: "Ctrl+Down"},
}

// tenPerRune stands in for the font: every character is 10 px wide.
func tenPerRune(s string) int32 { return int32(len([]rune(s))) * 10 }

func TestLayoutStacksRowsAndSeparators(t *testing.T) {
	size := metricsFor(96)
	tops, width, height := layout(entryItems, size, tenPerRune)

	if want := []int32{6, 42, 78, 114, 123, 159}; !slices.Equal(tops, want) {
		t.Errorf("tops = %v, want %v", tops, want)
	}
	if height != 201 {
		t.Errorf("height = %d, want 201 (5 rows, 1 separator, padding)", height)
	}
	// Longest label "Move down" 90 + padding 32 + gap 32 + longest shortcut "Ctrl+Down" 90.
	if width != 244 {
		t.Errorf("width = %d, want 244", width)
	}
}

func TestLayoutKeepsMinimumWidthWithoutShortcuts(t *testing.T) {
	_, width, _ := layout([]Item{{ID: 1, Label: "Open"}, {ID: 2, Label: "Quit"}}, metricsFor(96), tenPerRune)
	if width != 180 {
		t.Errorf("width = %d, want the 180 px minimum", width)
	}
}

func TestItemAtSkipsSeparatorsAndDisabledItems(t *testing.T) {
	size := metricsFor(96)
	items := slices.Clone(entryItems)
	items[4].Disabled = true // first entry: cannot move up
	tops, _, _ := layout(items, size, tenPerRune)

	tests := []struct {
		y    int32
		want int
	}{
		{0, -1},   // top padding
		{6, 0},    // first row
		{41, 0},   // its last pixel
		{42, 1},   // second row
		{118, -1}, // separator
		{130, -1}, // disabled "move up"
		{170, 5},  // "move down"
		{200, -1}, // bottom padding
	}
	for _, tt := range tests {
		if got := itemAt(items, tops, size, tt.y); got != tt.want {
			t.Errorf("itemAt(y=%d) = %d, want %d", tt.y, got, tt.want)
		}
	}
}

func TestNextEnabledWrapsAndSkips(t *testing.T) {
	items := slices.Clone(entryItems)
	items[5].Disabled = true // last entry: cannot move down

	tests := []struct {
		name      string
		from, dir int
		want      int
	}{
		{"down from nothing starts at the top", -1, 1, 0},
		{"up from nothing starts at the bottom", -1, -1, 4},
		{"down skips the separator", 2, 1, 4},
		{"down skips a disabled item and wraps", 4, 1, 0},
		{"up wraps past a disabled item", 0, -1, 4},
	}
	for _, tt := range tests {
		if got := nextEnabled(items, tt.from, tt.dir); got != tt.want {
			t.Errorf("%s: nextEnabled(%d, %d) = %d, want %d", tt.name, tt.from, tt.dir, got, tt.want)
		}
	}

	none := []Item{{Separator: true}, {ID: 1, Label: "x", Disabled: true}}
	if got := nextEnabled(none, -1, 1); got != -1 {
		t.Errorf("nothing pickable: got %d, want -1", got)
	}
}

func TestPlaceKeepsTheMenuOnTheMonitor(t *testing.T) {
	screen := winutil.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 1080}
	tests := []struct {
		name         string
		x, y         int32
		wantX, wantY int32
	}{
		{"fits where it was asked for", 100, 100, 100, 100},
		{"pushed left at the right edge", 1800, 100, 1920 - 244, 100},
		{"opens upwards near the bottom", 100, 1000, 100, 1000 - 201},
		{"never above the top", 100, 150, 100, 150},
	}
	for _, tt := range tests {
		x, y := place(tt.x, tt.y, 244, 201, screen)
		if x != tt.wantX || y != tt.wantY {
			t.Errorf("%s: place(%d, %d) = (%d, %d), want (%d, %d)", tt.name, tt.x, tt.y, x, y, tt.wantX, tt.wantY)
		}
	}

	short := winutil.Rect{Left: 0, Top: 0, Right: 1920, Bottom: 300}
	if _, y := place(100, 250, 244, 400, short); y != 0 {
		t.Errorf("a menu taller than the space above is clamped to the top: y = %d, want 0", y)
	}
}

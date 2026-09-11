//go:build windows

package winutil

import "testing"

func TestScaleForDPI(t *testing.T) {
	tests := []struct {
		v    int32
		dpi  uint32
		want int32
	}{
		{420, 96, 420},
		{420, 120, 525}, // 125 %
		{420, 144, 630}, // 150 %
		{420, 192, 840}, // 200 %
		{8, 120, 10},
		{6, 120, 8},     // 7.5 rounds up, like MulDiv
		{-14, 120, -18}, // negative values round away from zero too
		{451, 144, 677},
		{36, 0, 36}, // unknown DPI falls back to 96
	}
	for _, tt := range tests {
		if got := ScaleForDPI(tt.v, tt.dpi); got != tt.want {
			t.Errorf("ScaleForDPI(%d, %d) = %d, want %d", tt.v, tt.dpi, got, tt.want)
		}
	}
}

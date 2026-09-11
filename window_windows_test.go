package main

import "testing"

func TestWindowSizeForDPI(t *testing.T) {
	tests := []struct {
		name         string
		dpi          uint32
		contentCSS   int
		workAreaH    int32
		wantW, wantH int32
	}{
		{"100 %", 96, 451, 1032, 420, 451},
		{"125 %", 120, 451, 1140, 525, 564},
		{"150 %", 144, 451, 1400, 630, 677},
		{"200 %", 192, 451, 2000, 840, 902},
		{"tiny content gets the minimum height", 120, 10, 1140, 525, 100},
		{"tall content stops at the work area minus margins", 120, 2000, 1140, 525, 1120},
	}
	for _, tt := range tests {
		w, h := windowSizeForDPI(tt.dpi, tt.contentCSS, tt.workAreaH)
		if w != tt.wantW || h != tt.wantH {
			t.Errorf("%s: windowSizeForDPI(%d, %d, %d) = %dx%d, want %dx%d",
				tt.name, tt.dpi, tt.contentCSS, tt.workAreaH, w, h, tt.wantW, tt.wantH)
		}
	}
}

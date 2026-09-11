//go:build windows

package tray

import "testing"

func TestPopupMetricsFor(t *testing.T) {
	tests := []struct {
		dpi  uint32
		want popupMetrics
	}{
		{96, popupMetrics{dpi: 96, itemHeight: 36, padV: 6, padH: 16, minWidth: 180, hoverInset: 4, fontHeight: 14}},
		{120, popupMetrics{dpi: 120, itemHeight: 45, padV: 8, padH: 20, minWidth: 225, hoverInset: 5, fontHeight: 18}},
		{144, popupMetrics{dpi: 144, itemHeight: 54, padV: 9, padH: 24, minWidth: 270, hoverInset: 6, fontHeight: 21}},
	}
	for _, tt := range tests {
		if got := popupMetricsFor(tt.dpi); got != tt.want {
			t.Errorf("popupMetricsFor(%d) = %+v, want %+v", tt.dpi, got, tt.want)
		}
	}
}

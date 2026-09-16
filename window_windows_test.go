package main

import "testing"

func TestShowWaitsForHiddenPreparation(t *testing.T) {
	oldHidden, oldPrepared, oldPending := windowHidden.Load(), windowPrepared, showPending
	oldGeneration := windowGeneration.Load()
	t.Cleanup(func() {
		windowHidden.Store(oldHidden)
		windowPrepared, showPending = oldPrepared, oldPending
		windowGeneration.Store(oldGeneration)
	})
	windowHidden.Store(true)
	windowPrepared = false
	showPending = false
	windowGeneration.Store(42)
	showAndFocus(0)
	if !showPending || !windowHidden.Load() || windowGeneration.Load() != 42 {
		t.Fatal("a quick show must stay hidden and preserve the pending hide generation")
	}
	completeWindowPreparation(0, preparationID+1, 480)
	if windowPrepared || !showPending {
		t.Fatal("a stale layout acknowledgement must not release the pending show")
	}
}

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

func TestToggleActionForState(t *testing.T) {
	const hwnd = uintptr(42)
	tests := []struct {
		name       string
		hidden     bool
		foreground uintptr
		want       toggleAction
	}{
		{"hidden window is shown", true, 0, toggleShow},
		{"focused window is hidden", false, hwnd, toggleHide},
		{"visible inactive window is focused", false, 7, toggleFocus},
		{"no foreground window focuses visible window", false, 0, toggleFocus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toggleActionForState(tt.hidden, tt.foreground, hwnd); got != tt.want {
				t.Fatalf("toggleActionForState(%t, %d, %d) = %d, want %d", tt.hidden, tt.foreground, hwnd, got, tt.want)
			}
		})
	}
}

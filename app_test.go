package main

import "testing"

func TestRelaunchParentPID(t *testing.T) {
	tests := []struct {
		name string
		args []string
		pid  uint32
		ok   bool
	}{
		{"no arguments", nil, 0, false},
		{"autorun", []string{"--autostart"}, 0, false},
		{"relaunch", []string{waitForPIDFlag, "4321"}, 4321, true},
		{"relaunch after other arguments", []string{"--autostart", waitForPIDFlag, "17"}, 17, true},
		{"flag without a value", []string{waitForPIDFlag}, 0, false},
		{"not a number", []string{waitForPIDFlag, "abc"}, 0, false},
		{"zero is no process", []string{waitForPIDFlag, "0"}, 0, false},
		{"negative", []string{waitForPIDFlag, "-5"}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, ok := relaunchParentPID(tt.args)
			if ok != tt.ok || (ok && pid != tt.pid) {
				t.Fatalf("relaunchParentPID(%q) = %d, %t; want %d, %t", tt.args, pid, ok, tt.pid, tt.ok)
			}
		})
	}
}

// A relaunched copy inherits the value its parent set. Its own arguments
// must come out the same, or every update would grow the list and the two
// copies would ask WebView2 for different options on one user-data folder.

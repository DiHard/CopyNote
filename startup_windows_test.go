package main

import (
	"strings"
	"testing"
)

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
func TestWebViewArgumentsSurviveARelaunch(t *testing.T) {
	own := strings.Join(browserArgs, " ")
	const debug = "--remote-debugging-port=9222"

	if got := webViewArguments(""); got != own {
		t.Fatalf("nothing inherited: %q", got)
	}
	if got := webViewArguments(webViewArguments("")); got != own {
		t.Fatalf("a relaunched copy must get the same arguments, got %q", got)
	}
	first := webViewArguments(debug)
	if first != own+" "+debug {
		t.Fatalf("the caller's own arguments are kept after ours: %q", first)
	}
	if again := webViewArguments(webViewArguments(first)); again != first {
		t.Fatalf("two relaunches later: %q, want %q", again, first)
	}
	if got := webViewArguments("  " + own + "  " + own + " " + debug + " "); got != first {
		t.Fatalf("a value an older build already doubled is repaired: %q", got)
	}
}

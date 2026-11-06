package main

import (
	"strings"
	"testing"
)

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

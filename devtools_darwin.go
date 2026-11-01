//go:build devtools

package main

// A development build (go build -tags devtools) takes commands on the Unix
// socket named by COPYNOTE_DEVTOOLS, one per line, and answers each with one
// line — the macOS counterpart of driving WebView2 over CDP on Windows:
//
//	eval <js>     evaluate js in the page, answer its result
//	snap <path>   save the page as a PNG
//	show | hide | toggle | settings | state | quit
//
// A normal build has none of this.

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"copynote/internal/macui"
)

func init() {
	devtoolsStart = func(view *macui.WebView) {
		path := os.Getenv("COPYNOTE_DEVTOOLS")
		if path == "" {
			return
		}
		_ = os.Remove(path)
		ln, err := net.Listen("unix", path)
		if err != nil {
			log.Printf("devtools: %v", err)
			return
		}
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				go serveDevtools(view, conn)
			}
		}()
	}
}

func serveDevtools(view *macui.WebView, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	lines := bufio.NewScanner(conn)
	lines.Buffer(make([]byte, 1<<20), 1<<20)
	for lines.Scan() {
		cmd, arg, _ := strings.Cut(lines.Text(), " ")
		reply := make(chan string, 1)
		view.Dispatch(func() { runDevtools(cmd, arg, func(s string) { reply <- s }) })
		_, _ = fmt.Fprintln(conn, strings.ReplaceAll(<-reply, "\n", `\n`))
	}
}

func runDevtools(cmd, arg string, answer func(string)) {
	switch cmd {
	case "eval":
		macui.EvalResult(arg, answer)
	case "snap":
		macui.Snapshot(arg, answer)
	case "show":
		userRequest(requestShow)
		answer("ok")
	case "settings":
		userRequest(requestSettings)
		answer("ok")
	case "hide":
		hideWindow(true)
		answer("ok")
	case "toggle":
		userToggle()
		answer("ok")
	case "quit":
		answer("ok")
		goQuit()
	case "state":
		answer(fmt.Sprintf("phase=%d ready=%v key=%v hotkey=%v", phase, uiReady, macui.WindowKey(), liveHotkeyOn))
	default:
		answer("unknown command")
	}
}

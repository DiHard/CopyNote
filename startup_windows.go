package main

import (
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/windows"

	"copynote/internal/winutil"
)

const waitForPIDFlag = "--wait-for-pid"

// relaunchParentWait bounds how long a relaunched copy waits for the copy
// that started it. That copy has only its deferred cleanup left to do, so
// one still there after this long is stuck — and waiting for it without a
// limit left the user with no CopyNote at all and nothing on screen to say
// why.
const relaunchParentWait = 30 * time.Second

// relaunchParentPID finds the process a relaunched copy was told to wait for.
func relaunchParentPID(args []string) (uint32, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != waitForPIDFlag {
			continue
		}
		pid, err := strconv.ParseUint(args[i+1], 10, 32)
		return uint32(pid), err == nil && pid != 0
	}
	return 0, false
}

// waitForRelaunchParent is used by a relaunched copy before it creates
// WebView2. The old process should be gone first: until it is, its WebView2
// still has the user-data directory open, and starting both at once has made
// the new copy fail during controller creation.
func waitForRelaunchParent() {
	pid, ok := relaunchParentPID(os.Args[1:])
	if !ok {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		// The parent may have exited before this process opened its handle.
		return
	}
	defer func() { _ = windows.CloseHandle(h) }() // The wait below has already produced its result.
	if startedAfter(h, windows.CurrentProcess()) {
		// The parent is gone and Windows has given its PID to something new.
		log.Printf("relaunch: process %d is not the copy that started this one; not waiting", pid)
		return
	}
	event, err := windows.WaitForSingleObject(h, uint32(relaunchParentWait/time.Millisecond))
	switch {
	case err != nil:
		log.Printf("relaunch: waiting for process %d: %v", pid, err)
	case event == uint32(windows.WAIT_TIMEOUT):
		log.Printf("relaunch: process %d is still running after %v; starting anyway", pid, relaunchParentWait)
	}
}

// startedAfter reports whether process a was created later than process b.
// False when either creation time cannot be read.
func startedAfter(a, b windows.Handle) bool {
	created := func(h windows.Handle) (int64, bool) {
		var creation, exit, kernel, user windows.Filetime
		if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
			return 0, false
		}
		return creation.Nanoseconds(), true
	}
	ta, okA := created(a)
	tb, okB := created(b)
	return okA && okB && ta > tb
}

// logDir is %LOCALAPPDATA%\CopyNote, beside the WebView2 profile.
func logDir() string {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "CopyNote")
	}
	return filepath.Join(os.TempDir(), "CopyNote")
}

func systemLocale() string { return winutil.SystemLocale() }

func messageBox(text string, ask bool) bool { return winutil.MessageBox(text, ask) }

// serveFrontend serves the embedded frontend on a loopback port. WebView2's
// NavigateToString gives an about:blank origin that breaks ES modules, hence
// http://127.0.0.1:PORT/; only one request is made, everything is inlined.
// It returns the listener, so the caller can close it before a relaunch, and
// the URL.
func serveFrontend() (net.Listener, string, error) {
	staticFS, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		return nil, "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/", http.FileServer(http.FS(staticFS)))
		_ = http.Serve(ln, mux)
	}()
	return ln, fmt.Sprintf("http://%s/", ln.Addr().String()), nil
}

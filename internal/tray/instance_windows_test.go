//go:build windows

package tray

import (
	"fmt"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"copynote/internal/winutil"
)

// testNames returns a window class and message name unique to the test, so
// a CopyNote running on the developer's machine is never contacted.
func testNames(t *testing.T) (className, messageName string) {
	t.Helper()
	suffix := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	return "CopyNoteTestTrayWnd-" + suffix, "dev.copynote.test.SHOW-" + suffix
}

// startMessageOnlyWindow creates a message-only window of className on its
// own OS thread — like the real tray window — and forwards every registered
// message it receives (0xC000–0xFFFF) to got.
func startMessageOnlyWindow(t *testing.T, className string, got chan<- uint32) {
	t.Helper()
	created := make(chan uintptr, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		cls, _ := windows.UTF16PtrFromString(className)
		hInstance, _, _ := procGetModuleHandleW.Call(0)
		wc := wndClassExW{
			cbSize: uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc: syscall.NewCallback(func(hwnd, msgID, wParam, lParam uintptr) uintptr {
				if msgID >= 0xC000 && msgID <= 0xFFFF {
					select {
					case got <- uint32(msgID):
					default:
					}
					return 0
				}
				r, _, _ := procDefWindowProcW.Call(hwnd, msgID, wParam, lParam)
				return r
			}),
			hInstance:     hInstance,
			lpszClassName: cls,
		}
		_, _, _ = procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0,
			0, 0, 0, 0, hwndMessage, 0, hInstance, 0)
		created <- hwnd
		if hwnd == 0 {
			return
		}
		var m msg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		_, _, _ = procDestroyWindow.Call(hwnd)
	}()

	hwnd := <-created
	if hwnd == 0 {
		t.Fatal("CreateWindowExW failed for the message-only test window")
	}
	t.Cleanup(func() {
		_, _, _ = procPostMessageW.Call(hwnd, uintptr(winutil.WM_QUIT), 0, 0)
		<-done
	})
}

func TestPostToRunningTray_DeliversToMessageOnlyWindow(t *testing.T) {
	className, messageName := testNames(t)
	got := make(chan uint32, 1)
	startMessageOnlyWindow(t, className, got)
	want, err := winutil.RegisterWindowMessage(messageName)
	if err != nil {
		t.Fatal(err)
	}

	if !postToRunningTray(className, messageName, time.Second) {
		t.Fatal("postToRunningTray found no window")
	}
	select {
	case id := <-got:
		if id != want {
			t.Fatalf("received message %#x, want %#x", id, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("message-only window did not receive the show request")
	}
}

func TestPostToRunningTray_WaitsForWindowCreatedLater(t *testing.T) {
	className, messageName := testNames(t)
	got := make(chan uint32, 1)
	result := make(chan bool, 1)
	go func() { result <- postToRunningTray(className, messageName, 5*time.Second) }()

	// A running instance takes the mutex long before it creates its tray window.
	time.Sleep(300 * time.Millisecond)
	startMessageOnlyWindow(t, className, got)

	select {
	case ok := <-result:
		if !ok {
			t.Fatal("request was not delivered to a window created while waiting")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("postToRunningTray did not return")
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("window did not receive the show request")
	}
}

func TestPostToRunningTray_GivesUpWithoutWindow(t *testing.T) {
	className, messageName := testNames(t)
	start := time.Now()
	if postToRunningTray(className, messageName, 200*time.Millisecond) {
		t.Fatal("reported delivery although no window exists")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("gave up after %v, want about 200ms", elapsed)
	}
}

func TestShowRequestWaitsUntilReady(t *testing.T) {
	shown := 0
	tr := &Tray{OnShow: func() { shown++ }, showMsgID: 0xC0DE}
	prev := instance
	instance = tr
	defer func() { instance = prev }()

	trayWndProc(0, uintptr(tr.showMsgID), 0, 0)
	if shown != 0 {
		t.Fatalf("window shown %d times before WebView2 was ready, want 0", shown)
	}

	tr.ready.Store(true)
	trayWndProc(0, msgSetReady, 0, 0)
	if shown != 1 {
		t.Fatalf("window shown %d times once ready, want the deferred request (1)", shown)
	}

	trayWndProc(0, uintptr(tr.showMsgID), 0, 0)
	if shown != 2 {
		t.Fatalf("window shown %d times, want a request after ready to show at once (2)", shown)
	}

	trayWndProc(0, msgSetReady, 0, 0)
	if shown != 2 {
		t.Fatalf("window shown %d times, want no stale deferred request (2)", shown)
	}
}

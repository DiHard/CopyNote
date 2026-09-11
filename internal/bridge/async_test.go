package bridge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeHost struct {
	start     func(int) error
	script    string
	callbacks chan func()
	evaluated string
}

func (h *fakeHost) Bind(_ string, fn interface{}) error { h.start = fn.(func(int) error); return nil }
func (h *fakeHost) Init(script string)                  { h.script = script }
func (h *fakeHost) Dispatch(fn func())                  { h.callbacks <- fn }
func (h *fakeHost) Eval(script string)                  { h.evaluated = script }

func TestBindingReturnsBeforeSlowWorkAndDispatchesResult(t *testing.T) {
	h := &fakeHost{callbacks: make(chan func(), 1)}
	a := NewAsync(h)
	defer a.Close()
	release := make(chan struct{})
	if err := a.Bind("check", func(ctx context.Context) (any, error) {
		select {
		case <-release:
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}); err != nil {
		t.Fatal(err)
	}
	returned := make(chan error, 1)
	go func() { returned <- h.start(17) }()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("kickoff blocked on slow work")
	}
	close(release)
	select {
	case callback := <-h.callbacks:
		if h.evaluated != "" {
			t.Fatal("worker touched UI directly")
		}
		callback()
		if !strings.Contains(h.evaluated, `(17,{"value":"done"})`) {
			t.Fatal(h.evaluated)
		}
	case <-time.After(time.Second):
		t.Fatal("no completion callback")
	}
}

func TestCloseCancelsWorkersAndSuppressesQueuedCallbacks(t *testing.T) {
	h := &fakeHost{callbacks: make(chan func(), 1)}
	a := NewAsync(h)
	if err := a.Bind("check", func(context.Context) (any, error) { return nil, errors.New("network failed") }); err != nil {
		t.Fatal(err)
	}
	if err := h.start(1); err != nil {
		t.Fatal(err)
	}
	var callback func()
	select {
	case callback = <-h.callbacks:
	case <-time.After(time.Second):
		t.Fatal("no callback")
	}
	a.Close()
	callback()
	if h.evaluated != "" {
		t.Fatal("callback evaluated after Close")
	}
	if err := h.start(2); err == nil {
		t.Fatal("accepted work after Close")
	}

	h2 := &fakeHost{callbacks: make(chan func(), 1)}
	a2 := NewAsync(h2)
	started := make(chan struct{})
	if err := a2.Bind("check", func(ctx context.Context) (any, error) { close(started); <-ctx.Done(); return nil, ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	if err := h2.start(1); err != nil {
		t.Fatal(err)
	}
	<-started
	closed := make(chan struct{})
	go func() { a2.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel worker")
	}
	if len(h2.callbacks) != 0 {
		t.Fatal("cancelled worker dispatched callback")
	}
}

func TestErrorsAreJSONEncoded(t *testing.T) {
	h := &fakeHost{callbacks: make(chan func(), 1)}
	a := NewAsync(h)
	defer a.Close()
	if err := a.Bind("check", func(context.Context) (any, error) { return nil, errors.New("failure\n\"quoted\" <script>") }); err != nil {
		t.Fatal(err)
	}
	if err := h.start(3); err != nil {
		t.Fatal(err)
	}
	select {
	case callback := <-h.callbacks:
		callback()
		if !strings.Contains(h.evaluated, `"error":"failure\n\"quoted\" \u003cscript\u003e"`) {
			t.Fatal(h.evaluated)
		}
	case <-time.After(time.Second):
		t.Fatal("no callback")
	}
}

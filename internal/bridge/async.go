// Package bridge runs slow operations outside the WebView2 message thread.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Host interface {
	Bind(string, interface{}) error
	Init(string)
	Dispatch(func())
	Eval(string)
}

type Async struct {
	host   Host
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func NewAsync(host Host) *Async {
	ctx, cancel := context.WithCancel(context.Background())
	return &Async{host: host, ctx: ctx, cancel: cancel}
}

// DefaultTimeout bounds how long the page waits for a worker result before
// its Promise rejects. Long operations such as downloads use BindTimeout.
const DefaultTimeout = 10 * time.Second

// Bind installs a Promise API backed by a short kickoff binding and a worker.
// The host's Init and Bind calls must run before the initial navigation.
func (a *Async) Bind(name string, fn func(context.Context) (any, error)) error {
	return a.BindTimeout(name, DefaultTimeout, fn)
}

// BindTimeout is Bind with an explicit page-side timeout. The worker keeps
// running after the Promise rejects; the page drops its late result.
func (a *Async) BindTimeout(name string, timeout time.Duration, fn func(context.Context) (any, error)) error {
	start := "__start_" + name
	finish := "__finish_" + name
	if err := a.host.Bind(start, func(id int) error {
		a.mu.Lock()
		if a.closed {
			a.mu.Unlock()
			return errors.New("application is closing")
		}
		a.wg.Add(1)
		a.mu.Unlock()
		go func() {
			defer a.wg.Done()
			value, err := fn(a.ctx)
			payload := struct {
				Value any    `json:"value"`
				Error string `json:"error,omitempty"`
			}{Value: value}
			if err != nil {
				payload.Value = nil
				payload.Error = err.Error()
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				raw, _ = json.Marshal(map[string]string{"error": err.Error()})
			}
			if a.ctx.Err() != nil {
				return
			}
			a.host.Dispatch(func() {
				if a.ctx.Err() == nil {
					a.host.Eval(fmt.Sprintf("window[%q]?.(%d,%s)", finish, id, raw))
				}
			})
		}()
		return nil
	}); err != nil {
		return err
	}
	// Pending callbacks live inside the page and are removed on success,
	// failure, timeout, or navigation. Data is always JSON encoded above.
	a.host.Init(fmt.Sprintf(`(() => {
 let next = 0;
 const pending = new Map();
 window[%q] = (id, result) => {
  const p = pending.get(id);
  if (!p) return;
  pending.delete(id);
  clearTimeout(p.timer);
  result.error ? p.reject(new Error(result.error)) : p.resolve(result.value);
 };
 window[%q] = () => new Promise((resolve, reject) => {
  const id = ++next;
  const timer = setTimeout(() => {
   pending.delete(id);
   reject(new Error("Operation timed out"));
  }, %d);
  pending.set(id, {resolve, reject, timer});
  window[%q](id).catch(error => {
   const p = pending.get(id);
   if (!p) return;
   pending.delete(id); clearTimeout(timer); reject(error);
  });
 });
})();`, finish, name, timeout.Milliseconds(), start))
	return nil
}

// Close cancels workers before the WebView is destroyed. Already queued
// callbacks check the same context and never touch a destroyed window.
func (a *Async) Close() {
	a.mu.Lock()
	a.closed = true
	a.cancel()
	a.mu.Unlock()
	a.wg.Wait()
}

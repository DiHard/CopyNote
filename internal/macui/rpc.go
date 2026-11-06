//go:build darwin

package macui

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// The page calls a bound function as window.name(...args) and gets a
// Promise; the call arrives here as one JSON message. These are the same
// rules go-webview2 applies on Windows, so a binding behaves alike on both:
// arguments decode into the function's parameter types, and the function
// returns nothing, a value, an error, or a value and an error.

type rpcMessage struct {
	ID     int               `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

var errorType = reflect.TypeFor[error]()

func checkBindable(fn any) error {
	t := reflect.TypeOf(fn)
	if t == nil || t.Kind() != reflect.Func {
		return errors.New("only functions can be bound")
	}
	if t.NumOut() > 2 || (t.NumOut() == 2 && !t.Out(1).Implements(errorType)) {
		return errors.New("a bound function returns a value, an error, or a value and an error")
	}
	return nil
}

// call decodes params into fn's parameters and runs it.
func call(fn any, params []json.RawMessage) (any, error) {
	v := reflect.ValueOf(fn)
	t := v.Type()
	if len(params) != t.NumIn() {
		return nil, fmt.Errorf("function takes %d arguments, got %d", t.NumIn(), len(params))
	}
	args := make([]reflect.Value, len(params))
	for i, raw := range params {
		arg := reflect.New(t.In(i))
		if err := json.Unmarshal(raw, arg.Interface()); err != nil {
			return nil, fmt.Errorf("argument %d: %w", i+1, err)
		}
		args[i] = arg.Elem()
	}
	out := v.Call(args)
	asError := func(r reflect.Value) error {
		if r.IsNil() {
			return nil
		}
		return r.Interface().(error)
	}
	switch len(out) {
	case 0:
		return nil, nil
	case 1:
		if t.Out(0).Implements(errorType) {
			return nil, asError(out[0])
		}
		return out[0].Interface(), nil
	default:
		if err := asError(out[1]); err != nil {
			return nil, err
		}
		return out[0].Interface(), nil
	}
}

// answer is the script that settles the page's Promise for call id.
// Rejections carry the error text as a string, as on Windows.
func answer(id int, result any, err error) string {
	settle := func(how, payload string) string {
		return fmt.Sprintf("window._rpc[%d].%s(%s); window._rpc[%d] = undefined", id, how, payload, id)
	}
	if err == nil {
		raw, marshalErr := json.Marshal(result)
		if marshalErr == nil {
			return settle("resolve", string(raw))
		}
		err = marshalErr
	}
	text, _ := json.Marshal(err.Error()) // a string always marshals
	return settle("reject", string(text))
}

// bindScript installs window[name] for a binding. rpcScript defines the
// helper it calls.
func bindScript(name string) string {
	quoted, _ := json.Marshal(name)
	return fmt.Sprintf("window.__copynoteBind(%s);", quoted)
}

// rpcScript runs before the page's own scripts. Uncaught errors go to the
// log: there is no console to read them from in a menu bar app.
const rpcScript = `(() => {
  const RPC = (window._rpc = window._rpc || { nextSeq: 1 });
  const post = (message) => window.webkit.messageHandlers.copynote.postMessage(JSON.stringify(message));
  window.__copynoteBind = (name) => {
    window[name] = (...params) => {
      const id = RPC.nextSeq++;
      const promise = new Promise((resolve, reject) => { RPC[id] = { resolve, reject }; });
      post({ id, method: name, params });
      return promise;
    };
  };
  const report = (text) => post({ id: 0, method: "", params: [JSON.stringify(String(text))] });
  window.addEventListener("error", (e) => report(e.message + " at " + e.filename + ":" + e.lineno));
  window.addEventListener("unhandledrejection", (e) => report("unhandled rejection: " + (e.reason && e.reason.stack || e.reason)));
})();`

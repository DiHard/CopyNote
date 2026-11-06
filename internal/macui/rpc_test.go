//go:build darwin

package macui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func params(t *testing.T, values ...any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, len(values))
	for i, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = raw
	}
	return out
}

func TestCallDecodesArgumentsAndReturnsValues(t *testing.T) {
	got, err := call(func(id string, n int) (string, error) { return strings.Repeat(id, n), nil }, params(t, "ab", 2))
	if err != nil || got != "abab" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := call(func() error { return errors.New("refused") }, nil); err == nil || err.Error() != "refused" {
		t.Fatalf("error-only result: %v", err)
	}
	if got, err := call(func() error { return nil }, nil); got != nil || err != nil {
		t.Fatalf("nil error: %v, %v", got, err)
	}
	type req struct {
		Token int `json:"token"`
	}
	got, err = call(func(r req) int { return r.Token }, params(t, map[string]int{"token": 7}))
	if err != nil || got != 7 {
		t.Fatalf("struct argument: %v, %v", got, err)
	}
}

func TestCallRejectsAMismatch(t *testing.T) {
	if _, err := call(func(string) {}, nil); err == nil {
		t.Fatal("missing argument accepted")
	}
	if _, err := call(func(int) {}, params(t, "text")); err == nil {
		t.Fatal("wrong argument type accepted")
	}
}

func TestCheckBindable(t *testing.T) {
	if checkBindable("not a function") == nil || checkBindable(func() (int, int) { return 0, 0 }) == nil {
		t.Fatal("unbindable value accepted")
	}
	if err := checkBindable(func(int) (string, error) { return "", nil }); err != nil {
		t.Fatal(err)
	}
}

func TestAnswer(t *testing.T) {
	if got := answer(3, []string{"a"}, nil); got != `window._rpc[3].resolve(["a"]); window._rpc[3] = undefined` {
		t.Fatalf("resolve: %s", got)
	}
	if got := answer(4, nil, errors.New("said \"no\"")); got != `window._rpc[4].reject("said \"no\""); window._rpc[4] = undefined` {
		t.Fatalf("reject: %s", got)
	}
	if got := answer(5, func() {}, nil); !strings.Contains(got, ".reject(") {
		t.Fatalf("unmarshalable result must reject: %s", got)
	}
}

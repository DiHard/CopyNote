package storage

import (
	"encoding/json"
	"reflect"
	"testing"

	"copynote/internal/model"
)

func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		`{"version":2,"entries":[]}`,
		`{"version":1,"entries":null}`,
		`{"version":999,"entries":[]}`,
		`{"version":2,"entries":[],"settings":{"hotkey":"Ctrl+Alt+N"}}`,
		`null`, `[]`, `{`, "\x00",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		store, err := decode(raw)
		if err != nil {
			return
		}
		if err := model.ValidateStore(store); err != nil {
			t.Fatalf("decoder accepted an invalid snapshot: %v", err)
		}
		if store.Entries == nil {
			t.Fatal("successful decode must normalize missing entries")
		}
		encoded, err := json.Marshal(store)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := decode(encoded)
		if err != nil || !reflect.DeepEqual(store, restored) {
			t.Fatalf("valid snapshot changed after JSON round trip: %v", err)
		}
	})
}
